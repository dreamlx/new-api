package model

import (
	"regexp"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mkUser builds a fully-populated, unique User suitable for satisfying every
// UNIQUE/NOT-NULL constraint on the users table during model-level tests.
// Tests that need *multiple* users in the same DB must pass distinct names —
// aff_code in particular is uniqueIndex'd and an empty default would clash.
func mkUser(name string) *User {
	return &User{
		Username: name,
		Email:    name + "@example.com",
		AffCode:  "aff_" + name,
		Status:   common.UserStatusEnabled,
	}
}

// ============================================================================
// Model-level tests for Platform. The controller-level tests cover the
// admin HTTP surface; this file pins down the lower-level invariants of the
// Platform struct, its hashing primitives, and its Delete() transaction.
// ============================================================================

// resetPlatformState clears platform-related rows so each test starts fresh
// on the shared DB provided by TestMain (in task_cas_test.go). We do NOT
// replace model.DB — that would break sibling tests in this package.
func resetPlatformState(t *testing.T) {
	t.Helper()
	// Hard-delete every platform row (including soft-deleted tombstones) so
	// tests that exercise UNIQUE-index paths start from a clean slate.
	require.NoError(t, DB.Unscoped().Where("1=1").Delete(&Platform{}).Error)
}

func TestPlatform_HashIsDeterministicAndCaseSensitive(t *testing.T) {
	// Determinism: same input → same hash.
	a := HashPlatformSk("pk_alpha")
	b := HashPlatformSk("pk_alpha")
	assert.Equal(t, a, b)
	assert.Len(t, a, 64, "hex(sha256) is always 64 chars")

	// Case sensitivity: a single bit change flips the hash entirely.
	c := HashPlatformSk("pk_Alpha")
	assert.NotEqual(t, a, c)
}

func TestPlatform_VerifyPlatformSk_EdgeCases(t *testing.T) {
	p := &Platform{PlatformSkHash: HashPlatformSk("pk_xyz")}
	assert.True(t, p.VerifyPlatformSk("pk_xyz"))
	assert.False(t, p.VerifyPlatformSk("pk_xy"))
	assert.False(t, p.VerifyPlatformSk("pk_xyzz"))
	assert.False(t, p.VerifyPlatformSk(""), "empty plaintext is never valid")

	// nil receiver must not panic and must reject.
	var nilP *Platform
	assert.False(t, nilP.VerifyPlatformSk("anything"))

	// Empty stored hash must reject.
	empty := &Platform{PlatformSkHash: ""}
	assert.False(t, empty.VerifyPlatformSk("pk_xyz"))
}

func TestPlatform_Insert_RejectsBlanks(t *testing.T) {
	resetPlatformState(t)

	// Missing platform_id.
	assert.Error(t, (&Platform{PlatformSkHash: "x", ShadowUserId: 1}).Insert())
	// Missing hash.
	assert.Error(t, (&Platform{PlatformId: "p", ShadowUserId: 1}).Insert())
}

func TestPlatform_Insert_DefaultsStatusToEnabled(t *testing.T) {
	resetPlatformState(t)
	user := mkUser("u_default_status")
	require.NoError(t, DB.Create(user).Error)

	p := &Platform{
		PlatformId:     "default_status",
		PlatformSkHash: HashPlatformSk("pk"),
		ShadowUserId:   user.Id,
	}
	require.NoError(t, p.Insert())

	var got Platform
	require.NoError(t, DB.Where("platform_id = ?", "default_status").First(&got).Error)
	assert.Equal(t, common.UserStatusEnabled, got.Status,
		"Insert should default unspecified status to enabled")
}

func TestPlatform_Delete_TombstoneFormat(t *testing.T) {
	resetPlatformState(t)
	user := mkUser("u_tomb")
	require.NoError(t, DB.Create(user).Error)

	p := &Platform{
		PlatformId:     "tombstone_check",
		PlatformSkHash: HashPlatformSk("pk"),
		ShadowUserId:   user.Id,
		Status:         common.UserStatusEnabled,
	}
	require.NoError(t, p.Insert())

	_, err := p.Delete()
	require.NoError(t, err)

	// Re-read via Unscoped (soft-delete bypass) and assert the platform_id
	// was rewritten to the deleted_<id>_<ts>(suffix?) tombstone pattern.
	var deleted Platform
	require.NoError(t, DB.Unscoped().Where("id = ?", p.Id).First(&deleted).Error)
	pattern := regexp.MustCompile(`^deleted_\d+_\d+(_[a-zA-Z0-9]+)?$`)
	assert.True(t, pattern.MatchString(deleted.PlatformId),
		"tombstone should match the documented pattern: got %q", deleted.PlatformId)
	assert.NotZero(t, deleted.DeletedAt.Time, "DeletedAt should be set")
}

func TestPlatform_Delete_AllowsRecreateWithSamePlatformId(t *testing.T) {
	resetPlatformState(t)
	user := mkUser("u_recreate")
	require.NoError(t, DB.Create(user).Error)

	for i := 0; i < 2; i++ {
		p := &Platform{
			PlatformId:     "phoenix_id",
			PlatformSkHash: HashPlatformSk("pk"),
			ShadowUserId:   user.Id,
			Status:         common.UserStatusEnabled,
		}
		require.NoError(t, p.Insert(), "create cycle %d", i)
		_, err := p.Delete()
		require.NoError(t, err, "delete cycle %d", i)
	}
}

func TestPlatform_Delete_CascadesTokenStatusInTransaction(t *testing.T) {
	resetPlatformState(t)
	user := mkUser("u_casc")
	require.NoError(t, DB.Create(user).Error)

	p := &Platform{
		PlatformId:     "casc_test",
		PlatformSkHash: HashPlatformSk("pk"),
		ShadowUserId:   user.Id,
		Status:         common.UserStatusEnabled,
	}
	require.NoError(t, p.Insert())

	// Mix of enabled + already-disabled + exhausted tokens. Only the
	// currently-enabled ones should be flipped; others left untouched.
	require.NoError(t, DB.Create(&Token{UserId: user.Id, Key: "casc1", Status: common.TokenStatusEnabled, CreatedTime: common.GetTimestamp(), ExpiredTime: -1, UnlimitedQuota: true}).Error)
	require.NoError(t, DB.Create(&Token{UserId: user.Id, Key: "casc2", Status: common.TokenStatusEnabled, CreatedTime: common.GetTimestamp(), ExpiredTime: -1, UnlimitedQuota: true}).Error)
	require.NoError(t, DB.Create(&Token{UserId: user.Id, Key: "casc3", Status: common.TokenStatusDisabled, CreatedTime: common.GetTimestamp(), ExpiredTime: -1, UnlimitedQuota: true}).Error)
	require.NoError(t, DB.Create(&Token{UserId: user.Id, Key: "casc4", Status: common.TokenStatusExhausted, CreatedTime: common.GetTimestamp(), ExpiredTime: -1, UnlimitedQuota: true}).Error)

	disabled, err := p.Delete()
	require.NoError(t, err)
	assert.Equal(t, int64(2), disabled, "Delete should return count of newly-disabled tokens")

	var states []int
	require.NoError(t, DB.Model(&Token{}).Where("user_id = ?", user.Id).
		Order("id").Pluck("status", &states).Error)
	require.Len(t, states, 4)
	assert.Equal(t, common.TokenStatusDisabled, states[0], "casc1 should now be disabled")
	assert.Equal(t, common.TokenStatusDisabled, states[1], "casc2 should now be disabled")
	assert.Equal(t, common.TokenStatusDisabled, states[2], "casc3 was already disabled — keep")
	assert.Equal(t, common.TokenStatusExhausted, states[3], "casc4 was exhausted — must not be flipped")
}

func TestPlatform_UpdateFields_WhitelistOnly(t *testing.T) {
	resetPlatformState(t)
	user := mkUser("u_wl_main")
	require.NoError(t, DB.Create(user).Error)
	other := mkUser("u_wl_other")
	require.NoError(t, DB.Create(other).Error)

	p := &Platform{
		PlatformId:     "wl_check",
		PlatformSkHash: HashPlatformSk("pk_original"),
		ShadowUserId:   user.Id,
		Status:         common.UserStatusEnabled,
	}
	require.NoError(t, p.Insert())

	// Whitelist: name / status / shadow_user_id are mutable. Anything else
	// in the map is silently dropped, so platform_id and sk hash cannot be
	// abused as an "update" vector.
	require.NoError(t, p.UpdateFields(map[string]interface{}{
		"name":             "Renamed",
		"status":           common.UserStatusDisabled,
		"shadow_user_id":   other.Id,
		"platform_id":      "would_be_evil",
		"platform_sk_hash": HashPlatformSk("pk_attacker"),
	}))

	var got Platform
	require.NoError(t, DB.First(&got, p.Id).Error)
	assert.Equal(t, "Renamed", got.Name)
	assert.Equal(t, common.UserStatusDisabled, got.Status)
	assert.Equal(t, other.Id, got.ShadowUserId)
	assert.Equal(t, "wl_check", got.PlatformId, "platform_id MUST be immutable")
	assert.Equal(t, HashPlatformSk("pk_original"), got.PlatformSkHash, "sk hash MUST be immutable")
}

func TestPlatform_UpdateFields_NoOpOnEmptyMap(t *testing.T) {
	resetPlatformState(t)
	user := mkUser("u_noop")
	require.NoError(t, DB.Create(user).Error)

	p := &Platform{PlatformId: "noop_check", PlatformSkHash: HashPlatformSk("pk"),
		ShadowUserId: user.Id, Status: common.UserStatusEnabled}
	require.NoError(t, p.Insert())

	assert.NoError(t, p.UpdateFields(map[string]interface{}{}))
	assert.NoError(t, p.UpdateFields(map[string]interface{}{"unknown_field": 1}))
}
