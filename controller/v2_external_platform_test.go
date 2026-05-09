package controller

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupV2TestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	testDB := setupTestDB()
	model.DB = testDB
	model.LOG_DB = testDB

	v2 := router.Group("/api/v2/external")
	{
		v2.POST("/tokens/authorize", V2AuthorizeToken)
		v2.GET("/platforms/:platform_id/logs", V2GetPlatformLogs)
	}

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
	router := setupV2TestRouter()

	t.Run("授权新Token成功", func(t *testing.T) {
		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"platform_id": "asd",
			"token_key":   "sk-a99416b67cb54e178e9ffe8a55c255ae",
		})
		assert.Equal(t, 200, w.Code)
		resp := parseV2Response(t, w)
		assert.Equal(t, true, resp["success"])
		assert.Equal(t, "密钥授权成功", resp["message"])

		data := resp["data"].(map[string]interface{})
		assert.Equal(t, "sk-a99416b67cb54e178e9ffe8a55c255ae", data["token_key"])
		assert.Equal(t, "authorized", data["status"])
		assert.Equal(t, "asd", data["platform_id"])

		// Verify platform user created with large quota
		var user model.User
		model.DB.Where("username = ?", "platform_asd").First(&user)
		assert.Equal(t, PlatformQuotaForNewUser, user.Quota)
		assert.True(t, user.IsExternal)

		// Verify token is unlimited
		var token model.Token
		model.DB.Where("user_id = ?", user.Id).First(&token)
		assert.True(t, token.UnlimitedQuota)
		assert.Equal(t, int64(-1), token.ExpiredTime)
	})

	t.Run("幂等性-重复授权返回exists", func(t *testing.T) {
		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"platform_id": "asd",
			"token_key":   "sk-a99416b67cb54e178e9ffe8a55c255ae",
		})
		assert.Equal(t, 200, w.Code)
		resp := parseV2Response(t, w)
		data := resp["data"].(map[string]interface{})
		assert.Equal(t, "exists", data["status"])
	})

	t.Run("幂等性-被禁用token重授权后自动恢复", func(t *testing.T) {
		// 先授权一个新 token
		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"platform_id": "asd",
			"token_key":   "sk-disabledtokenrecoverytest00000001",
		})
		require.Equal(t, 200, w.Code)

		// 手动禁用该 token
		model.DB.Model(&model.Token{}).
			Where("key = ?", "disabledtokenrecoverytest00000001").
			Updates(map[string]interface{}{"status": common.TokenStatusDisabled, "unlimited_quota": false})

		// 验证已禁用
		tk, err := model.GetTokenByKey("disabledtokenrecoverytest00000001", true)
		require.NoError(t, err)
		assert.Equal(t, common.TokenStatusDisabled, tk.Status)

		// 重复授权 — 应自动恢复为启用+无限额度
		w = doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"platform_id": "asd",
			"token_key":   "sk-disabledtokenrecoverytest00000001",
		})
		assert.Equal(t, 200, w.Code)
		resp := parseV2Response(t, w)
		assert.Equal(t, "exists", resp["data"].(map[string]interface{})["status"])

		// 验证 token 已恢复
		tk, err = model.GetTokenByKey("disabledtokenrecoverytest00000001", true)
		require.NoError(t, err)
		assert.Equal(t, common.TokenStatusEnabled, tk.Status)
		assert.True(t, tk.UnlimitedQuota)
	})

	t.Run("带metadata授权", func(t *testing.T) {
		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"platform_id": "asd",
			"token_key":   "sk-b12345678901234567890123456789ab",
			"metadata": map[string]interface{}{
				"platform_user_id": "alice_123",
				"user_type":        "premium",
			},
		})
		assert.Equal(t, 200, w.Code)
		resp := parseV2Response(t, w)
		assert.Equal(t, "authorized", resp["data"].(map[string]interface{})["status"])

		// Verify token name includes platform_user_id
		var token model.Token
		model.DB.Where("name = ?", "v2_asd_alice_123").First(&token)
		assert.True(t, token.Id > 0)
	})

	t.Run("token_key格式错误-无sk前缀", func(t *testing.T) {
		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"platform_id": "asd",
			"token_key":   "abc123def456789xyz0123456789",
		})
		assert.Equal(t, 400, w.Code)
	})

	t.Run("token_key格式错误-含额外短横线", func(t *testing.T) {
		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"platform_id": "asd",
			"token_key":   "sk-2-a99416b67cb54e178e9ffe8a55c255ae",
		})
		assert.Equal(t, 400, w.Code)
		resp := parseV2Response(t, w)
		assert.Contains(t, resp["message"], "不能包含")
	})

	t.Run("跨平台冲突", func(t *testing.T) {
		// First authorize on platform "other"
		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"platform_id": "other_platform",
			"token_key":   "sk-c11111111111111111111111111111111",
		})
		assert.Equal(t, 200, w.Code)

		// Try same token_key on platform "asd"
		w = doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"platform_id": "asd",
			"token_key":   "sk-c11111111111111111111111111111111",
		})
		assert.Equal(t, 409, w.Code)
		resp := parseV2Response(t, w)
		assert.Contains(t, resp["message"], "其他平台")
	})

	t.Run("平台用户复用", func(t *testing.T) {
		// Count users before
		var countBefore int64
		model.DB.Model(&model.User{}).Where("username = ?", "platform_asd").Count(&countBefore)

		// Authorize another token on same platform
		w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
			"platform_id": "asd",
			"token_key":   "sk-d22222222222222222222222222222222",
		})
		assert.Equal(t, 200, w.Code)

		// Count users after — should be same
		var countAfter int64
		model.DB.Model(&model.User{}).Where("username = ?", "platform_asd").Count(&countAfter)
		assert.Equal(t, countBefore, countAfter)
	})
}

// ==================== Platform Logs ====================

func TestV2GetPlatformLogs(t *testing.T) {
	router := setupV2TestRouter()

	// Setup: create platform user and tokens
	user := &model.User{
		Username:       "platform_logtest",
		Email:          "logtest@platform.local",
		ExternalUserId: ptrExternalUserId("platform_logtest"),
		IsExternal:     true,
		Quota:          PlatformQuotaForNewUser,
	}
	model.DB.Create(user)

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
	model.DB.Create(token1)
	model.DB.Create(token2)

	// Insert test logs
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
		w := doV2Request(router, "GET", "/api/v2/external/platforms/logtest/logs?page=1&page_size=50", nil)
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

		// Verify full token_key in logs
		firstLog := logs[0].(map[string]interface{})
		tokenKey := firstLog["token_key"].(string)
		assert.True(t, tokenKey == "sk-logtesttoken1aaaaaaaaaaaaaaaaaaaaa" || tokenKey == "sk-logtesttoken2bbbbbbbbbbbbbbbbbbbbb")
	})

	t.Run("按token_key筛选", func(t *testing.T) {
		w := doV2Request(router, "GET",
			fmt.Sprintf("/api/v2/external/platforms/logtest/logs?token_key=sk-%s", token1.Key), nil)
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
		w := doV2Request(router, "GET",
			"/api/v2/external/platforms/logtest/logs?token_key=sk-nonexistent", nil)
		assert.Equal(t, 200, w.Code)

		resp := parseV2Response(t, w)
		data := resp["data"].(map[string]interface{})
		logs := data["logs"].([]interface{})
		assert.Len(t, logs, 0)
	})

	t.Run("平台不存在", func(t *testing.T) {
		w := doV2Request(router, "GET", "/api/v2/external/platforms/nonexistent/logs", nil)
		assert.Equal(t, 404, w.Code)
	})

	t.Run("分页", func(t *testing.T) {
		w := doV2Request(router, "GET", "/api/v2/external/platforms/logtest/logs?page=1&page_size=3", nil)
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

// ==================== Integration: Authorize + Query ====================

func TestV2AuthorizeThenQueryLogs(t *testing.T) {
	router := setupV2TestRouter()

	// 1. Authorize a token
	w := doV2Request(router, "POST", "/api/v2/external/tokens/authorize", map[string]interface{}{
		"platform_id": "integ_test",
		"token_key":   "sk-e33333333333333333333333333333333",
		"metadata":    map[string]interface{}{"platform_user_id": "bob"},
	})
	assert.Equal(t, 200, w.Code)
	resp := parseV2Response(t, w)
	tokenId := int(resp["data"].(map[string]interface{})["token_id"].(float64))

	// 2. Simulate consumption logs
	var user model.User
	model.DB.Where("username = ?", "platform_integ_test").First(&user)

	for i := 0; i < 3; i++ {
		model.LOG_DB.Create(&model.Log{
			UserId: user.Id, Username: user.Username,
			TokenId: tokenId, CreatedAt: common.GetTimestamp(),
			Type: model.LogTypeConsume, ModelName: "claude-3-5-sonnet",
			Quota: 5000, PromptTokens: 500, CompletionTokens: 200,
		})
	}

	// 3. Query platform logs
	w = doV2Request(router, "GET", "/api/v2/external/platforms/integ_test/logs", nil)
	assert.Equal(t, 200, w.Code)

	resp = parseV2Response(t, w)
	data := resp["data"].(map[string]interface{})
	logs := data["logs"].([]interface{})
	assert.Len(t, logs, 3)

	// Verify full token key in response
	for _, l := range logs {
		logItem := l.(map[string]interface{})
		assert.Equal(t, "sk-e33333333333333333333333333333333", logItem["token_key"])
		assert.Equal(t, "claude-3-5-sonnet", logItem["model_name"])
	}

	summary := data["summary"].(map[string]interface{})
	assert.Equal(t, float64(15000), summary["total_quota_consumed"])
}
