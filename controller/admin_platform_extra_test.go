package controller

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Gap-coverage tests for the platform CRUD admin endpoints.
//
// The base admin_platform_test.go covers the happy paths and the most obvious
// validation rejections. This file is for the subtler invariants that are
// easy to break silently — boundary values, tombstone uniqueness across
// multiple delete cycles, and the "no side effect" guarantees admins rely on
// when re-pointing a platform at a different shadow user.
// ============================================================================

// TestAdminPlatform_Update_RejectsNonPositiveShadowUserId enforces the same
// "positive integer" rule on Update that Create enforces. A 0 or negative
// value would otherwise sneak into the row and break PlatformAuth.
func TestAdminPlatform_Update_RejectsNonPositiveShadowUserId(t *testing.T) {
	router := setupAdminPlatformRouter(t)
	w := doRequest(router, "POST", "/api/admin/v2/platforms/", map[string]interface{}{
		"platform_id": "upd_neg_shadow",
	})
	require.Equal(t, 200, w.Code)
	id := int(parseResponse(t, w)["data"].(map[string]interface{})["id"].(float64))

	for _, bad := range []int{0, -1, -999} {
		t.Run(fmt.Sprintf("shadow=%d", bad), func(t *testing.T) {
			w := doRequest(router, "PATCH",
				fmt.Sprintf("/api/admin/v2/platforms/%d", id),
				map[string]interface{}{"shadow_user_id": bad})
			resp := parseResponse(t, w)
			assert.Equal(t, false, resp["success"], "should reject shadow=%d", bad)
		})
	}
}

// TestAdminPlatform_Update_EmptyBodyReturnsError ensures the handler does not
// silently accept a no-op PATCH with no recognized fields. Without this guard
// the response could confuse callers into believing something changed.
func TestAdminPlatform_Update_EmptyBodyReturnsError(t *testing.T) {
	router := setupAdminPlatformRouter(t)
	w := doRequest(router, "POST", "/api/admin/v2/platforms/", map[string]interface{}{
		"platform_id": "upd_empty_body",
	})
	require.Equal(t, 200, w.Code)
	id := int(parseResponse(t, w)["data"].(map[string]interface{})["id"].(float64))

	w = doRequest(router, "PATCH", fmt.Sprintf("/api/admin/v2/platforms/%d", id),
		map[string]interface{}{})
	resp := parseResponse(t, w)
	assert.Equal(t, false, resp["success"])
	assert.Contains(t, resp["message"], "无可更新字段")
}

// TestAdminPlatform_DeleteRecreate_MultipleCyclesTombstonesUnique guards
// the platform_id tombstoning logic against same-second collisions when an
// admin rapidly cycles delete+recreate. With second-resolution timestamps,
// two deletes in the same second would otherwise generate identical
// tombstones and trip the platforms.platform_id UNIQUE index.
func TestAdminPlatform_DeleteRecreate_MultipleCyclesTombstonesUnique(t *testing.T) {
	router := setupAdminPlatformRouter(t)
	pid := "cycle_pid"

	for i := 0; i < 3; i++ {
		// Create.
		w := doRequest(router, "POST", "/api/admin/v2/platforms/",
			map[string]interface{}{"platform_id": pid})
		require.Equal(t, 200, w.Code, "cycle %d create failed: %s", i, w.Body.String())
		id := int(parseResponse(t, w)["data"].(map[string]interface{})["id"].(float64))

		// Delete.
		w = doRequest(router, "DELETE", fmt.Sprintf("/api/admin/v2/platforms/%d", id), nil)
		require.Equal(t, 200, w.Code, "cycle %d delete failed: %s", i, w.Body.String())
	}

	// After 3 delete cycles we expect 3 soft-deleted rows whose platform_id
	// columns are all distinct tombstones (deleted_<id>_<ts><possible-suffix>).
	var tombstones []string
	require.NoError(t,
		model.DB.Unscoped().Model(&model.Platform{}).
			Where("deleted_at IS NOT NULL").
			Pluck("platform_id", &tombstones).Error)
	require.Len(t, tombstones, 3, "expected 3 tombstoned rows")

	seen := make(map[string]bool, len(tombstones))
	for _, ts := range tombstones {
		assert.True(t, strings.HasPrefix(ts, "deleted_"),
			"tombstone should be prefixed: %q", ts)
		assert.False(t, seen[ts], "tombstone collision detected: %q", ts)
		seen[ts] = true
	}
}

// TestAdminPlatform_DeleteRecreate_ReusesSameShadowUser documents the
// INTENTIONAL "lost sk, please re-issue" recovery semantics:
//
//   - When a platform is deleted, its tokens are cascade-disabled (kept for log integrity).
//   - Recreating the same platform_id (without an explicit shadow_user_id)
//     deterministically reuses the SAME shadow user, because shadow username
//     is derived from platform_id alone.
//   - The old tokens remain attached, still disabled. Re-authorizing any of
//     them via V2 tokens/authorize will auto-restore them (self-healing path).
//
// If strict tenant isolation is required across recreate, the admin must
// provide an explicit shadow_user_id pointing at a fresh user — covered by
// TestAdminPlatform_DeleteRecreate_StrictIsolationViaManualShadowUser below.
func TestAdminPlatform_DeleteRecreate_ReusesSameShadowUser(t *testing.T) {
	router := setupAdminPlatformRouter(t)
	pid := "isolation_check"

	w := doRequest(router, "POST", "/api/admin/v2/platforms/",
		map[string]interface{}{"platform_id": pid})
	require.Equal(t, 200, w.Code)
	firstData := parseResponse(t, w)["data"].(map[string]interface{})
	firstId := int(firstData["id"].(float64))
	firstShadow := int(firstData["shadow_user_id"].(float64))

	require.NoError(t, model.DB.Create(&model.Token{
		UserId: firstShadow, Key: "isolation_token_key_aaaaaaaaaaa",
		Name: "first-gen", Status: common.TokenStatusEnabled,
		CreatedTime: common.GetTimestamp(), ExpiredTime: -1, UnlimitedQuota: true,
	}).Error)

	w = doRequest(router, "DELETE", fmt.Sprintf("/api/admin/v2/platforms/%d", firstId), nil)
	require.Equal(t, 200, w.Code)

	w = doRequest(router, "POST", "/api/admin/v2/platforms/",
		map[string]interface{}{"platform_id": pid})
	require.Equal(t, 200, w.Code)
	secondShadow := int(parseResponse(t, w)["data"].(map[string]interface{})["shadow_user_id"].(float64))

	// Recovery: same platform_id => same shadow user.
	assert.Equal(t, firstShadow, secondShadow,
		"same platform_id intentionally reuses the existing shadow user (recovery semantics)")

	// The pre-existing token is still attached (UserId unchanged) but in
	// disabled state from the cascade in Delete.
	var orig model.Token
	require.NoError(t,
		model.DB.Where("key = ?", "isolation_token_key_aaaaaaaaaaa").First(&orig).Error)
	assert.Equal(t, firstShadow, orig.UserId)
	assert.Equal(t, common.TokenStatusDisabled, orig.Status,
		"cascade-disable should have flipped the token to disabled")
}

// TestAdminPlatform_DeleteRecreate_StrictIsolationViaManualShadowUser is the
// counterpart contract: when the admin wants the recreated platform to be a
// fresh tenant (no inherited tokens), they pass an explicit shadow_user_id
// at create time. The new platform then has zero residual tokens visible
// through V2 endpoints because the shadow user pointer differs.
func TestAdminPlatform_DeleteRecreate_StrictIsolationViaManualShadowUser(t *testing.T) {
	router := setupAdminPlatformRouter(t)
	pid := "isolation_strict"

	w := doRequest(router, "POST", "/api/admin/v2/platforms/",
		map[string]interface{}{"platform_id": pid})
	require.Equal(t, 200, w.Code)
	firstData := parseResponse(t, w)["data"].(map[string]interface{})
	firstId := int(firstData["id"].(float64))
	firstShadow := int(firstData["shadow_user_id"].(float64))

	require.NoError(t, model.DB.Create(&model.Token{
		UserId: firstShadow, Key: "strict_token_belongs_to_origin_a",
		Name: "first-gen", Status: common.TokenStatusEnabled,
		CreatedTime: common.GetTimestamp(), ExpiredTime: -1, UnlimitedQuota: true,
	}).Error)

	w = doRequest(router, "DELETE", fmt.Sprintf("/api/admin/v2/platforms/%d", firstId), nil)
	require.Equal(t, 200, w.Code)

	// Provision a fresh shadow user for the new tenant.
	fresh := &model.User{Username: "fresh_tenant", Email: "fresh@example.com", Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(fresh).Error)

	w = doRequest(router, "POST", "/api/admin/v2/platforms/", map[string]interface{}{
		"platform_id":    pid,
		"shadow_user_id": fresh.Id,
	})
	require.Equal(t, 200, w.Code, w.Body.String())
	newShadow := int(parseResponse(t, w)["data"].(map[string]interface{})["shadow_user_id"].(float64))

	assert.NotEqual(t, firstShadow, newShadow, "strict isolation: shadow user must differ")
	assert.Equal(t, fresh.Id, newShadow, "shadow_user_id should equal the explicit override")

	// The fresh tenant sees zero tokens via its shadow user.
	var freshSideTokens int64
	require.NoError(t,
		model.DB.Model(&model.Token{}).Where("user_id = ?", fresh.Id).Count(&freshSideTokens).Error)
	assert.Zero(t, freshSideTokens, "fresh tenant must start with no tokens")
}

// TestAdminPlatform_UpdateShadowUserId_DoesNotRelinkExistingTokens locks in
// the symmetric isolation guarantee for the UPDATE path: pointing the
// platform at a different shadow user does NOT move that platform's
// previously-registered tokens. The admin path can re-target *future*
// authorizations but historical tokens (and their logs) stay anchored.
func TestAdminPlatform_UpdateShadowUserId_DoesNotRelinkExistingTokens(t *testing.T) {
	router := setupAdminPlatformRouter(t)
	w := doRequest(router, "POST", "/api/admin/v2/platforms/",
		map[string]interface{}{"platform_id": "shadow_swap"})
	require.Equal(t, 200, w.Code)
	created := parseResponse(t, w)["data"].(map[string]interface{})
	platformId := int(created["id"].(float64))
	originalShadow := int(created["shadow_user_id"].(float64))

	// Token registered under the original shadow user.
	require.NoError(t, model.DB.Create(&model.Token{
		UserId: originalShadow, Key: "swap_token_keep_under_original_a",
		Name: "should-stay-put", Status: common.TokenStatusEnabled,
		CreatedTime: common.GetTimestamp(), ExpiredTime: -1, UnlimitedQuota: true,
	}).Error)

	// Repoint the platform at a brand-new shadow user.
	newShadow := &model.User{Username: "swap_target_user", Email: "swap@example.com", Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(newShadow).Error)

	w = doRequest(router, "PATCH", fmt.Sprintf("/api/admin/v2/platforms/%d", platformId),
		map[string]interface{}{"shadow_user_id": newShadow.Id})
	require.Equal(t, 200, w.Code, w.Body.String())

	// The existing token must still belong to the ORIGINAL shadow user.
	var tok model.Token
	require.NoError(t,
		model.DB.Where("key = ?", "swap_token_keep_under_original_a").First(&tok).Error)
	assert.Equal(t, originalShadow, tok.UserId,
		"updating shadow_user_id must not relink historical tokens")

	// And the new shadow user must have zero tokens.
	var newSideTokens int64
	require.NoError(t,
		model.DB.Model(&model.Token{}).Where("user_id = ?", newShadow.Id).Count(&newSideTokens).Error)
	assert.Zero(t, newSideTokens)
}

// TestAdminPlatform_List_ExcludesSoftDeletedRows asserts that soft-deleted
// platforms never appear in the admin list. Without this, the UI would
// silently surface tombstoned platform_ids (deleted_42_<ts>) and confuse
// operators.
func TestAdminPlatform_List_ExcludesSoftDeletedRows(t *testing.T) {
	router := setupAdminPlatformRouter(t)
	w := doRequest(router, "POST", "/api/admin/v2/platforms/",
		map[string]interface{}{"platform_id": "to_be_deleted"})
	require.Equal(t, 200, w.Code)
	id := int(parseResponse(t, w)["data"].(map[string]interface{})["id"].(float64))

	w = doRequest(router, "POST", "/api/admin/v2/platforms/",
		map[string]interface{}{"platform_id": "to_remain"})
	require.Equal(t, 200, w.Code)

	w = doRequest(router, "DELETE", fmt.Sprintf("/api/admin/v2/platforms/%d", id), nil)
	require.Equal(t, 200, w.Code)

	w = doRequest(router, "GET", "/api/admin/v2/platforms/?page=1&page_size=50", nil)
	require.Equal(t, 200, w.Code)
	resp := parseResponse(t, w)
	items := resp["data"].(map[string]interface{})["items"].([]interface{})

	for _, raw := range items {
		item := raw.(map[string]interface{})
		pid := item["platform_id"].(string)
		assert.False(t, strings.HasPrefix(pid, "deleted_"),
			"list must not surface tombstoned ids: %q", pid)
		assert.NotEqual(t, "to_be_deleted", pid,
			"deleted platform must not appear in list")
	}
}

// TestAdminPlatform_Update_UpdatesTimestamp ensures GORM's autoUpdateTime
// hook is wired: a successful PATCH bumps updated_time relative to the
// original created_time. If this regresses, audit trails lose chronology.
func TestAdminPlatform_Update_UpdatesTimestamp(t *testing.T) {
	router := setupAdminPlatformRouter(t)
	w := doRequest(router, "POST", "/api/admin/v2/platforms/",
		map[string]interface{}{"platform_id": "ts_check", "name": "Original"})
	require.Equal(t, 200, w.Code)
	id := int(parseResponse(t, w)["data"].(map[string]interface{})["id"].(float64))

	created, err := model.GetPlatformById(id)
	require.NoError(t, err)
	createdAt := created.UpdatedTime

	// Sleep just past one second so the seconds-resolution timestamp must move.
	time.Sleep(1100 * time.Millisecond)

	w = doRequest(router, "PATCH", fmt.Sprintf("/api/admin/v2/platforms/%d", id),
		map[string]interface{}{"name": "Renamed"})
	require.Equal(t, 200, w.Code)

	updated, err := model.GetPlatformById(id)
	require.NoError(t, err)
	assert.Greater(t, updated.UpdatedTime, createdAt,
		"updated_time should advance after a successful PATCH")
}
