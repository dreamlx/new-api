package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// setupV2TestRouter 设置V2 API测试路由（复用V1的setupTestDB）
func setupV2TestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	testDB := setupTestDB()
	model.DB = testDB
	model.LOG_DB = testDB

	// 注册V2路由（与 router/api-router.go 保持一致）
	v2Route := router.Group("/api/v2/external")
	{
		v2Route.POST("/tokens/authorize", V2TokenAuthorize)
		v2Route.GET("/platforms/:platform_id/logs", V2GetPlatformLogs)
	}

	return router
}

// --- V2TokenAuthorize Tests ---

func TestV2TokenAuthorize(t *testing.T) {
	router := setupV2TestRouter()

	t.Run("新Token授权成功", func(t *testing.T) {
		body := map[string]interface{}{
			"platform_id":   "test_platform",
			"token_key":     "sk-a99416b67cb54e178e9ffe8a55c255ae",
			"initial_quota": 500000,
			"metadata":      map[string]interface{}{"env": "test"},
		}
		jsonData, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", "/api/v2/external/tokens/authorize", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp V2TokenAuthorizeResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, "密钥授权成功", resp.Message)
		assert.Equal(t, "sk-a99416b67cb54e178e9ffe8a55c255ae", resp.Data.TokenKey)
		assert.Equal(t, 500000, resp.Data.CurrentQuota)
		assert.Equal(t, 1.0, resp.Data.QuotaUsd) // 500000 / 500000 = 1.0
		assert.Equal(t, "authorized", resp.Data.Status)
		assert.NotEmpty(t, resp.Data.CreatedAt)
		assert.NotZero(t, resp.Data.ProxyUserId)

		// 验证DB中Token存储（去掉sk-前缀）
		var token model.Token
		err = model.DB.Where("`key` = ?", "a99416b67cb54e178e9ffe8a55c255ae").First(&token).Error
		assert.NoError(t, err)
		assert.True(t, token.UnlimitedQuota)
		assert.Equal(t, 0, token.RemainQuota)

		// 验证平台用户已创建
		var user model.User
		err = model.DB.Where("username = ?", "platform_test_platform").First(&user).Error
		assert.NoError(t, err)
		assert.Equal(t, token.UserId, user.Id)
	})

	t.Run("同平台Token更新为无限额度", func(t *testing.T) {
		// 先手动创建平台用户和有限额度Token
		platformUser, _ := model.GetOrCreatePlatformUser("platform_update_test")
		existingToken := &model.Token{
			UserId:         platformUser.Id,
			Key:            "existingtoken12345678",
			Name:           "old-token",
			UnlimitedQuota: false,
			RemainQuota:    100000,
			Status:         1,
			CreatedTime:    time.Now().Unix(),
		}
		model.DB.Create(existingToken)

		body := map[string]interface{}{
			"platform_id":   "update_test",
			"token_key":     "sk-existingtoken12345678",
			"initial_quota": 1, // >0 required by gin binding
		}
		jsonData, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", "/api/v2/external/tokens/authorize", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp V2TokenAuthorizeResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "updated_unlimited", resp.Data.Status)
		assert.Equal(t, 100000, resp.Data.PreviousQuota)
		assert.Equal(t, 0, resp.Data.CurrentQuota) // 无限额度模式

		// 验证DB中Token已更新
		var updated model.Token
		model.DB.Where("id = ?", existingToken.Id).First(&updated)
		assert.True(t, updated.UnlimitedQuota)
		assert.Equal(t, 0, updated.RemainQuota)
	})

	t.Run("不同平台Token冲突409", func(t *testing.T) {
		// 先在平台A创建Token
		platformA, _ := model.GetOrCreatePlatformUser("platform_conflict_a")
		conflictToken := &model.Token{
			UserId:      platformA.Id,
			Key:         "conflicttoken12345678",
			Name:        "platform-a-token",
			Status:      1,
			CreatedTime: time.Now().Unix(),
		}
		model.DB.Create(conflictToken)

		// 平台B尝试授权同一个Token
		body := map[string]interface{}{
			"platform_id":   "conflict_b",
			"token_key":     "sk-conflicttoken12345678",
			"initial_quota": 1,
		}
		jsonData, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", "/api/v2/external/tokens/authorize", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, false, resp["success"])
		assert.Equal(t, "TOKEN_EXISTS", resp["error_code"])
	})
}

func TestV2TokenAuthorizeValidation(t *testing.T) {
	router := setupV2TestRouter()

	tests := []struct {
		name           string
		body           map[string]interface{}
		expectedStatus int
		expectedCode   string
	}{
		{
			name: "platform_id含特殊字符",
			body: map[string]interface{}{
				"platform_id":   "test@platform!",
				"token_key":     "sk-validtoken12345678",
				"initial_quota": 100,
			},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_PARAMETER",
		},
		{
			name: "token_key缺少sk-前缀",
			body: map[string]interface{}{
				"platform_id":   "valid_platform",
				"token_key":     "noprefix12345678",
				"initial_quota": 100,
			},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_PARAMETER",
		},
		{
			name: "token_key含多个短横线",
			body: map[string]interface{}{
				"platform_id":   "valid_platform",
				"token_key":     "sk-part1-part2-part3",
				"initial_quota": 100,
			},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_PARAMETER",
		},
		{
			name: "token_key太短",
			body: map[string]interface{}{
				"platform_id":   "valid_platform",
				"token_key":     "sk-short",
				"initial_quota": 100,
			},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_PARAMETER",
		},
		{
			name: "缺少必填字段platform_id",
			body: map[string]interface{}{
				"token_key":     "sk-validtoken12345678",
				"initial_quota": 100,
			},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_PARAMETER",
		},
		{
			name: "缺少必填字段token_key",
			body: map[string]interface{}{
				"platform_id":   "valid_platform",
				"initial_quota": 100,
			},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_PARAMETER",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.body)
			req, _ := http.NewRequest("POST", "/api/v2/external/tokens/authorize", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)
			assert.Equal(t, false, resp["success"])
			assert.Equal(t, tt.expectedCode, resp["error_code"])
		})
	}
}

// --- V2GetPlatformLogs Tests ---

func TestV2GetPlatformLogs(t *testing.T) {
	router := setupV2TestRouter()

	// 创建平台用户和测试Token
	platformUser, _ := model.GetOrCreatePlatformUser("platform_logtest")
	testToken := &model.Token{
		UserId:      platformUser.Id,
		Key:         "logtesttoken12345678",
		Name:        "log-test-token",
		Status:      1,
		CreatedTime: time.Now().Unix(),
	}
	model.DB.Create(testToken)

	// 创建消费日志
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)
	yesterday := today.AddDate(0, 0, -1)

	testLogs := []*model.Log{
		{
			UserId:           platformUser.Id,
			TokenId:          testToken.Id,
			TokenName:        "sk-logtesttoken12345678",
			CreatedAt:        today.Unix(),
			Type:             model.LogTypeConsume,
			Content:          "Chat completion",
			ModelName:        "gpt-4",
			Quota:            2000,
			PromptTokens:     100,
			CompletionTokens: 50,
		},
		{
			UserId:           platformUser.Id,
			TokenId:          testToken.Id,
			TokenName:        "sk-logtesttoken12345678",
			CreatedAt:        today.Unix() + 3600,
			Type:             model.LogTypeConsume,
			Content:          "Chat completion 2",
			ModelName:        "gpt-3.5-turbo",
			Quota:            500,
			PromptTokens:     200,
			CompletionTokens: 100,
		},
		{
			UserId:           platformUser.Id,
			TokenId:          testToken.Id,
			TokenName:        "sk-logtesttoken12345678",
			CreatedAt:        yesterday.Unix(),
			Type:             model.LogTypeConsume,
			Content:          "Yesterday request",
			ModelName:        "gpt-4",
			Quota:            1500,
			PromptTokens:     80,
			CompletionTokens: 40,
		},
	}
	for _, log := range testLogs {
		model.LOG_DB.Create(log)
	}

	todayStr := today.Format("2006-01-02")
	yesterdayStr := yesterday.Format("2006-01-02")

	t.Run("查询日期范围内全部日志", func(t *testing.T) {
		url := fmt.Sprintf("/api/v2/external/platforms/logtest/logs?start_date=%s&end_date=%s", yesterdayStr, todayStr)
		req, _ := http.NewRequest("GET", url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp V2PlatformLogResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, "logtest", resp.Data.PlatformId)
		assert.Equal(t, 3, resp.Data.Pagination.TotalItems)
		assert.Len(t, resp.Data.Logs, 3)

		// 验证Token Key包含sk-前缀
		for _, log := range resp.Data.Logs {
			assert.Contains(t, log.TokenKey, "sk-")
		}
	})

	t.Run("仅查询今天的日志", func(t *testing.T) {
		url := fmt.Sprintf("/api/v2/external/platforms/logtest/logs?start_date=%s&end_date=%s", todayStr, todayStr)
		req, _ := http.NewRequest("GET", url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp V2PlatformLogResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.True(t, resp.Success)
		assert.Equal(t, 2, resp.Data.Pagination.TotalItems)
	})

	t.Run("分页查询", func(t *testing.T) {
		url := fmt.Sprintf("/api/v2/external/platforms/logtest/logs?start_date=%s&end_date=%s&page=1&page_size=2", yesterdayStr, todayStr)
		req, _ := http.NewRequest("GET", url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp V2PlatformLogResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.True(t, resp.Success)
		assert.Len(t, resp.Data.Logs, 2)
		assert.Equal(t, 3, resp.Data.Pagination.TotalItems)
		assert.Equal(t, 2, resp.Data.Pagination.TotalPages)
		assert.True(t, resp.Data.Pagination.HasNext)
		assert.False(t, resp.Data.Pagination.HasPrev)

		// 第2页
		url = fmt.Sprintf("/api/v2/external/platforms/logtest/logs?start_date=%s&end_date=%s&page=2&page_size=2", yesterdayStr, todayStr)
		req, _ = http.NewRequest("GET", url, nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Len(t, resp.Data.Logs, 1)
		assert.False(t, resp.Data.Pagination.HasNext)
		assert.True(t, resp.Data.Pagination.HasPrev)
	})

	t.Run("无日志的日期范围", func(t *testing.T) {
		url := "/api/v2/external/platforms/logtest/logs?start_date=2020-01-01&end_date=2020-01-31"
		req, _ := http.NewRequest("GET", url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp V2PlatformLogResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.True(t, resp.Success)
		assert.Equal(t, 0, resp.Data.Pagination.TotalItems)
		assert.Len(t, resp.Data.Logs, 0)
		assert.NotNil(t, resp.Data.Logs) // 空数组而非nil
	})
}

func TestV2GetPlatformLogsValidation(t *testing.T) {
	router := setupV2TestRouter()

	tests := []struct {
		name           string
		url            string
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "缺少start_date",
			url:            "/api/v2/external/platforms/test_plat/logs?end_date=2026-01-01",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "MISSING_PARAMETER",
		},
		{
			name:           "缺少end_date",
			url:            "/api/v2/external/platforms/test_plat/logs?start_date=2026-01-01",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "MISSING_PARAMETER",
		},
		{
			name:           "start_date格式错误",
			url:            "/api/v2/external/platforms/test_plat/logs?start_date=01-2026-01&end_date=2026-01-31",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_PARAMETER",
		},
		{
			name:           "end_date格式错误",
			url:            "/api/v2/external/platforms/test_plat/logs?start_date=2026-01-01&end_date=bad-date",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_PARAMETER",
		},
		{
			name:           "platform_id格式无效",
			url:            "/api/v2/external/platforms/bad@id!/logs?start_date=2026-01-01&end_date=2026-01-31",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_PARAMETER",
		},
		{
			name:           "平台不存在",
			url:            "/api/v2/external/platforms/nonexistent_plat/logs?start_date=2026-01-01&end_date=2026-01-31",
			expectedStatus: http.StatusNotFound,
			expectedCode:   "PLATFORM_NOT_FOUND",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tt.url, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)
			assert.Equal(t, false, resp["success"])
			assert.Equal(t, tt.expectedCode, resp["error_code"])
		})
	}
}

// --- Validation Helper Tests ---

func TestIsValidPlatformId(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"valid_platform", true},
		{"platform-123", true},
		{"Platform_A", true},
		{"a", true},
		{"", false},
		{"bad@id", false},
		{"has space", false},
		{"中文", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, isValidPlatformId(tt.input))
		})
	}
}

func TestIsValidTokenKey(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"sk-a99416b67cb54e178e9ffe8a55c255ae", true},
		{"sk-12345678", true},
		{"sk-abcdefghijk", true},
		{"noprefix12345678", false},     // 缺少sk-
		{"sk-short", false},             // 太短（<8 chars after sk-）
		{"sk-part1-part2", false},       // 含短横线
		{"sk-", false},                  // 空内容
		{"", false},                     // 空字符串
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, isValidTokenKey(tt.input))
		})
	}
}
