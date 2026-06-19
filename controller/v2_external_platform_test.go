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

// setupV2TestRouter wires a fresh in-memory DB + the V2 external routes guarded
// by the real PlatformAuth middleware (mirroring production), and provisions a
// default platform whose credentials are auto-injected into every subsequent
// doRequest call. The platform identity is resolved from auth context — there
// is no platform_id path/query param. Returns the router plus the default
// platform's id and shadow user id for data setup and assertions.
func setupV2TestRouter(t *testing.T) (router *gin.Engine, platformId string, shadowUserId int) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router = gin.New()

	testDB := setupTestDB()
	model.DB = testDB
	model.LOG_DB = testDB

	installDefaultTestPlatform(t, fmt.Sprintf("v2_%s", t.Name()))
	t.Cleanup(resetDefaultTestPlatform)

	v2 := router.Group("/api/v2/external")
	v2.Use(middleware.PlatformAuth())
	{
		v2.POST("/tokens/authorize", V2AuthorizeToken)
		v2.GET("/logs", V2GetPlatformLogs)
	}

	platformId = defaultTestPlatformHeaders["X-Platform-Id"]
	shadow, err := model.GetOrCreatePlatformShadowUser(platformId)
	require.NoError(t, err)
	return router, platformId, shadow.Id
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
	router, pid, shadowUserId := setupV2TestRouter(t)

	t.Run("授权新Token成功", func(t *testing.T) {
		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"token_key": "sk-a99416b67cb54e178e9ffe8a55c255ae",
		})
		assert.Equal(t, 200, w.Code)
		resp := parseV2Response(t, w)
		assert.Equal(t, true, resp["success"])
		assert.Equal(t, "密钥授权成功", resp["message"])

		data := resp["data"].(map[string]interface{})
		assert.Equal(t, "sk-a99416b67cb54e178e9ffe8a55c255ae", data["token_key"])
		assert.Equal(t, "authorized", data["status"])
		// platform_id is derived from the authenticated platform, not the body.
		assert.Equal(t, pid, data["platform_id"])

		// Shadow user (provisioned by PlatformAuth) carries the bypass quota.
		var user model.User
		require.NoError(t, model.DB.First(&user, shadowUserId).Error)
		assert.Equal(t, PlatformQuotaForNewUser, user.Quota)
		assert.True(t, user.IsExternal)

		// Token is unlimited and never expires.
		var token model.Token
		require.NoError(t, model.DB.Where("user_id = ?", shadowUserId).First(&token).Error)
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

		// Token name includes platform_id (from auth) + platform_user_id.
		var token model.Token
		require.NoError(t, model.DB.Where("name = ?", fmt.Sprintf("v2_%s_alice_123", pid)).First(&token).Error)
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

	t.Run("body平台一致放行", func(t *testing.T) {
		// A client that still sends platform_id matching the authenticated
		// platform must keep working (backward compatibility).
		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"platform_id": pid,
			"token_key":   "sk-f44444444444444444444444444444444",
		})
		assert.Equal(t, 200, w.Code)
		resp := parseV2Response(t, w)
		assert.Equal(t, pid, resp["data"].(map[string]interface{})["platform_id"])
	})

	t.Run("body平台不一致拒绝", func(t *testing.T) {
		// A body platform_id that contradicts the authenticated platform is
		// rejected — an authenticated platform cannot mint tokens under another
		// platform's identity.
		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"platform_id": "someone_else",
			"token_key":   "sk-99999999999999999999999999999999",
		})
		assert.Equal(t, 403, w.Code)
		resp := parseV2Response(t, w)
		assert.Contains(t, resp["message"], "不一致")
	})

	t.Run("跨平台冲突", func(t *testing.T) {
		// Authorize a key on a SECOND platform (its own auth headers)...
		otherId, otherSk := setupTestPlatform(t, "v2other")
		w := doAuthRequest(router, "POST", "/api/v2/external/tokens/authorize",
			authHeaders(otherId, otherSk), map[string]interface{}{
				"token_key": "sk-c11111111111111111111111111111111",
			})
		assert.Equal(t, 200, w.Code)

		// ...then the same key on the default platform → 409.
		w = doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"token_key": "sk-c11111111111111111111111111111111",
		})
		assert.Equal(t, 409, w.Code)
		resp := parseV2Response(t, w)
		assert.Contains(t, resp["message"], "其他平台")
	})

	t.Run("平台用户复用", func(t *testing.T) {
		platformUsername := "platform_" + pid
		var countBefore int64
		model.DB.Model(&model.User{}).Where("username = ?", platformUsername).Count(&countBefore)

		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"token_key": "sk-d22222222222222222222222222222222",
		})
		assert.Equal(t, 200, w.Code)

		var countAfter int64
		model.DB.Model(&model.User{}).Where("username = ?", platformUsername).Count(&countAfter)
		assert.Equal(t, countBefore, countAfter)
	})
}

// TestV2AuthorizeMissingAuth verifies the route is actually guarded: without
// platform credentials PlatformAuth rejects before the handler runs.
func TestV2AuthorizeMissingAuth(t *testing.T) {
	router, _, _ := setupV2TestRouter(t)
	w := doRequestNoAuth(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
		"token_key": "sk-a99416b67cb54e178e9ffe8a55c255ae",
	})
	assert.Equal(t, 401, w.Code)
}

// ==================== Platform Logs ====================

func TestV2GetPlatformLogs(t *testing.T) {
	router, pid, shadowUserId := setupV2TestRouter(t)

	// Tokens + logs belong to the default platform's shadow user.
	token1 := &model.Token{
		UserId: shadowUserId, Key: "logtesttoken1aaaaaaaaaaaaaaaaaaaaa",
		Name: "v2_logtest_user1", Status: common.TokenStatusEnabled,
		CreatedTime: common.GetTimestamp(), ExpiredTime: -1, UnlimitedQuota: true,
	}
	token2 := &model.Token{
		UserId: shadowUserId, Key: "logtesttoken2bbbbbbbbbbbbbbbbbbbbb",
		Name: "v2_logtest_user2", Status: common.TokenStatusEnabled,
		CreatedTime: common.GetTimestamp(), ExpiredTime: -1, UnlimitedQuota: true,
	}
	require.NoError(t, model.DB.Create(token1).Error)
	require.NoError(t, model.DB.Create(token2).Error)

	now := common.GetTimestamp()
	for i := 0; i < 5; i++ {
		model.LOG_DB.Create(&model.Log{
			UserId: shadowUserId, Username: "platform_" + pid,
			TokenId:   token1.Id,
			CreatedAt: now - int64(i*100),
			Type:      model.LogTypeConsume, ModelName: "deepseek-chat",
			Quota: 1000, PromptTokens: 100, CompletionTokens: 50,
		})
	}
	for i := 0; i < 3; i++ {
		model.LOG_DB.Create(&model.Log{
			UserId: shadowUserId, Username: "platform_" + pid,
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
		assert.Equal(t, pid, data["platform_id"])
		logs := data["logs"].([]interface{})
		assert.Len(t, logs, 8) // 5 + 3

		summary := data["summary"].(map[string]interface{})
		assert.Equal(t, float64(2), summary["unique_tokens"])
		assert.Equal(t, float64(11000), summary["total_quota_consumed"]) // 5*1000 + 3*2000

		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(8), pagination["total"])

		firstLog := logs[0].(map[string]interface{})
		tokenKey := firstLog["token_key"].(string)
		assert.True(t, tokenKey == "sk-logtesttoken1aaaaaaaaaaaaaaaaaaaaa" || tokenKey == "sk-logtesttoken2bbbbbbbbbbbbbbbbbbbbb")
	})

	t.Run("按token_key筛选", func(t *testing.T) {
		w := doV2Request(router, "GET", fmtV2LogPath("token_key=sk-"+token1.Key), nil)
		assert.Equal(t, 200, w.Code)

		resp := parseV2Response(t, w)
		data := resp["data"].(map[string]interface{})
		logs := data["logs"].([]interface{})
		assert.Len(t, logs, 5) // only token1 logs

		summary := data["summary"].(map[string]interface{})
		assert.Equal(t, float64(1), summary["unique_tokens"])
		assert.Equal(t, float64(5000), summary["total_quota_consumed"]) // 5*1000
	})

	t.Run("token_key不存在返回空", func(t *testing.T) {
		w := doV2Request(router, "GET", fmtV2LogPath("token_key=sk-nonexistent"), nil)
		assert.Equal(t, 200, w.Code)

		resp := parseV2Response(t, w)
		data := resp["data"].(map[string]interface{})
		logs := data["logs"].([]interface{})
		assert.Len(t, logs, 0)
	})

	t.Run("分页", func(t *testing.T) {
		w := doV2Request(router, "GET", fmtV2LogPath("page=1&page_size=3"), nil)
		assert.Equal(t, 200, w.Code)

		resp := parseV2Response(t, w)
		data := resp["data"].(map[string]interface{})
		logs := data["logs"].([]interface{})
		assert.Len(t, logs, 3)

		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(8), pagination["total"])
		assert.Equal(t, float64(3), pagination["total_pages"])
	})
}

// TestV2GetPlatformLogsScopedToPlatform verifies a platform sees ONLY its own
// shadow user's logs — another platform's consumption never leaks across the
// auth boundary.
func TestV2GetPlatformLogsScopedToPlatform(t *testing.T) {
	router, pid, shadowUserId := setupV2TestRouter(t)

	// Default platform has 2 logs.
	token := &model.Token{
		UserId: shadowUserId, Key: "scopedtokenaaaaaaaaaaaaaaaaaaaaaaa",
		Name: "v2_scoped", Status: common.TokenStatusEnabled,
		CreatedTime: common.GetTimestamp(), ExpiredTime: -1, UnlimitedQuota: true,
	}
	require.NoError(t, model.DB.Create(token).Error)
	for i := 0; i < 2; i++ {
		model.LOG_DB.Create(&model.Log{
			UserId: shadowUserId, Username: "platform_" + pid, TokenId: token.Id,
			CreatedAt: common.GetTimestamp(), Type: model.LogTypeConsume,
			ModelName: "deepseek-chat", Quota: 1000,
		})
	}

	// A second platform with its own consumption.
	otherId, otherSk := setupTestPlatform(t, "v2scopeother")
	otherShadow, err := model.GetOrCreatePlatformShadowUser(otherId)
	require.NoError(t, err)
	otherToken := &model.Token{
		UserId: otherShadow.Id, Key: "othertokenbbbbbbbbbbbbbbbbbbbbbbbbb",
		Name: "v2_other", Status: common.TokenStatusEnabled,
		CreatedTime: common.GetTimestamp(), ExpiredTime: -1, UnlimitedQuota: true,
	}
	require.NoError(t, model.DB.Create(otherToken).Error)
	for i := 0; i < 4; i++ {
		model.LOG_DB.Create(&model.Log{
			UserId: otherShadow.Id, Username: "platform_" + otherId, TokenId: otherToken.Id,
			CreatedAt: common.GetTimestamp(), Type: model.LogTypeConsume,
			ModelName: "gpt-4o", Quota: 2000,
		})
	}

	// Authenticated as the SECOND platform, only its own 4 logs are visible.
	w := doAuthRequest(router, "GET", fmtV2LogPath("page=1&page_size=50"),
		authHeaders(otherId, otherSk), nil)
	assert.Equal(t, 200, w.Code)
	resp := parseV2Response(t, w)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, otherId, data["platform_id"])
	assert.Len(t, data["logs"].([]interface{}), 4)
}

// ==================== Integration: Authorize + Query ====================

func TestV2AuthorizeThenQueryLogs(t *testing.T) {
	router, pid, shadowUserId := setupV2TestRouter(t)

	// 1. Authorize a token (auto-auth as the default platform).
	w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
		"token_key": "sk-e33333333333333333333333333333333",
		"metadata":  map[string]interface{}{"platform_user_id": "bob"},
	})
	assert.Equal(t, 200, w.Code)
	resp := parseV2Response(t, w)
	tokenId := int(resp["data"].(map[string]interface{})["token_id"].(float64))

	// 2. Simulate consumption logs under the platform's shadow user.
	for i := 0; i < 3; i++ {
		model.LOG_DB.Create(&model.Log{
			UserId: shadowUserId, Username: "platform_" + pid,
			TokenId: tokenId, CreatedAt: common.GetTimestamp(),
			Type: model.LogTypeConsume, ModelName: "claude-3-5-sonnet",
			Quota: 5000, PromptTokens: 500, CompletionTokens: 200,
		})
	}

	// 3. Query platform logs.
	w = doV2Request(router, "GET", fmtV2LogPath(""), nil)
	assert.Equal(t, 200, w.Code)

	resp = parseV2Response(t, w)
	data := resp["data"].(map[string]interface{})
	logs := data["logs"].([]interface{})
	assert.Len(t, logs, 3)

	for _, l := range logs {
		logItem := l.(map[string]interface{})
		assert.Equal(t, "sk-e33333333333333333333333333333333", logItem["token_key"])
		assert.Equal(t, "claude-3-5-sonnet", logItem["model_name"])
	}

	summary := data["summary"].(map[string]interface{})
	assert.Equal(t, float64(15000), summary["total_quota_consumed"])
}

// TestV2GetPlatformLogsCacheTokens verifies that cache_tokens stored in the
// per-log Other JSON (written by service/log_info_generate.go) is surfaced in
// each log item and aggregated into the summary. The LH side pulls this to
// expose prompt-cache hit counts in its usage reporting (customer-facing).
func TestV2GetPlatformLogsCacheTokens(t *testing.T) {
	router, pid, shadowUserId := setupV2TestRouter(t)

	token := &model.Token{
		UserId: shadowUserId, Key: "cachetesttoken111111111111111111111",
		Name: "v2_cachetest", Status: common.TokenStatusEnabled,
		CreatedTime: common.GetTimestamp(), ExpiredTime: -1, UnlimitedQuota: true,
	}
	require.NoError(t, model.DB.Create(token).Error)

	now := common.GetTimestamp()
	// One log with a cache hit (80 cached prompt tokens), one without.
	model.LOG_DB.Create(&model.Log{
		UserId: shadowUserId, Username: "platform_" + pid, TokenId: token.Id,
		CreatedAt: now, Type: model.LogTypeConsume, ModelName: "gemini-3.1-pro-preview",
		Quota: 3000, PromptTokens: 200, CompletionTokens: 100,
		Other: common.MapToJsonStr(map[string]interface{}{"cache_tokens": 80, "cache_ratio": 0.25}),
	})
	model.LOG_DB.Create(&model.Log{
		UserId: shadowUserId, Username: "platform_" + pid, TokenId: token.Id,
		CreatedAt: now - 100, Type: model.LogTypeConsume, ModelName: "gemini-3.1-pro-preview",
		Quota: 1000, PromptTokens: 50, CompletionTokens: 20,
	})

	w := doV2Request(router, "GET", fmtV2LogPath("page=1&page_size=50"), nil)
	assert.Equal(t, 200, w.Code)
	resp := parseV2Response(t, w)
	data := resp["data"].(map[string]interface{})
	logs := data["logs"].([]interface{})
	require.Len(t, logs, 2)

	// Logs are ordered created_at DESC → cache-hit log first.
	hit := logs[0].(map[string]interface{})
	assert.Equal(t, float64(80), hit["cache_tokens"], "cache_tokens must be surfaced from Other")
	miss := logs[1].(map[string]interface{})
	assert.Equal(t, float64(0), miss["cache_tokens"], "absent cache_tokens defaults to 0")

	summary := data["summary"].(map[string]interface{})
	assert.Equal(t, float64(80), summary["total_cache_tokens"])
}
