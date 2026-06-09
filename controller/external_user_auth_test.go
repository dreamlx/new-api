package controller

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestV1AuthMatrix exercises every PlatformAuth failure branch on a
// representative V1 endpoint (stats GET). We don't repeat the matrix
// across all 11 V1 endpoints — they share the same middleware code path
// and once one path is covered, the rest are exercised by the existing
// happy-path tests that already inject auth via setupTestRouter(t).
func TestV1AuthMatrix(t *testing.T) {
	router := setupTestRouter(t)
	defaultPid := defaultTestPlatformHeaders["X-Platform-Id"]
	defaultSk := defaultTestPlatformHeaders["X-Platform-Sk"]

	// Seed a user the GET endpoint can resolve so a 200 path is meaningful.
	require.NoError(t, model.DB.Create(&model.User{
		Username: "auth_matrix_user", Email: "matrix@example.com",
		ExternalUserId: ptrExternalUserId("matrix_001"), IsExternal: true, Quota: 1000,
	}).Error)

	t.Run("无header返回401", func(t *testing.T) {
		w := doRequestNoAuth(router, "GET", "/api/user/external/matrix_001/stats", nil)
		assert.Equal(t, 401, w.Code)
	})

	t.Run("仅platform_id无sk返回401", func(t *testing.T) {
		w := doAuthRequest(router, "GET", "/api/user/external/matrix_001/stats",
			map[string]string{"X-Platform-Id": defaultPid}, nil)
		assert.Equal(t, 401, w.Code)
	})

	t.Run("错误sk返回401", func(t *testing.T) {
		w := doAuthRequest(router, "GET", "/api/user/external/matrix_001/stats",
			map[string]string{"X-Platform-Id": defaultPid, "X-Platform-Sk": "pk_wrong"}, nil)
		assert.Equal(t, 401, w.Code)
	})

	t.Run("未知platform_id返回401", func(t *testing.T) {
		w := doAuthRequest(router, "GET", "/api/user/external/matrix_001/stats",
			map[string]string{"X-Platform-Id": "no_such_platform", "X-Platform-Sk": defaultSk}, nil)
		assert.Equal(t, 401, w.Code)
	})

	t.Run("disabled_platform返回403", func(t *testing.T) {
		// Spin up a second platform we can disable independently.
		pid, sk := setupTestPlatform(t, "v1_disabled_path")
		disablePlatform(t, pid)
		w := doAuthRequest(router, "GET", "/api/user/external/matrix_001/stats",
			authHeaders(pid, sk), nil)
		assert.Equal(t, 403, w.Code)
	})

	t.Run("通过鉴权返回200", func(t *testing.T) {
		w := doRequest(router, "GET", "/api/user/external/matrix_001/stats", nil)
		assert.Equal(t, 200, w.Code)
	})
}

// TestV1StatsSkMaskingAfterFix is the regression guard for the sk leak that
// was the V1 portion of this PR. The stats response must mask sk to the
// "sk-<first8>****<last4>" format — never the full key.
func TestV1StatsSkMaskingAfterFix(t *testing.T) {
	router := setupTestRouter(t)

	user := &model.User{
		Username: "stats_mask_user", Email: "mask@example.com",
		ExternalUserId: ptrExternalUserId("mask_001"), IsExternal: true, Quota: 1000,
	}
	require.NoError(t, model.DB.Create(user).Error)

	// 32-char key. Mask should leave 8 chars + **** + 4 chars visible.
	rawKey := "aabbccddeeff00112233445566778899"
	require.NoError(t, model.DB.Create(&model.Token{
		UserId: user.Id, Key: rawKey,
		Name: "primary", Status: 1, CreatedTime: 0, ExpiredTime: -1,
	}).Error)

	w := doRequest(router, "GET", "/api/user/external/mask_001/stats", nil)
	require.Equal(t, 200, w.Code)
	resp := parseResponse(t, w)
	body := w.Body.String()

	// Negative assertion: the full key must not appear anywhere in the response.
	assert.False(t, strings.Contains(body, rawKey),
		"stats response must not leak the full token key")

	// Positive assertion: the masked form does appear.
	data := resp["data"].(map[string]interface{})
	tokens := data["tokens"].([]interface{})
	require.NotEmpty(t, tokens)
	tok := tokens[0].(map[string]interface{})
	keyField := tok["key"].(string)
	assert.True(t, strings.HasPrefix(keyField, "sk-aabbccdd"), "should keep first 8 chars after sk-: got %q", keyField)
	assert.True(t, strings.Contains(keyField, "****"), "should contain masking marker")
	assert.True(t, strings.HasSuffix(keyField, "8899"), "should keep last 4 chars: got %q", keyField)
}

// TestV1CrossPlatformSharing documents (and locks in) the YAGNI choice that
// V1 has no per-platform isolation: a second platform's credentials can
// freely read another platform's external_users. If we ever add
// users.platform_id-based isolation, this test should be updated to assert
// 403/empty for the cross-platform read.
func TestV1CrossPlatformSharing(t *testing.T) {
	router := setupTestRouter(t)

	// Seed a user under the default platform's auth.
	require.NoError(t, model.DB.Create(&model.User{
		Username: "shared_user", Email: "shared@example.com",
		ExternalUserId: ptrExternalUserId("shared_001"), IsExternal: true, Quota: 1000,
	}).Error)

	// Switch to a different platform's credentials and read the same user.
	otherPid, otherSk := setupTestPlatform(t, "v1_share_other")
	w := doAuthRequest(router, "GET", "/api/user/external/shared_001/stats",
		authHeaders(otherPid, otherSk), nil)
	assert.Equal(t, 200, w.Code,
		"V1 currently has no platform-level isolation (intentional YAGNI)")
}
