package controller

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupV2TestRouter wires the V2 external routes behind PlatformAuth (matching
// production) and provisions a default platform whose credentials are
// auto-injected into every doRequest call from this test suite.
func setupV2TestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()

	testDB := setupTestDB()
	model.DB = testDB
	model.LOG_DB = testDB

	installDefaultTestPlatform(t, fmt.Sprintf("v2_%s", t.Name()))

	v2 := router.Group("/api/v2/external")
	v2.Use(middleware.PlatformAuth())
	{
		v2.POST("/tokens/authorize", V2AuthorizeToken)
		v2.GET("/logs", V2GetPlatformLogs)
	}

	t.Cleanup(resetDefaultTestPlatform)
	return router
}

func doV2Request(router *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	return doRequest(router, method, path, body)
}

func parseV2Response(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	return resp
}

// ==================== Authorize ====================

func TestV2AuthorizeToken(t *testing.T) {
	router := setupV2TestRouter(t)
	// The default platform_id is "testplt_v2_TestV2AuthorizeToken" — we capture
	// it via the middleware-injected ctx, so assertions compare against the
	// actual platform_id rather than a hardcoded literal.
	expectedPlatformId := defaultTestPlatformHeaders["X-Platform-Id"]

	t.Run("授权新Token成功", func(t *testing.T) {
		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"token_key": "sk-a99416b67cb54e178e9ffe8a55c255ae",
		})
		assert.Equal(t, 200, w.Code)
		resp := parseV2Response(t, w)
		assert.Equal(t, true, resp["success"])
		assert.Equal(t, "密钥授权成功", resp["message"])

		data := resp["data"].(map[string]interface{})
		assert.Equal(t, "authorized", data["status"])
		assert.Equal(t, expectedPlatformId, data["platform_id"])
		// Response carries the integer token_id; the plaintext sk MUST NOT be returned.
		assert.NotZero(t, data["token_id"])
		assert.Nil(t, data["token_key"], "sk plaintext must not be echoed back")

		// Verify shadow user exists and is sized for V2 platform consumption.
		var user model.User
		require.NoError(t, model.DB.Where("username = ?", "platform_"+expectedPlatformId).First(&user).Error)
		assert.Equal(t, model.PlatformShadowQuotaForNewUser, user.Quota)
		assert.True(t, user.IsExternal)

		// Verify token is unlimited.
		var token model.Token
		require.NoError(t, model.DB.Where("user_id = ?", user.Id).First(&token).Error)
		assert.True(t, token.UnlimitedQuota)
		assert.Equal(t, int64(-1), token.ExpiredTime)
	})

	t.Run("幂等性-重复授权返回exists", func(t *testing.T) {
		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"token_key": "sk-a99416b67cb54e178e9ffe8a55c255ae",
		})
		assert.Equal(t, 200, w.Code)
		resp := parseV2Response(t, w)
		data := resp["data"].(map[string]interface{})
		assert.Equal(t, "exists", data["status"])
		assert.NotZero(t, data["token_id"])
	})

	t.Run("幂等性-被禁用token重授权后自动恢复", func(t *testing.T) {
		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"token_key": "sk-disabledtokenrecoverytest00000001",
		})
		require.Equal(t, 200, w.Code)

		// Externally disable + change quota mode.
		model.DB.Model(&model.Token{}).
			Where("key = ?", "disabledtokenrecoverytest00000001").
			Updates(map[string]interface{}{"status": common.TokenStatusDisabled, "unlimited_quota": false})

		tk, err := model.GetTokenByKey("disabledtokenrecoverytest00000001", true)
		require.NoError(t, err)
		assert.Equal(t, common.TokenStatusDisabled, tk.Status)

		// Re-authorize — handler must restore the V2 invariant.
		w = doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"token_key": "sk-disabledtokenrecoverytest00000001",
		})
		assert.Equal(t, 200, w.Code)
		resp := parseV2Response(t, w)
		assert.Equal(t, "exists", resp["data"].(map[string]interface{})["status"])

		tk, err = model.GetTokenByKey("disabledtokenrecoverytest00000001", true)
		require.NoError(t, err)
		assert.Equal(t, common.TokenStatusEnabled, tk.Status)
		assert.True(t, tk.UnlimitedQuota)
	})

	t.Run("带metadata授权", func(t *testing.T) {
		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"token_key": "sk-b12345678901234567890123456789ab",
			"metadata": map[string]interface{}{
				"platform_user_id": "alice_123",
				"user_type":        "premium",
			},
		})
		assert.Equal(t, 200, w.Code)
		resp := parseV2Response(t, w)
		assert.Equal(t, "authorized", resp["data"].(map[string]interface{})["status"])

		// Verify token name includes platform_user_id.
		var token model.Token
		require.NoError(t,
			model.DB.Where("name = ?", fmt.Sprintf("v2_%s_alice_123", expectedPlatformId)).
				First(&token).Error)
		assert.True(t, token.Id > 0)
	})

	t.Run("token_key格式错误-无sk前缀", func(t *testing.T) {
		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"token_key": "abc123def456789xyz0123456789",
		})
		assert.Equal(t, 400, w.Code)
	})

	t.Run("token_key格式错误-含额外短横线", func(t *testing.T) {
		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"token_key": "sk-2-a99416b67cb54e178e9ffe8a55c255ae",
		})
		assert.Equal(t, 400, w.Code)
		resp := parseV2Response(t, w)
		assert.Contains(t, resp["message"], "不能包含")
	})

	t.Run("跨平台冲突", func(t *testing.T) {
		// Provision a second platform on the same in-memory DB and authorize a
		// token under it; then try to claim the same token_key as the default
		// platform — expect 409 because Token.Key has a UNIQUE index and the
		// handler explicitly rejects cross-platform reuse.
		otherPid, otherSk := setupTestPlatform(t, "v2_conflict_other")
		w := doAuthRequest(router, "POST", "/api/v2/external/tokens/authorize",
			authHeaders(otherPid, otherSk),
			map[string]interface{}{
				"token_key": "sk-c11111111111111111111111111111111",
			})
		assert.Equal(t, 200, w.Code)

		// Default platform tries the same sk -> 409.
		w = doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"token_key": "sk-c11111111111111111111111111111111",
		})
		assert.Equal(t, 409, w.Code)
		resp := parseV2Response(t, w)
		assert.Contains(t, resp["message"], "其他平台")
	})

	t.Run("平台用户复用", func(t *testing.T) {
		// Two registrations on the same platform share one shadow user.
		var countBefore int64
		model.DB.Model(&model.User{}).Where("username = ?", "platform_"+expectedPlatformId).Count(&countBefore)

		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"token_key": "sk-d22222222222222222222222222222222",
		})
		assert.Equal(t, 200, w.Code)

		var countAfter int64
		model.DB.Model(&model.User{}).Where("username = ?", "platform_"+expectedPlatformId).Count(&countAfter)
		assert.Equal(t, countBefore, countAfter)
	})

	// ===== Auth matrix =====

	t.Run("无header返回401", func(t *testing.T) {
		w := doRequestNoAuth(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"token_key": "sk-noauth1111111111111111111111111",
		})
		assert.Equal(t, 401, w.Code)
	})

	t.Run("错误sk返回401", func(t *testing.T) {
		w := doAuthRequest(router, "POST", "/api/v2/external/tokens/authorize",
			authHeaders(expectedPlatformId, "pk_wrong_key"),
			map[string]interface{}{"token_key": "sk-noauth2222222222222222222222222"})
		assert.Equal(t, 401, w.Code)
	})

	t.Run("未知platform_id返回401", func(t *testing.T) {
		w := doAuthRequest(router, "POST", "/api/v2/external/tokens/authorize",
			authHeaders("nonexistent_platform", "pk_irrelevant"),
			map[string]interface{}{"token_key": "sk-noauth3333333333333333333333333"})
		assert.Equal(t, 401, w.Code)
	})
}

// ==================== Platform Logs ====================

func TestV2GetPlatformLogs(t *testing.T) {
	router := setupV2TestRouter(t)
	expectedPlatformId := defaultTestPlatformHeaders["X-Platform-Id"]

	// Setup: get the shadow user the middleware created and seed it with tokens + logs.
	var user model.User
	require.NoError(t, model.DB.Where("username = ?", "platform_"+expectedPlatformId).First(&user).Error)

	token1 := &model.Token{
		UserId: user.Id, Key: "logtesttoken1aaaaaaaaaaaaaaaaaaaaa",
		Name: "v2_logtest_user1", Status: common.TokenStatusEnabled,
		CreatedTime: common.GetTimestamp(), ExpiredTime: -1, UnlimitedQuota: true,
	}
	token2 := &model.Token{
		UserId: user.Id, Key: "logtesttoken2bbbbbbbbbbbbbbbbbbbbb",
		Name: "v2_logtest_user2", Status: common.TokenStatusEnabled,
		CreatedTime: common.GetTimestamp(), ExpiredTime: -1, UnlimitedQuota: true,
	}
	require.NoError(t, model.DB.Create(token1).Error)
	require.NoError(t, model.DB.Create(token2).Error)

	now := common.GetTimestamp()
	for i := 0; i < 5; i++ {
		model.LOG_DB.Create(&model.Log{
			UserId: user.Id, Username: user.Username,
			TokenId:   token1.Id,
			CreatedAt: now - int64(i*100),
			Type:      model.LogTypeConsume, ModelName: "deepseek-chat",
			Quota: 1000, PromptTokens: 100, CompletionTokens: 50,
		})
	}
	for i := 0; i < 3; i++ {
		model.LOG_DB.Create(&model.Log{
			UserId: user.Id, Username: user.Username,
			TokenId:   token2.Id,
			CreatedAt: now - int64(i*100),
			Type:      model.LogTypeConsume, ModelName: "gpt-4o",
			Quota: 2000, PromptTokens: 200, CompletionTokens: 100,
		})
	}

	t.Run("查询全部日志", func(t *testing.T) {
		w := doV2Request(router, "GET", fmtV2LogPath("page=1&page_size=50"), nil)
		assert.Equal(t, 200, w.Code)

		resp := parseV2Response(t, w)
		data := resp["data"].(map[string]interface{})
		logs := data["logs"].([]interface{})
		assert.Len(t, logs, 8) // 5 + 3

		summary := data["summary"].(map[string]interface{})
		assert.Equal(t, float64(2), summary["unique_tokens"])
		assert.Equal(t, float64(11000), summary["total_quota_consumed"]) // 5*1000 + 3*2000

		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(8), pagination["total"])

		// Verify logs carry token_id (integer) NOT token_key (sk plaintext).
		firstLog := logs[0].(map[string]interface{})
		tokenId, ok := firstLog["token_id"].(float64)
		require.True(t, ok, "log item must carry numeric token_id")
		assert.True(t, int(tokenId) == token1.Id || int(tokenId) == token2.Id)
		assert.Nil(t, firstLog["token_key"], "logs must never echo sk plaintext")
	})

	t.Run("按token_id筛选", func(t *testing.T) {
		w := doV2Request(router, "GET",
			fmtV2LogPath(fmt.Sprintf("token_id=%d", token1.Id)), nil)
		assert.Equal(t, 200, w.Code)

		resp := parseV2Response(t, w)
		data := resp["data"].(map[string]interface{})
		logs := data["logs"].([]interface{})
		assert.Len(t, logs, 5)

		summary := data["summary"].(map[string]interface{})
		assert.Equal(t, float64(1), summary["unique_tokens"])
		assert.Equal(t, float64(5000), summary["total_quota_consumed"])
	})

	t.Run("token_id不属于该平台返回空", func(t *testing.T) {
		// Create a token under a DIFFERENT platform's shadow user.
		otherPid, _ := setupTestPlatform(t, "v2_log_other")
		var otherUser model.User
		require.NoError(t, model.DB.Where("username = ?", "platform_"+otherPid).First(&otherUser).Error)
		alienToken := &model.Token{
			UserId: otherUser.Id, Key: "aliendoesnotbelongtocurrentplatformX",
			Name: "alien", Status: common.TokenStatusEnabled,
			CreatedTime: common.GetTimestamp(), ExpiredTime: -1, UnlimitedQuota: true,
		}
		require.NoError(t, model.DB.Create(alienToken).Error)

		w := doV2Request(router, "GET", fmtV2LogPath(fmt.Sprintf("token_id=%d", alienToken.Id)), nil)
		assert.Equal(t, 200, w.Code)
		resp := parseV2Response(t, w)
		data := resp["data"].(map[string]interface{})
		assert.Len(t, data["logs"].([]interface{}), 0)
	})

	t.Run("token_id非法格式返回400", func(t *testing.T) {
		w := doV2Request(router, "GET", fmtV2LogPath("token_id=notanint"), nil)
		assert.Equal(t, 400, w.Code)
	})

	t.Run("分页", func(t *testing.T) {
		w := doV2Request(router, "GET", fmtV2LogPath("page=1&page_size=3"), nil)
		assert.Equal(t, 200, w.Code)

		resp := parseV2Response(t, w)
		data := resp["data"].(map[string]interface{})
		assert.Len(t, data["logs"].([]interface{}), 3)

		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(8), pagination["total"])
		assert.Equal(t, float64(3), pagination["total_pages"])
	})

	t.Run("无header返回401", func(t *testing.T) {
		w := doRequestNoAuth(router, "GET", fmtV2LogPath(""), nil)
		assert.Equal(t, 401, w.Code)
	})
}

// ==================== Integration: Authorize + Query ====================

func TestV2AuthorizeThenQueryLogs(t *testing.T) {
	router := setupV2TestRouter(t)
	expectedPlatformId := defaultTestPlatformHeaders["X-Platform-Id"]

	// 1. Authorize a token.
	w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
		"token_key": "sk-e33333333333333333333333333333333",
		"metadata":  map[string]interface{}{"platform_user_id": "bob"},
	})
	assert.Equal(t, 200, w.Code)
	resp := parseV2Response(t, w)
	tokenId := int(resp["data"].(map[string]interface{})["token_id"].(float64))

	// 2. Simulate consumption logs.
	var user model.User
	require.NoError(t, model.DB.Where("username = ?", "platform_"+expectedPlatformId).First(&user).Error)
	for i := 0; i < 3; i++ {
		model.LOG_DB.Create(&model.Log{
			UserId: user.Id, Username: user.Username,
			TokenId: tokenId, CreatedAt: common.GetTimestamp(),
			Type: model.LogTypeConsume, ModelName: "claude-3-5-sonnet",
			Quota: 5000, PromptTokens: 500, CompletionTokens: 200,
		})
	}

	// 3. Query logs.
	w = doV2Request(router, "GET", fmtV2LogPath(""), nil)
	assert.Equal(t, 200, w.Code)

	resp = parseV2Response(t, w)
	data := resp["data"].(map[string]interface{})
	logs := data["logs"].([]interface{})
	assert.Len(t, logs, 3)

	for _, l := range logs {
		logItem := l.(map[string]interface{})
		assert.Equal(t, float64(tokenId), logItem["token_id"])
		assert.Equal(t, "claude-3-5-sonnet", logItem["model_name"])
		assert.Nil(t, logItem["token_key"], "no sk plaintext in logs")
	}

	summary := data["summary"].(map[string]interface{})
	assert.Equal(t, float64(15000), summary["total_quota_consumed"])
}

// ==================== Disabled platform ====================

func TestV2DisabledPlatformIs403(t *testing.T) {
	router := setupV2TestRouter(t)
	pid := defaultTestPlatformHeaders["X-Platform-Id"]
	disablePlatform(t, pid)

	w := doV2Request(router, "GET", fmtV2LogPath(""), nil)
	assert.Equal(t, 403, w.Code)

	w = doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
		"token_key": "sk-f44444444444444444444444444444444",
	})
	assert.Equal(t, 403, w.Code)
}
