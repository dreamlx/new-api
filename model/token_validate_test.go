package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTokenTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}))

	origDB := DB
	origRedis := common.RedisEnabled
	DB = db
	common.RedisEnabled = false
	initCol()
	t.Cleanup(func() {
		DB = origDB
		common.RedisEnabled = origRedis
	})
	return db
}

// TestValidateUserTokenRevokedVsInvalid locks the contract the relay auth gate
// (middleware.TokenAuth) relies on to return 403 api_key_revoked for a revoked
// (disabled) key while keeping 401 for an unknown key:
//   - a disabled token is returned NON-nil alongside ErrTokenInvalid, so the
//     middleware can inspect Status and map it to 403 api_key_revoked;
//   - an unknown key yields a nil token, which the middleware keeps as 401.
//
// If a future refactor of ValidateUserToken returns a nil token for the
// disabled case, the 403 path silently regresses to 401 — this test guards it.
func TestValidateUserTokenRevokedVsInvalid(t *testing.T) {
	setupTokenTestDB(t)

	user := &User{Username: "platform_lh", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(user).Error)

	tok := &Token{
		UserId: user.Id, Key: "revoketestkeyaaaaaaaaaaaaaaaaaaaaa",
		Name: "v2_lh", Status: common.TokenStatusEnabled,
		ExpiredTime: -1, UnlimitedQuota: true,
	}
	require.NoError(t, DB.Create(tok).Error)

	// Enabled → valid.
	got, err := ValidateUserToken(tok.Key)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, common.TokenStatusEnabled, got.Status)

	// Revoke (disable) → returned non-nil with ErrTokenInvalid + disabled status,
	// which is exactly what the middleware branches on for 403 api_key_revoked.
	require.NoError(t, DisableTokenByKey(tok.Key))
	got, err = ValidateUserToken(tok.Key)
	require.ErrorIs(t, err, ErrTokenInvalid)
	require.NotNil(t, got, "disabled token must be surfaced so middleware can return 403, not 401")
	assert.Equal(t, common.TokenStatusDisabled, got.Status)

	// Unknown key → nil token (middleware keeps this as 401, code "").
	got, err = ValidateUserToken("sk-doesnotexist00000000000000000000")
	require.ErrorIs(t, err, ErrTokenInvalid)
	assert.Nil(t, got)
}
