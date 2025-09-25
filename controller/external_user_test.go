package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"one-api/common"
	"one-api/model"
	"one-api/setting/ratio_setting"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 测试数据库设置
func setupTestDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	// 自动迁移表结构
	db.AutoMigrate(&model.User{}, &model.Token{}, &model.TopUp{}, &model.Log{})
	
	// 创建channels表（模拟真实的渠道表结构）
	db.Exec(`CREATE TABLE IF NOT EXISTS channels (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name VARCHAR(191),
		type INTEGER,
		status INTEGER DEFAULT 1,
		models TEXT,
		test_model VARCHAR(191),
		created_time INTEGER,
		other TEXT
	)`)
	
	// 插入测试渠道数据（基于你的实际配置）
	db.Exec(`INSERT INTO channels (name, type, status, models, test_model, created_time) VALUES (?, ?, ?, ?, ?, ?)`,
		"test_ds", 43, 1, 
		`deepseek-chat,deepseek-reasoner,deepseek-coder,gpt-3.5-turbo,gpt-4,claude-3-haiku-20240307`,
		"deepseek-chat", // 设置默认测试模型
		1640995200) // 2022-01-01 的时间戳
	
	// 初始化ratio setting
	ratio_setting.InitRatioSettings()
	
	return db
}

// 设置测试路由
func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	// 设置测试数据库
	testDB := setupTestDB()
	model.DB = testDB
	model.LOG_DB = testDB
	
	// 注册外部用户路由
	api := router.Group("/api")
	{
		externalUser := api.Group("/user/external")
		{
			externalUser.POST("/sync", SyncExternalUser)
			externalUser.POST("/topup", ExternalUserTopUp)
			externalUser.POST("/token", CreateExternalUserToken)
			externalUser.DELETE("/token", DeleteExternalUserToken)
			externalUser.GET("/:external_user_id/stats", GetExternalUserStats)
			externalUser.GET("/:external_user_id/logs", GetExternalUserLogs)
		}
	}
	
	return router
}

// 测试用户同步API
func TestSyncExternalUser(t *testing.T) {
	router := setupTestRouter()

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
		expectedMsg    string
	}{
		{
			name: "创建新用户成功",
			requestBody: map[string]interface{}{
				"external_user_id": "test_user_001",
				"username":         "testuser",
				"email":           "test@example.com",
				"phone":           "13800138000",
				"login_type":      "email",
			},
			expectedStatus: 200,
			expectedMsg:    "用户创建成功",
		},
		{
			name: "更新现有用户成功",
			requestBody: map[string]interface{}{
				"external_user_id": "test_user_001",
				"username":         "updateduser",
				"email":           "updated@example.com",
				"phone":           "13900139000",
				"login_type":      "sms",
			},
			expectedStatus: 200,
			expectedMsg:    "用户信息同步成功",
		},
		{
			name: "缺少必需字段",
			requestBody: map[string]interface{}{
				"username": "testuser",
				"email":    "test@example.com",
			},
			expectedStatus: 400,
			expectedMsg:    "ExternalUserId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.requestBody)
			req, _ := http.NewRequest("POST", "/api/user/external/sync", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedStatus == 200 {
				assert.Equal(t, true, response["success"])
				assert.Equal(t, tt.expectedMsg, response["message"])
				assert.NotNil(t, response["data"])
			} else {
				assert.Equal(t, false, response["success"])
				assert.Contains(t, response["message"], tt.expectedMsg)
			}
		})
	}
}

// 测试用户充值API
func TestTopupExternalUser(t *testing.T) {
	router := setupTestRouter()

	// 先创建一个测试用户
	user := &model.User{
		Username:       "testuser",
		Email:          "test@example.com",
		ExternalUserId: "test_user_topup",
		IsExternal:     true,
		Quota:          100000, // 初始quota
	}
	model.DB.Create(user)

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
		expectedMsg    string
	}{
		{
			name: "充值成功",
			requestBody: map[string]interface{}{
				"external_user_id": "test_user_topup",
				"amount_usd":       10.0,
				"payment_id":       "stripe_payment_123",
			},
			expectedStatus: 200,
			expectedMsg:    "充值成功",
		},
		{
			name: "用户不存在",
			requestBody: map[string]interface{}{
				"external_user_id": "nonexistent_user",
				"amount_usd":       10.0,
				"payment_id":       "stripe_payment_456",
			},
			expectedStatus: 404,
			expectedMsg:    "用户不存在",
		},
		{
			name: "无效金额",
			requestBody: map[string]interface{}{
				"external_user_id": "test_user_topup",
				"amount_usd":       -5.0,
				"payment_id":       "stripe_payment_789",
			},
			expectedStatus: 400,
			expectedMsg:    "min",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.requestBody)
			req, _ := http.NewRequest("POST", "/api/user/external/topup", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedStatus == 200 {
				assert.Equal(t, true, response["success"])
				assert.Equal(t, tt.expectedMsg, response["message"])
				
				// 验证用户quota是否正确增加
				var updatedUser model.User
				model.DB.Where("external_user_id = ?", "test_user_topup").First(&updatedUser)
				expectedQuota := 100000 + int(10.0*float64(common.QuotaPerUnit))
				assert.Equal(t, expectedQuota, updatedUser.Quota)
			} else {
				assert.Equal(t, false, response["success"])
				assert.Contains(t, response["message"], tt.expectedMsg)
			}
		})
	}
}

// 测试Token创建API
func TestCreateExternalUserToken(t *testing.T) {
	router := setupTestRouter()

	// 先创建一个测试用户
	user := &model.User{
		Username:       "testuser",
		Email:          "test@example.com",
		ExternalUserId: "test_user_token",
		IsExternal:     true,
		Quota:          500000,
	}
	model.DB.Create(user)

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
		expectedMsg    string
	}{
		{
			name: "创建Token成功",
			requestBody: map[string]interface{}{
				"external_user_id": "test_user_token",
				"token_name":       "Test Token",
				"expires_in_days":  365,
			},
			expectedStatus: 200,
			expectedMsg:    "Token创建成功",
		},
		{
			name: "用户不存在",
			requestBody: map[string]interface{}{
				"external_user_id": "nonexistent_user",
				"token_name":       "Test Token",
				"expires_in_days":  365,
			},
			expectedStatus: 404,
			expectedMsg:    "用户不存在",
		},
		{
			name: "缺少Token名称",
			requestBody: map[string]interface{}{
				"external_user_id": "test_user_token",
				"expires_in_days":  365,
			},
			expectedStatus: 400,
			expectedMsg:    "TokenName",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.requestBody)
			req, _ := http.NewRequest("POST", "/api/user/external/token", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedStatus == 200 {
				assert.Equal(t, true, response["success"])
				assert.Equal(t, tt.expectedMsg, response["message"])
				if response["data"] != nil {
					data := response["data"].(map[string]interface{})
					assert.NotEmpty(t, data["access_key"])
					assert.Equal(t, "Test Token", data["token_name"])
				}
			} else {
				assert.Equal(t, false, response["success"])
				assert.Contains(t, response["message"], tt.expectedMsg)
			}
		})
	}
}

// 测试消费记录查询API
func TestGetExternalUserLogs(t *testing.T) {
	// 设置测试数据库
	testDB := setupTestDB()
	model.DB = testDB
	model.LOG_DB = testDB // 日志使用同一个数据库

	// 创建测试路由
	router := gin.New()
	router.GET("/api/user/external/:external_user_id/logs", GetExternalUserLogs)

	// 创建测试用户
	testUser := &model.User{
		Id:             999,
		Username:       "logtest",
		ExternalUserId: "log_test_user",
		Email:          "logtest@external.local",
		Quota:          1000000,
		IsExternal:     true,
	}
	if err := model.DB.Create(testUser).Error; err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}

	// 创建测试日志记录
	testLogs := []*model.Log{
		{
			UserId:           testUser.Id,
			Username:         testUser.Username,
			CreatedAt:        1640995200, // 2022-01-01 00:00:00
			Type:             model.LogTypeConsume,
			Content:          "Chat completion request",
			ModelName:        "qwen-turbo",
			Quota:            1000,
			PromptTokens:     50,
			CompletionTokens: 30,
		},
		{
			UserId:           testUser.Id,
			Username:         testUser.Username,
			CreatedAt:        1641081600, // 2022-01-02 00:00:00
			Type:             model.LogTypeTopup,
			Content:          "User topup",
			ModelName:        "",
			Quota:            500000, // $1.00
			PromptTokens:     0,
			CompletionTokens: 0,
		},
	}

	for _, log := range testLogs {
		if err := model.LOG_DB.Create(log).Error; err != nil {
			t.Fatalf("创建测试日志失败: %v", err)
		}
	}

	tests := []struct {
		name           string
		externalUserId string
		queryParams    string
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:           "查询所有记录",
			externalUserId: "log_test_user",
			queryParams:    "",
			expectedStatus: 200,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response ExternalUserLogsResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.True(t, response.Success)
				assert.Len(t, response.Data.Logs, 2)
				assert.Equal(t, 1, response.Data.Pagination.Page)
				assert.Equal(t, 20, response.Data.Pagination.PageSize)
				assert.Equal(t, int64(2), response.Data.Pagination.Total)
			},
		},
		{
			name:           "按日期筛选",
			externalUserId: "log_test_user",
			queryParams:    "?start_date=2022-01-01&end_date=2022-01-01",
			expectedStatus: 200,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response ExternalUserLogsResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.True(t, response.Success)
				assert.Len(t, response.Data.Logs, 1)
				assert.Equal(t, "qwen-turbo", response.Data.Logs[0].Model)
			},
		},
		{
			name:           "按模型筛选",
			externalUserId: "log_test_user",
			queryParams:    "?model_name=qwen",
			expectedStatus: 200,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response ExternalUserLogsResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.True(t, response.Success)
				assert.Len(t, response.Data.Logs, 1)
				assert.Equal(t, "consume", response.Data.Logs[0].Type)
			},
		},
		{
			name:           "分页测试",
			externalUserId: "log_test_user",
			queryParams:    "?page=1&page_size=1",
			expectedStatus: 200,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response ExternalUserLogsResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.True(t, response.Success)
				assert.Len(t, response.Data.Logs, 1)
				assert.Equal(t, 1, response.Data.Pagination.Page)
				assert.Equal(t, 1, response.Data.Pagination.PageSize)
				assert.Equal(t, 2, response.Data.Pagination.TotalPage)
			},
		},
		{
			name:           "用户不存在",
			externalUserId: "nonexistent_user",
			queryParams:    "",
			expectedStatus: 404,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.False(t, response["success"].(bool))
				assert.Equal(t, "用户不存在", response["message"].(string))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/user/external/"+tt.externalUserId+"/logs"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.checkResponse != nil {
				tt.checkResponse(t, w)
			}
		})
	}

	// 清理测试数据
	model.LOG_DB.Where("user_id = ?", testUser.Id).Delete(&model.Log{})
	model.DB.Delete(testUser)
}

// 测试用户统计API
func TestGetExternalUserStats(t *testing.T) {
	router := setupTestRouter()

	// 先创建一个测试用户
	user := &model.User{
		Username:       "testuser",
		Email:          "test@example.com",
		ExternalUserId: "test_user_stats",
		IsExternal:     true,
		Quota:          250000,
		UsedQuota:      50000,
	}
	model.DB.Create(user)

	tests := []struct {
		name           string
		externalUserID string
		expectedStatus int
		expectedMsg    string
	}{
		{
			name:           "获取用户统计成功",
			externalUserID: "test_user_stats",
			expectedStatus: 200,
			expectedMsg:    "",
		},
		{
			name:           "用户不存在",
			externalUserID: "nonexistent_user",
			expectedStatus: 404,
			expectedMsg:    "用户不存在",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/user/external/"+tt.externalUserID+"/stats", nil)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedStatus == 200 {
				assert.Equal(t, true, response["success"])
				assert.NotNil(t, response["data"])
				
				data := response["data"].(map[string]interface{})
				userInfo := data["user_info"].(map[string]interface{})
				assert.Equal(t, "testuser", userInfo["username"])
				assert.Equal(t, "test_user_stats", userInfo["external_user_id"])
				assert.Equal(t, float64(250000), userInfo["current_quota"])
				assert.Equal(t, float64(50000), userInfo["used_quota"])
				assert.Equal(t, float64(0.5), userInfo["current_balance"]) // 250000 / 500000 = 0.5
			} else {
				assert.Equal(t, false, response["success"])
				assert.Contains(t, response["message"], tt.expectedMsg)
			}
		})
	}
}

// 测试删除外部用户Token
func TestDeleteExternalUserToken(t *testing.T) {
	router := setupTestRouter()

	// 先创建一个测试用户
	user := &model.User{
		Username:       "tokenuser",
		Email:          "token@example.com",
		ExternalUserId: "test_token_user",
		IsExternal:     true,
		Quota:          1000000,
	}
	model.DB.Create(user)

	// 创建一个测试Token
	token := &model.Token{
		UserId:      user.Id,
		Key:         "test_token_key_12345678",
		Name:        "Test Token",
		Status:      1,
		CreatedTime: common.GetTimestamp(),
		ExpiredTime: common.GetTimestamp() + 86400*365,
	}
	model.DB.Create(token)

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
		expectedMsg    string
	}{
		{
			name: "删除Token成功",
			requestBody: map[string]interface{}{
				"external_user_id": "test_token_user",
				"token_id":         token.Id,
			},
			expectedStatus: 200,
			expectedMsg:    "Token删除成功",
		},
		{
			name: "用户不存在",
			requestBody: map[string]interface{}{
				"external_user_id": "nonexistent_user",
				"token_id":         999,
			},
			expectedStatus: 404,
			expectedMsg:    "用户不存在",
		},
		{
			name: "Token不存在",
			requestBody: map[string]interface{}{
				"external_user_id": "test_token_user",
				"token_id":         999999,
			},
			expectedStatus: 404,
			expectedMsg:    "Token不存在或无权删除",
		},
		{
			name: "缺少必需字段 - external_user_id",
			requestBody: map[string]interface{}{
				"token_id": token.Id,
			},
			expectedStatus: 400,
			expectedMsg:    "ExternalUserId",
		},
		{
			name: "缺少必需字段 - token_id",
			requestBody: map[string]interface{}{
				"external_user_id": "test_token_user",
			},
			expectedStatus: 400,
			expectedMsg:    "TokenId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBody, _ := json.Marshal(tt.requestBody)
			req, _ := http.NewRequest("DELETE", "/api/user/external/token", bytes.NewBuffer(jsonBody))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedStatus == 200 {
				assert.Equal(t, true, response["success"])
				assert.Equal(t, tt.expectedMsg, response["message"])
				
				// 验证响应数据
				data := response["data"].(map[string]interface{})
				assert.Equal(t, float64(token.Id), data["token_id"])
				assert.Equal(t, "test_token_user", data["external_user_id"])
				
				// 验证Token确实被删除
				var deletedToken model.Token
				err := model.DB.Where("id = ?", token.Id).First(&deletedToken).Error
				assert.Error(t, err) // 应该找不到已删除的Token
			} else {
				assert.Equal(t, false, response["success"])
				assert.Contains(t, response["message"], tt.expectedMsg)
			}
		})
	}

	// 清理测试数据
	model.DB.Delete(user)
}

// 测试OpenID查询功能
func TestGetUserByWechatOpenId(t *testing.T) {
	// 设置测试数据库
	testDB := setupTestDB()
	model.DB = testDB

	tests := []struct {
		name     string
		setup    func() *model.User
		openId   string
		expected bool // 是否期望找到用户
	}{
		{
			name: "找到已存在的微信用户",
			setup: func() *model.User {
				user := &model.User{
					Username:     "wechat_user_1",
					Email:        "wechat1@external.local",
					WechatOpenId: "test_openid_12345",
					LoginType:    "wechat",
					IsExternal:   true,
				}
				model.DB.Create(user)
				return user
			},
			openId:   "test_openid_12345",
			expected: true,
		},
		{
			name: "OpenID不存在",
			setup: func() *model.User {
				return nil
			},
			openId:   "nonexistent_openid",
			expected: false,
		},
		{
			name: "空OpenID",
			setup: func() *model.User {
				return nil
			},
			openId:   "",
			expected: false,
		},
		{
			name: "OpenID为空字符串的用户不应该被匹配",
			setup: func() *model.User {
				user := &model.User{
					Username:     "normal_user",
					Email:        "normal@external.local",
					WechatOpenId: "", // 空的OpenID
					IsExternal:   true,
				}
				model.DB.Create(user)
				return user
			},
			openId:   "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置测试数据
			var createdUser *model.User
			if tt.setup != nil {
				createdUser = tt.setup()
			}

			// 执行查询
			result := model.GetUserByWechatOpenId(tt.openId)

			// 验证结果
			if tt.expected {
				assert.NotNil(t, result, "应该找到用户")
				assert.Equal(t, tt.openId, result.WechatOpenId, "OpenID应该匹配")
			} else {
				assert.Nil(t, result, "不应该找到用户")
			}

			// 清理测试数据
			if createdUser != nil {
				model.DB.Delete(createdUser)
			}
		})
	}
}

// 测试微信账号统一功能
func TestWechatAccountUnification(t *testing.T) {
	// 设置测试数据库
	testDB := setupTestDB()
	model.DB = testDB

	// 设置Gin为测试模式
	gin.SetMode(gin.TestMode)

	// 创建测试路由
	router := gin.New()
	router.POST("/api/user/external/sync", SyncExternalUser)

	tests := []struct {
		name           string
		setup          func() *model.User  // 预先创建的用户
		requestData    SyncExternalUserRequest
		expectedStatus int
		expectedUnified bool
		expectedMsg    string
	}{
		{
			name: "首次微信登录-创建新用户",
			setup: nil,
			requestData: SyncExternalUserRequest{
				ExternalUserId: "wx_mini_test_openid_001",
				Username:       "wx_user_001",
				DisplayName:    "微信用户001",
				WechatOpenId:   "test_openid_001",
				LoginType:      "wechat",
			},
			expectedStatus: 200,
			expectedUnified: false,
			expectedMsg:    "用户创建成功",
		},
		{
			name: "微信小程序登录-统一到已存在账号",
			setup: func() *model.User {
				// 预先创建一个前端平台用户
				user := &model.User{
					Username:       "frontend_user_001",
					ExternalUserId: "frontend_user_001",
					Email:          "frontend001@external.local",
					WechatOpenId:   "test_openid_002",
					LoginType:      "wechat",
					IsExternal:     true,
					Quota:          1000000,
				}
				model.DB.Create(user)
				return user
			},
			requestData: SyncExternalUserRequest{
				ExternalUserId: "wx_mini_test_openid_002",  // 小程序的external_user_id（请求）
				Username:       "wx_user_002",
				DisplayName:    "微信用户002",
				WechatOpenId:   "test_openid_002",          // 相同的OpenID
				LoginType:      "wechat",
			},
			expectedStatus: 200,
			expectedUnified: true,
			expectedMsg:    "账号统一成功",
		},
		{
			name: "相同external_user_id的更新-非统一场景",
			setup: func() *model.User {
				user := &model.User{
					Username:       "existing_user",
					ExternalUserId: "existing_external_id",
					Email:          "existing@external.local",
					WechatOpenId:   "existing_openid",
					LoginType:      "wechat",
					IsExternal:     true,
				}
				model.DB.Create(user)
				return user
			},
			requestData: SyncExternalUserRequest{
				ExternalUserId: "existing_external_id",    // 相同的external_user_id
				Username:       "existing_user",
				DisplayName:    "更新的显示名",
				WechatOpenId:   "existing_openid",
				LoginType:      "wechat",
			},
			expectedStatus: 200,
			expectedUnified: false,
			expectedMsg:    "用户信息同步成功",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置测试数据
			var createdUser *model.User
			if tt.setup != nil {
				createdUser = tt.setup()
			}

			// 准备请求数据
			jsonData, err := json.Marshal(tt.requestData)
			assert.NoError(t, err)

			// 创建请求
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("POST", "/api/user/external/sync", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			// 执行请求
			router.ServeHTTP(w, req)

			// 验证响应状态
			assert.Equal(t, tt.expectedStatus, w.Code)

			// 解析响应
			var response map[string]interface{}
			err = json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			// 验证基本响应
			if tt.expectedStatus == 200 {
				assert.Equal(t, true, response["success"])
				assert.Contains(t, response["message"], tt.expectedMsg)

				// 验证数据部分
				data := response["data"].(map[string]interface{})
				assert.NotEmpty(t, data["user_id"])
				assert.NotEmpty(t, data["external_user_id"])

				// 验证统一状态
				if isUnified, exists := data["is_unified"]; exists {
					assert.Equal(t, tt.expectedUnified, isUnified)
					if tt.expectedUnified {
						// 统一账号应该返回原有的external_user_id，而不是请求的ID
						if createdUser != nil {
							assert.Equal(t, createdUser.ExternalUserId, data["external_user_id"])
							assert.NotEqual(t, tt.requestData.ExternalUserId, data["external_user_id"])
						}
						// 应该包含wechat_openid
						assert.Equal(t, tt.requestData.WechatOpenId, data["wechat_openid"])
					}
				}

				// 验证数据库中的用户信息
				if tt.expectedUnified && createdUser != nil {
					// 统一账号情况：应该只有一个用户，但OpenID相同
					var userCount int64
					model.DB.Model(&model.User{}).Where("wechat_openid = ?", tt.requestData.WechatOpenId).Count(&userCount)
					assert.Equal(t, int64(1), userCount, "应该只有一个用户拥有此OpenID")

					// 验证统一后的用户信息
					var unifiedUser model.User
					err := model.DB.Where("wechat_openid = ?", tt.requestData.WechatOpenId).First(&unifiedUser).Error
					assert.NoError(t, err)
					assert.Equal(t, createdUser.Id, unifiedUser.Id, "应该是同一个用户")
				}
			}

			// 清理测试数据
			if createdUser != nil {
				model.DB.Delete(createdUser)
			}
			// 清理可能创建的新用户
			model.DB.Where("wechat_openid = ?", tt.requestData.WechatOpenId).Delete(&model.User{})
		})
	}
}

// 测试使用 wechat_openid 参数的API功能
func TestWechatOpenIdParameters(t *testing.T) {
	router := setupTestRouter()

	// 创建测试用户
	testUser := &model.User{
		Username:       "wechat_param_test",
		Email:          "wechat_param@test.com",
		ExternalUserId: "wx_mini_oNeBc5Gh3iXXXXXX",
		WechatOpenId:   "oNeBc5Gh3iXXXXXX",
		LoginType:      "wechat",
		IsExternal:     true,
		Quota:          1000000,
	}
	model.DB.Create(testUser)

	t.Run("测试充值API使用wechat_openid", func(t *testing.T) {
		requestBody := map[string]interface{}{
			"wechat_openid": "oNeBc5Gh3iXXXXXX",
			"amount_usd":    5.0,
			"payment_id":    "wechat_topup_test",
		}

		jsonData, _ := json.Marshal(requestBody)
		req, _ := http.NewRequest("POST", "/api/user/external/topup", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, true, response["success"])

		// 验证用户余额增加
		var updatedUser model.User
		model.DB.First(&updatedUser, testUser.Id)
		expectedQuota := 1000000 + int(5.0*float64(common.QuotaPerUnit))
		assert.Equal(t, expectedQuota, updatedUser.Quota)
	})

	t.Run("测试Token创建API使用wechat_openid", func(t *testing.T) {
		requestBody := map[string]interface{}{
			"wechat_openid": "oNeBc5Gh3iXXXXXX",
			"token_name":    "WeChat OpenID Token",
			"expires_in_days": 365,
		}

		jsonData, _ := json.Marshal(requestBody)
		req, _ := http.NewRequest("POST", "/api/user/external/token", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, true, response["success"])

		data := response["data"].(map[string]interface{})
		assert.Equal(t, "WeChat OpenID Token", data["token_name"])
		assert.NotEmpty(t, data["access_key"])
	})

	t.Run("测试用户统计API使用wechat_openid作为路径参数", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/user/external/oNeBc5Gh3iXXXXXX/stats", nil)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, true, response["success"])

		data := response["data"].(map[string]interface{})
		userInfo := data["user_info"].(map[string]interface{})
		assert.Equal(t, "wx_mini_oNeBc5Gh3iXXXXXX", userInfo["external_user_id"])
		assert.Equal(t, "oNeBc5Gh3iXXXXXX", userInfo["wechat_openid"])
	})

	t.Run("测试消费记录API使用wechat_openid作为路径参数", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/user/external/oNeBc5Gh3iXXXXXX/logs", nil)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, true, response["success"])
	})

	t.Run("测试参数验证-两个参数都缺失", func(t *testing.T) {
		requestBody := map[string]interface{}{
			"amount_usd": 5.0,
			"payment_id": "test_payment",
		}

		jsonData, _ := json.Marshal(requestBody)
		req, _ := http.NewRequest("POST", "/api/user/external/topup", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, 400, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, false, response["success"])
		assert.Contains(t, response["message"], "必须提供其中一个")
	})

	// 清理测试数据
	model.DB.Delete(testUser)
	model.DB.Where("user_id = ?", testUser.Id).Delete(&model.Token{})
}