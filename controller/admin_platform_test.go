package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupAdminPlatformRouter wires the platform CRUD endpoints directly without
// AdminAuth — the goal of these tests is to verify the handler logic and the
// model contract (one-time sk reveal, cascade-disable on delete, sk never
// leaked through list/get/update). AdminAuth integration is tested elsewhere.
func setupAdminPlatformRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()

	testDB := setupTestDB()
	model.DB = testDB
	model.LOG_DB = testDB

	g := router.Group("/api/admin/v2/platforms")
	{
		g.POST("/", CreatePlatform)
		g.GET("/", ListPlatforms)
		g.GET("/:id", GetPlatform)
		g.PATCH("/:id", UpdatePlatform)
		g.DELETE("/:id", DeletePlatform)
	}
	return router
}

func TestAdminPlatform_Create_ReturnsPlaintextSkOnce(t *testing.T) {
	router := setupAdminPlatformRouter(t)

	w := doRequest(router, "POST", "/api/admin/v2/platforms/", map[string]interface{}{
		"platform_id": "alpha",
		"name":        "Alpha Inc",
	})
	require.Equal(t, 200, w.Code, w.Body.String())
	resp := parseResponse(t, w)
	data := resp["data"].(map[string]interface{})

	sk := data["platform_sk"].(string)
	require.True(t, strings.HasPrefix(sk, "pk_"), "sk must use pk_ prefix")
	require.True(t, len(sk) >= 40, "sk too short: %q", sk)

	// Stored form is hex(sha256(sk)); never the plaintext.
	var stored model.Platform
	require.NoError(t, model.DB.Where("platform_id = ?", "alpha").First(&stored).Error)
	want := sha256.Sum256([]byte(sk))
	assert.Equal(t, hex.EncodeToString(want[:]), stored.PlatformSkHash)
	assert.True(t, stored.VerifyPlatformSk(sk), "round-trip verify must succeed")
	assert.False(t, stored.VerifyPlatformSk(sk+"X"), "wrong sk must not verify")

	// Caller is warned the sk is one-time.
	assert.Contains(t, data["warning"], "仅本次")
}

func TestAdminPlatform_Create_RejectsBadPlatformId(t *testing.T) {
	router := setupAdminPlatformRouter(t)
	// Project convention: validation errors return HTTP 200 with success:false.
	// We assert on the response envelope, not the status code.
	for _, bad := range []string{"With Space", "UPPERCASE", "has.dot", "with$dollar", ""} {
		t.Run("reject_"+bad, func(t *testing.T) {
			w := doRequest(router, "POST", "/api/admin/v2/platforms/", map[string]interface{}{
				"platform_id": bad,
			})
			resp := parseResponse(t, w)
			assert.Equal(t, false, resp["success"], "should reject %q", bad)
		})
	}
}

func TestAdminPlatform_Create_DuplicatePlatformId(t *testing.T) {
	router := setupAdminPlatformRouter(t)
	body := map[string]interface{}{"platform_id": "dup_test", "name": "First"}
	w := doRequest(router, "POST", "/api/admin/v2/platforms/", body)
	require.Equal(t, 200, w.Code)

	w = doRequest(router, "POST", "/api/admin/v2/platforms/", body)
	assert.Equal(t, 409, w.Code)
}

func TestAdminPlatform_ListDoesNotLeakHash(t *testing.T) {
	router := setupAdminPlatformRouter(t)
	for _, id := range []string{"list_a", "list_b", "list_c"} {
		w := doRequest(router, "POST", "/api/admin/v2/platforms/", map[string]interface{}{"platform_id": id})
		require.Equal(t, 200, w.Code)
	}

	w := doRequest(router, "GET", "/api/admin/v2/platforms/?page=1&page_size=10", nil)
	require.Equal(t, 200, w.Code)
	body := w.Body.String()

	// Whatever item shape we render, the hash field name must not be in the JSON.
	assert.NotContains(t, body, "platform_sk_hash",
		"hash column name must not be exposed in list response")
	// And no platform_sk plaintext should ever be in a list/get response either.
	assert.NotContains(t, body, "\"platform_sk\":",
		"plaintext sk must only appear in the Create response")
}

func TestAdminPlatform_GetDoesNotLeakHash(t *testing.T) {
	router := setupAdminPlatformRouter(t)
	w := doRequest(router, "POST", "/api/admin/v2/platforms/", map[string]interface{}{"platform_id": "get_one"})
	require.Equal(t, 200, w.Code)
	id := int(parseResponse(t, w)["data"].(map[string]interface{})["id"].(float64))

	w = doRequest(router, "GET", fmt.Sprintf("/api/admin/v2/platforms/%d", id), nil)
	require.Equal(t, 200, w.Code)
	body := w.Body.String()
	assert.NotContains(t, body, "platform_sk_hash")
	assert.NotContains(t, body, "\"platform_sk\":")
}

func TestAdminPlatform_UpdateNameAndStatus(t *testing.T) {
	router := setupAdminPlatformRouter(t)
	w := doRequest(router, "POST", "/api/admin/v2/platforms/", map[string]interface{}{
		"platform_id": "upd_one", "name": "Original",
	})
	require.Equal(t, 200, w.Code)
	id := int(parseResponse(t, w)["data"].(map[string]interface{})["id"].(float64))

	body, _ := json.Marshal(map[string]interface{}{
		"name":   "Renamed",
		"status": common.UserStatusDisabled,
	})
	w = doRequest(router, "PATCH", fmt.Sprintf("/api/admin/v2/platforms/%d", id), json.RawMessage(body))
	require.Equal(t, 200, w.Code)

	p, err := model.GetPlatformById(id)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", p.Name)
	assert.Equal(t, common.UserStatusDisabled, p.Status)
}

func TestAdminPlatform_UpdateRejectsInvalidStatus(t *testing.T) {
	router := setupAdminPlatformRouter(t)
	w := doRequest(router, "POST", "/api/admin/v2/platforms/", map[string]interface{}{"platform_id": "upd_bad"})
	require.Equal(t, 200, w.Code)
	id := int(parseResponse(t, w)["data"].(map[string]interface{})["id"].(float64))

	w = doRequest(router, "PATCH", fmt.Sprintf("/api/admin/v2/platforms/%d", id),
		map[string]interface{}{"status": 99})
	resp := parseResponse(t, w)
	assert.Equal(t, false, resp["success"])
	assert.Contains(t, resp["message"], "status")
}

func TestAdminPlatform_DeleteCascadesTokenDisable(t *testing.T) {
	router := setupAdminPlatformRouter(t)
	w := doRequest(router, "POST", "/api/admin/v2/platforms/", map[string]interface{}{"platform_id": "del_cascade"})
	require.Equal(t, 200, w.Code)
	created := parseResponse(t, w)["data"].(map[string]interface{})
	id := int(created["id"].(float64))
	shadowUserId := int(created["shadow_user_id"].(float64))

	// Seed two enabled tokens under the shadow user.
	for _, key := range []string{"keep_alive_token_aaaaaaaaaaaaa", "keep_alive_token_bbbbbbbbbbbbb"} {
		require.NoError(t, model.DB.Create(&model.Token{
			UserId: shadowUserId, Key: key, Name: "t-" + key,
			Status: common.TokenStatusEnabled, CreatedTime: common.GetTimestamp(),
			ExpiredTime: -1, UnlimitedQuota: true,
		}).Error)
	}

	w = doRequest(router, "DELETE", fmt.Sprintf("/api/admin/v2/platforms/%d", id), nil)
	require.Equal(t, 200, w.Code)
	resp := parseResponse(t, w)
	assert.Equal(t, float64(2), resp["data"].(map[string]interface{})["tokens_disabled"])

	// Tokens still exist (soft-keep for log integrity) but status is disabled.
	var tokens []model.Token
	require.NoError(t, model.DB.Where("user_id = ?", shadowUserId).Find(&tokens).Error)
	require.Len(t, tokens, 2)
	for _, tok := range tokens {
		assert.Equal(t, common.TokenStatusDisabled, tok.Status, "token %s should be disabled", tok.Key)
	}

	// Platform itself is soft-deleted: lookup by platform_id must now miss.
	_, err := model.GetPlatformByPlatformId("del_cascade")
	assert.Error(t, err)
}

func TestAdminPlatform_DeleteNotFound(t *testing.T) {
	router := setupAdminPlatformRouter(t)
	w := doRequest(router, "DELETE", "/api/admin/v2/platforms/9999", nil)
	assert.Equal(t, 404, w.Code)
}
