package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserExternalIdTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))

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

func TestInsertInternalUsersStoresNullExternalUserId(t *testing.T) {
	db := setupUserExternalIdTestDB(t)

	first := &User{Username: "internal_one", Password: "password123", DisplayName: "Internal One", Role: common.RoleCommonUser}
	require.NoError(t, first.Insert(0))

	second := &User{Username: "internal_two", Password: "password123", DisplayName: "Internal Two", Role: common.RoleCommonUser}
	require.NoError(t, second.Insert(0))

	var nullCount int64
	require.NoError(t, db.Model(&User{}).Where("external_user_id IS NULL").Count(&nullCount).Error)
	require.EqualValues(t, 2, nullCount)
}

func TestNormalizeEmptyExternalUserIdsConvertsEmptyStringToNull(t *testing.T) {
	db := setupUserExternalIdTestDB(t)

	require.NoError(t, db.Exec(`
		INSERT INTO users (username, password, display_name, external_user_id)
		VALUES (?, ?, ?, ?)
	`, "legacy_internal", "password", "Legacy Internal", "").Error)

	require.NoError(t, normalizeEmptyExternalUserIds())

	var nullCount int64
	require.NoError(t, db.Model(&User{}).Where("username = ? AND external_user_id IS NULL", "legacy_internal").Count(&nullCount).Error)
	require.EqualValues(t, 1, nullCount)
}
