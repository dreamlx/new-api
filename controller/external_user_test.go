package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTestDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	db.AutoMigrate(&model.User{}, &model.Token{}, &model.TopUp{}, &model.Log{})

	// Create channels table
	db.Exec(`CREATE TABLE IF NOT EXISTS channels (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name VARCHAR(191), type INTEGER, status INTEGER DEFAULT 1,
		models TEXT, test_model VARCHAR(191), created_time INTEGER, other TEXT
	)`)
	db.Exec(`INSERT INTO channels (name, type, status, models, test_model, created_time)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"test_ds", 43, 1,
		`deepseek-chat,deepseek-reasoner`,
		"deepseek-chat", 1640995200)

	ratio_setting.InitRatioSettings()
	model.InitColumnsForTest()
	common.RedisEnabled = false

	return db
}

func ptrExternalUserId(externalUserId string) *string {
	return &externalUserId
}

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	testDB := setupTestDB()
	model.DB = testDB
	model.LOG_DB = testDB

	api := router.Group("/api")
	ext := api.Group("/user/external")
	{
		ext.POST("/sync", SyncExternalUser)
		ext.POST("/topup", ExternalUserTopUp)
		ext.POST("/deduct", ExternalUserDeduct)
		ext.POST("/token", CreateExternalUserToken)
		ext.DELETE("/:external_user_id/token/:token_id", DeleteExternalUserToken)
		ext.GET("/:external_user_id/tokens", GetExternalUserTokens)
		ext.POST("/token/verify", VerifyExternalUserToken)
		ext.GET("/:external_user_id/stats", GetExternalUserStats)
		ext.GET("/:external_user_id/logs", GetExternalUserLogs)
		ext.GET("/models", GetExternalUserModels)
		ext.DELETE("/:external_user_id", DeleteExternalUser)
	}

	return router
}

func doRequest(router *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	var buf *bytes.Buffer
	if body != nil {
		jsonData, _ := json.Marshal(body)
		buf = bytes.NewBuffer(jsonData)
	} else {
		buf = &bytes.Buffer{}
	}
	req, _ := http.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func parseResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	return resp
}

// ==================== Sync ====================

func TestSyncExternalUser(t *testing.T) {
	router := setupTestRouter()

	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "创建新用户成功",
			body:       map[string]interface{}{"external_user_id": "test_001", "username": "testuser", "email": "test@example.com"},
			wantStatus: 200,
			wantMsg:    "用户创建成功",
		},
		{
			name:       "更新现有用户",
			body:       map[string]interface{}{"external_user_id": "test_001", "username": "updated", "email": "updated@example.com"},
			wantStatus: 200,
			wantMsg:    "用户信息同步成功",
		},
		{
			name:       "缺少必需字段",
			body:       map[string]interface{}{"username": "testuser"},
			wantStatus: 400,
			wantMsg:    "ExternalUserId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doRequest(router, "POST", "/api/user/external/sync", tt.body)
			assert.Equal(t, tt.wantStatus, w.Code)
			resp := parseResponse(t, w)
			if tt.wantStatus == 200 {
				assert.Equal(t, true, resp["success"])
				assert.Equal(t, tt.wantMsg, resp["message"])
			} else {
				assert.Contains(t, resp["message"], tt.wantMsg)
			}
		})
	}
}

// ==================== TopUp ====================

func TestTopupExternalUser(t *testing.T) {
	router := setupTestRouter()

	model.DB.Create(&model.User{
		Username: "topup_user", Email: "topup@test.com",
		ExternalUserId: ptrExternalUserId("topup_001"), IsExternal: true, Quota: 100000,
	})

	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "充值成功",
			body:       map[string]interface{}{"external_user_id": "topup_001", "amount_usd": 10.0, "payment_id": "pay_123"},
			wantStatus: 200,
			wantMsg:    "充值成功",
		},
		{
			name:       "用户不存在",
			body:       map[string]interface{}{"external_user_id": "nonexistent", "amount_usd": 10.0, "payment_id": "pay_456"},
			wantStatus: 404,
			wantMsg:    "用户不存在",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doRequest(router, "POST", "/api/user/external/topup", tt.body)
			assert.Equal(t, tt.wantStatus, w.Code)
			resp := parseResponse(t, w)
			if tt.wantStatus == 200 {
				assert.Equal(t, true, resp["success"])
				data := resp["data"].(map[string]interface{})
				assert.Equal(t, 10.0, data["amount_usd"])
				assert.Equal(t, float64(5000000), data["quota_added"])
			}
		})
	}
}

// ==================== Deduct ====================

func TestExternalUserDeduct(t *testing.T) {
	router := setupTestRouter()

	model.DB.Create(&model.User{
		Username: "deduct_user", Email: "deduct@test.com",
		ExternalUserId: ptrExternalUserId("deduct_001"), IsExternal: true, Quota: 10000000, // $20
	})

	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
		checkFunc  func(t *testing.T, resp map[string]interface{})
	}{
		{
			name: "扣除成功",
			body: map[string]interface{}{
				"external_user_id": "deduct_001", "amount_usd": 5.0,
				"reason": "测试扣除", "admin_note": "test",
			},
			wantStatus: 200,
			checkFunc: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, true, resp["success"])
				data := resp["data"].(map[string]interface{})
				assert.Equal(t, float64(2500000), data["quota_deducted"])
			},
		},
		{
			name: "余额不足",
			body: map[string]interface{}{
				"external_user_id": "deduct_001", "amount_usd": 100.0,
				"reason": "大额扣除",
			},
			wantStatus: 400,
			checkFunc: func(t *testing.T, resp map[string]interface{}) {
				assert.Contains(t, resp["message"], "余额不足")
			},
		},
		{
			name:       "用户不存在",
			body:       map[string]interface{}{"external_user_id": "nonexistent", "amount_usd": 1.0, "reason": "test"},
			wantStatus: 404,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doRequest(router, "POST", "/api/user/external/deduct", tt.body)
			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.checkFunc != nil {
				tt.checkFunc(t, parseResponse(t, w))
			}
		})
	}
}

// ==================== Token Create ====================

func TestCreateExternalUserToken(t *testing.T) {
	router := setupTestRouter()

	model.DB.Create(&model.User{
		Username: "token_user", Email: "token@test.com",
		ExternalUserId: ptrExternalUserId("token_001"), IsExternal: true, Quota: 20000000, // $40
	})

	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
		checkFunc  func(t *testing.T, resp map[string]interface{})
	}{
		{
			name: "创建独立额度Token",
			body: map[string]interface{}{
				"external_user_id": "token_001",
				"token_name":       "Test Token",
				"allocated_quota":  10000000,
			},
			wantStatus: 200,
			checkFunc: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, true, resp["success"])
				data := resp["data"].(map[string]interface{})
				assert.Contains(t, data["access_key"], "sk-")
				assert.Equal(t, float64(10000000), data["remain_quota"])
			},
		},
		{
			name: "余额不足",
			body: map[string]interface{}{
				"external_user_id": "token_001",
				"token_name":       "Too Expensive",
				"allocated_quota":  50000000,
			},
			wantStatus: 400,
			checkFunc: func(t *testing.T, resp map[string]interface{}) {
				assert.Contains(t, resp["message"], "余额不足")
			},
		},
		{
			name: "带Callback的Token",
			body: map[string]interface{}{
				"external_user_id": "token_001",
				"token_name":       "Callback Token",
				"allocated_quota":  5000000,
				"callback_url":     "https://example.com/notify",
				"callback_enabled": true,
			},
			wantStatus: 200,
			checkFunc: func(t *testing.T, resp map[string]interface{}) {
				data := resp["data"].(map[string]interface{})
				assert.Equal(t, true, data["callback_enabled"])
				assert.Equal(t, "https://example.com/***", data["callback_url_masked"])
			},
		},
		{
			name:       "用户不存在",
			body:       map[string]interface{}{"external_user_id": "nonexistent", "token_name": "T", "allocated_quota": 100},
			wantStatus: 404,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doRequest(router, "POST", "/api/user/external/token", tt.body)
			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.checkFunc != nil {
				tt.checkFunc(t, parseResponse(t, w))
			}
		})
	}
}

// ==================== Token Delete ====================

func TestDeleteExternalUserToken(t *testing.T) {
	router := setupTestRouter()

	user := &model.User{
		Username: "del_token_user", Email: "del_token@test.com",
		ExternalUserId: ptrExternalUserId("del_token_001"), IsExternal: true,
	}
	model.DB.Create(user)

	token := &model.Token{
		UserId: user.Id, Key: common.GetRandomString(32),
		Name: "Token to Delete", Status: common.TokenStatusEnabled,
		CreatedTime: common.GetTimestamp(), ExpiredTime: -1,
	}
	model.DB.Create(token)

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{
			name:       "删除成功",
			path:       fmt.Sprintf("/api/user/external/del_token_001/token/%d", token.Id),
			wantStatus: 200,
		},
		{
			name:       "Token不存在",
			path:       "/api/user/external/del_token_001/token/99999",
			wantStatus: 404,
		},
		{
			name:       "用户不存在",
			path:       "/api/user/external/nonexistent/token/1",
			wantStatus: 404,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doRequest(router, "DELETE", tt.path, nil)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

// ==================== Token List ====================

func TestGetExternalUserTokens(t *testing.T) {
	router := setupTestRouter()

	user := &model.User{
		Username: "list_user", Email: "list@test.com",
		ExternalUserId: ptrExternalUserId("list_001"), IsExternal: true,
	}
	model.DB.Create(user)

	for i := 0; i < 3; i++ {
		model.DB.Create(&model.Token{
			UserId: user.Id, Key: common.GetRandomString(32),
			Name: fmt.Sprintf("Token %d", i), Status: common.TokenStatusEnabled,
			CreatedTime: common.GetTimestamp(), ExpiredTime: -1, RemainQuota: 1000000,
		})
	}

	w := doRequest(router, "GET", "/api/user/external/list_001/tokens", nil)
	assert.Equal(t, 200, w.Code)

	resp := parseResponse(t, w)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, float64(3), data["total_tokens"])

	tokens := data["tokens"].([]interface{})
	assert.Len(t, tokens, 3)

	// Check masking
	firstToken := tokens[0].(map[string]interface{})
	assert.Contains(t, firstToken["access_key"], "sk-")
	assert.Contains(t, firstToken["access_key"].(string), "****")
}

// ==================== Token Verify ====================

func TestVerifyExternalUserToken(t *testing.T) {
	router := setupTestRouter()

	user := &model.User{
		Username: "verify_user", Email: "verify@test.com",
		ExternalUserId: ptrExternalUserId("verify_001"), IsExternal: true,
	}
	model.DB.Create(user)

	validToken := &model.Token{
		UserId: user.Id, Key: common.GetRandomString(32),
		Name: "Valid Token", Status: common.TokenStatusEnabled,
		CreatedTime: common.GetTimestamp(),
		ExpiredTime: common.GetTimestamp() + 86400,
		RemainQuota: 1000000,
	}
	model.DB.Create(validToken)

	disabledToken := &model.Token{
		UserId: user.Id, Key: common.GetRandomString(32),
		Name: "Disabled Token", Status: common.TokenStatusDisabled,
		CreatedTime: common.GetTimestamp(), ExpiredTime: -1,
	}
	model.DB.Create(disabledToken)

	exhaustedToken := &model.Token{
		UserId: user.Id, Key: common.GetRandomString(32),
		Name: "Exhausted Token", Status: common.TokenStatusEnabled,
		CreatedTime: common.GetTimestamp(), ExpiredTime: -1,
		RemainQuota: 0,
	}
	model.DB.Create(exhaustedToken)

	tests := []struct {
		name      string
		body      map[string]interface{}
		wantValid bool
		wantError string
	}{
		{
			name:      "有效Token（带sk-前缀）",
			body:      map[string]interface{}{"access_key": "sk-" + validToken.Key},
			wantValid: true,
		},
		{
			name:      "有效Token（不带前缀）",
			body:      map[string]interface{}{"access_key": validToken.Key},
			wantValid: true,
		},
		{
			name:      "Token不存在",
			body:      map[string]interface{}{"access_key": "sk-nonexistent"},
			wantValid: false,
			wantError: "Token不存在",
		},
		{
			name:      "Token已禁用",
			body:      map[string]interface{}{"access_key": "sk-" + disabledToken.Key},
			wantValid: false,
			wantError: "Token已被禁用",
		},
		{
			name:      "Token额度不足",
			body:      map[string]interface{}{"access_key": "sk-" + exhaustedToken.Key},
			wantValid: false,
			wantError: "Token剩余额度不足",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doRequest(router, "POST", "/api/user/external/token/verify", tt.body)
			assert.Equal(t, 200, w.Code)
			resp := parseResponse(t, w)
			data := resp["data"].(map[string]interface{})
			assert.Equal(t, tt.wantValid, data["is_valid"])
			if tt.wantError != "" {
				assert.Equal(t, tt.wantError, data["error_reason"])
			}
		})
	}
}

// ==================== Stats ====================

func TestGetExternalUserStats(t *testing.T) {
	router := setupTestRouter()

	model.DB.Create(&model.User{
		Username: "stats_user", Email: "stats@test.com",
		ExternalUserId: ptrExternalUserId("stats_001"), IsExternal: true, Quota: 5000000,
	})

	w := doRequest(router, "GET", "/api/user/external/stats_001/stats", nil)
	assert.Equal(t, 200, w.Code)

	resp := parseResponse(t, w)
	data := resp["data"].(map[string]interface{})
	userInfo := data["user_info"].(map[string]interface{})
	assert.Equal(t, "stats_001", userInfo["external_user_id"])
	assert.Equal(t, float64(5000000), userInfo["current_quota"])
	assert.Equal(t, 10.0, userInfo["current_balance"])
}

// ==================== Logs ====================

func TestGetExternalUserLogs(t *testing.T) {
	router := setupTestRouter()

	user := &model.User{
		Username: "logs_user", Email: "logs@test.com",
		ExternalUserId: ptrExternalUserId("logs_001"), IsExternal: true,
	}
	model.DB.Create(user)

	// Insert test logs
	for i := 0; i < 5; i++ {
		model.LOG_DB.Create(&model.Log{
			UserId: user.Id, Username: user.Username,
			CreatedAt: common.GetTimestamp() - int64(i*100),
			Type:      model.LogTypeConsume, ModelName: "deepseek-chat",
			Quota: 1000, PromptTokens: 100, CompletionTokens: 50,
		})
	}

	w := doRequest(router, "GET", "/api/user/external/logs_001/logs?page=1&page_size=10", nil)
	assert.Equal(t, 200, w.Code)

	resp := parseResponse(t, w)
	data := resp["data"].(map[string]interface{})
	logs := data["logs"].([]interface{})
	assert.Len(t, logs, 5)

	pagination := data["pagination"].(map[string]interface{})
	assert.Equal(t, float64(5), pagination["total"])
}

// ==================== Models ====================

func TestGetExternalUserModels(t *testing.T) {
	router := setupTestRouter()
	w := doRequest(router, "GET", "/api/user/external/models", nil)
	assert.Equal(t, 200, w.Code)

	resp := parseResponse(t, w)
	assert.Equal(t, true, resp["success"])
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, float64(common.QuotaPerUnit), data["quota_per_unit"])
}

// ==================== Delete User ====================

func TestDeleteExternalUser(t *testing.T) {
	router := setupTestRouter()

	user := &model.User{
		Username: "del_user", Email: "del@test.com",
		ExternalUserId: ptrExternalUserId("del_001"), IsExternal: true,
		Status: common.UserStatusEnabled,
	}
	model.DB.Create(user)

	// Create tokens
	for i := 0; i < 2; i++ {
		model.DB.Create(&model.Token{
			UserId: user.Id, Key: common.GetRandomString(32),
			Name: fmt.Sprintf("Token %d", i), Status: common.TokenStatusEnabled,
			CreatedTime: common.GetTimestamp(), ExpiredTime: -1,
		})
	}

	// Delete user
	w := doRequest(router, "DELETE", "/api/user/external/del_001", nil)
	assert.Equal(t, 200, w.Code)

	resp := parseResponse(t, w)
	assert.Equal(t, true, resp["success"])
	assert.Equal(t, "用户已注销", resp["message"])
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "del_001", data["original_external_user_id"])
	assert.Equal(t, float64(2), data["tokens_disabled"])
}

func TestDeleteExternalUserThenReregister(t *testing.T) {
	router := setupTestRouter()

	// Create user
	w := doRequest(router, "POST", "/api/user/external/sync", map[string]interface{}{
		"external_user_id": "reregister_001", "username": "reregister_user",
	})
	assert.Equal(t, 200, w.Code)

	// Delete user
	w = doRequest(router, "DELETE", "/api/user/external/reregister_001", nil)
	assert.Equal(t, 200, w.Code)

	// Re-register with same external_user_id
	w = doRequest(router, "POST", "/api/user/external/sync", map[string]interface{}{
		"external_user_id": "reregister_001", "username": "reregister_user_v2",
	})
	assert.Equal(t, 200, w.Code)
	resp := parseResponse(t, w)
	assert.Equal(t, "用户创建成功", resp["message"])
}

// ==================== TopUp Idempotency ====================

func TestTopupIdempotency(t *testing.T) {
	router := setupTestRouter()

	model.DB.Create(&model.User{
		Username: "idempotent_user", Email: "idempotent@test.com",
		ExternalUserId: ptrExternalUserId("idempotent_001"), IsExternal: true, Quota: 0,
	})

	body := map[string]interface{}{
		"external_user_id": "idempotent_001",
		"amount_usd":       10.0,
		"payment_id":       "pay_idempotent_001",
	}

	// First request should succeed
	w := doRequest(router, "POST", "/api/user/external/topup", body)
	assert.Equal(t, 200, w.Code)
	resp := parseResponse(t, w)
	assert.Equal(t, true, resp["success"])

	// Second request with same payment_id should be rejected
	w = doRequest(router, "POST", "/api/user/external/topup", body)
	assert.Equal(t, 409, w.Code)
	resp = parseResponse(t, w)
	assert.Contains(t, resp["message"], "已处理")

	// Verify quota was only added once
	var user model.User
	model.DB.Where("external_user_id = ?", "idempotent_001").First(&user)
	assert.Equal(t, int(10.0*common.QuotaPerUnit), user.Quota)
}

// ==================== Token Delete Refund ====================

func TestDeleteExternalUserTokenNoRefund(t *testing.T) {
	router := setupTestRouter()

	initialQuota := 20000000
	user := &model.User{
		Username: "norefund_user", Email: "norefund@test.com",
		ExternalUserId: ptrExternalUserId("norefund_001"), IsExternal: true, Quota: initialQuota,
	}
	model.DB.Create(user)

	allocatedQuota := 5000000

	// Create token via API (deducts from user)
	w := doRequest(router, "POST", "/api/user/external/token", map[string]interface{}{
		"external_user_id": "norefund_001",
		"token_name":       "Non-Refundable Token",
		"allocated_quota":  allocatedQuota,
	})
	assert.Equal(t, 200, w.Code)
	resp := parseResponse(t, w)
	data := resp["data"].(map[string]interface{})
	tokenId := int(data["token_id"].(float64))

	// Verify user quota was deducted
	model.DB.First(user, user.Id)
	assert.Equal(t, initialQuota-allocatedQuota, user.Quota)

	// Delete token — NO refund (remaining quota stays consumed)
	w = doRequest(router, "DELETE", fmt.Sprintf("/api/user/external/norefund_001/token/%d", tokenId), nil)
	assert.Equal(t, 200, w.Code)
	resp = parseResponse(t, w)
	data = resp["data"].(map[string]interface{})
	assert.Nil(t, data["refunded_quota"]) // no refund field in response
	assert.Equal(t, float64(tokenId), data["token_id"])

	// Verify user quota was NOT restored
	model.DB.First(user, user.Id)
	assert.Equal(t, initialQuota-allocatedQuota, user.Quota)
}

// ==================== Token with ModelLimits ====================

func TestCreateExternalUserTokenWithModelLimits(t *testing.T) {
	router := setupTestRouter()

	model.DB.Create(&model.User{
		Username: "ml_user", Email: "ml@test.com",
		ExternalUserId: ptrExternalUserId("ml_001"), IsExternal: true, Quota: 20000000,
	})

	t.Run("带模型限制", func(t *testing.T) {
		w := doRequest(router, "POST", "/api/user/external/token", map[string]interface{}{
			"external_user_id": "ml_001",
			"token_name":       "Limited Token",
			"allocated_quota":  5000000,
			"model_limits":     []string{"deepseek-chat", "deepseek-reasoner"},
		})
		assert.Equal(t, 200, w.Code)
		resp := parseResponse(t, w)
		data := resp["data"].(map[string]interface{})
		assert.NotNil(t, data["model_limits"])
		models := data["model_limits"].([]interface{})
		assert.Len(t, models, 2)

		// Verify in DB
		var token model.Token
		model.DB.Where("name = ?", "Limited Token").First(&token)
		assert.True(t, token.ModelLimitsEnabled)
		assert.Equal(t, "deepseek-chat,deepseek-reasoner", token.ModelLimits)
	})

	t.Run("不带模型限制", func(t *testing.T) {
		w := doRequest(router, "POST", "/api/user/external/token", map[string]interface{}{
			"external_user_id": "ml_001",
			"token_name":       "Unlimited Models Token",
			"allocated_quota":  5000000,
		})
		assert.Equal(t, 200, w.Code)
		resp := parseResponse(t, w)
		data := resp["data"].(map[string]interface{})
		assert.Nil(t, data["model_limits"])

		var token model.Token
		model.DB.Where("name = ?", "Unlimited Models Token").First(&token)
		assert.False(t, token.ModelLimitsEnabled)
		assert.Equal(t, "", token.ModelLimits)
	})
}

// ==================== Token with Group ====================

func TestCreateExternalUserTokenWithGroup(t *testing.T) {
	router := setupTestRouter()

	model.DB.Create(&model.User{
		Username: "grp_user", Email: "grp@test.com",
		ExternalUserId: ptrExternalUserId("grp_001"), IsExternal: true, Quota: 20000000,
	})

	t.Run("指定有效分组", func(t *testing.T) {
		w := doRequest(router, "POST", "/api/user/external/token", map[string]interface{}{
			"external_user_id": "grp_001",
			"token_name":       "VIP Token",
			"allocated_quota":  5000000,
			"group":            "default",
		})
		assert.Equal(t, 200, w.Code)
		resp := parseResponse(t, w)
		data := resp["data"].(map[string]interface{})
		assert.Equal(t, "default", data["group"])

		var token model.Token
		model.DB.Where("name = ?", "VIP Token").First(&token)
		assert.Equal(t, "default", token.Group)
	})

	t.Run("指定无效分组", func(t *testing.T) {
		w := doRequest(router, "POST", "/api/user/external/token", map[string]interface{}{
			"external_user_id": "grp_001",
			"token_name":       "Bad Group Token",
			"allocated_quota":  5000000,
			"group":            "nonexistent_group",
		})
		assert.Equal(t, 400, w.Code)
		resp := parseResponse(t, w)
		assert.Contains(t, resp["message"], "不存在")
	})
}

// ==================== Models with Group ====================

func TestGetExternalUserModelsWithGroup(t *testing.T) {
	router := setupTestRouter()

	t.Run("默认分组", func(t *testing.T) {
		w := doRequest(router, "GET", "/api/user/external/models", nil)
		assert.Equal(t, 200, w.Code)
		resp := parseResponse(t, w)
		data := resp["data"].(map[string]interface{})
		assert.Equal(t, "default", data["group"])
		assert.Equal(t, 1.0, data["group_ratio"])
	})

	t.Run("指定分组", func(t *testing.T) {
		w := doRequest(router, "GET", "/api/user/external/models?group=vip", nil)
		assert.Equal(t, 200, w.Code)
		resp := parseResponse(t, w)
		data := resp["data"].(map[string]interface{})
		assert.Equal(t, "vip", data["group"])
	})
}

// ==================== Unlimited Quota Token ====================

func TestCreateExternalUserTokenUnlimitedQuota(t *testing.T) {
	router := setupTestRouter()

	initialQuota := 10000000
	model.DB.Create(&model.User{
		Username: "unl_user", Email: "unl@test.com",
		ExternalUserId: ptrExternalUserId("unl_001"), IsExternal: true, Quota: initialQuota,
	})

	t.Run("无限额度模式-不扣用户余额", func(t *testing.T) {
		w := doRequest(router, "POST", "/api/user/external/token", map[string]interface{}{
			"external_user_id": "unl_001",
			"token_name":       "Unlimited Token",
			"unlimited_quota":  true,
		})
		assert.Equal(t, 200, w.Code)
		resp := parseResponse(t, w)
		assert.Equal(t, true, resp["success"])
		data := resp["data"].(map[string]interface{})
		assert.Equal(t, true, data["unlimited_quota"])

		// User quota must NOT be deducted
		var user model.User
		model.DB.Where("external_user_id = ?", "unl_001").First(&user)
		assert.Equal(t, initialQuota, user.Quota)

		// Token must have UnlimitedQuota=true
		var token model.Token
		model.DB.Where("name = ?", "Unlimited Token").First(&token)
		assert.True(t, token.UnlimitedQuota)
		assert.Equal(t, 0, token.RemainQuota)
	})

	t.Run("独立额度模式-缺少allocated_quota报错", func(t *testing.T) {
		w := doRequest(router, "POST", "/api/user/external/token", map[string]interface{}{
			"external_user_id": "unl_001",
			"token_name":       "Bad Token",
			// unlimited_quota=false (default), allocated_quota missing
		})
		assert.Equal(t, 400, w.Code)
		resp := parseResponse(t, w)
		assert.Contains(t, resp["message"], "allocated_quota")
	})
}
