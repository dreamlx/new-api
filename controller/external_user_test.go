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
	db.AutoMigrate(&model.User{}, &model.Token{}, &model.TopUp{})
	
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
	
	// 注册外部用户路由
	api := router.Group("/api")
	{
		externalUser := api.Group("/user/external")
		{
			externalUser.POST("/sync", SyncExternalUser)
			externalUser.POST("/topup", ExternalUserTopUp)
			externalUser.POST("/token", CreateExternalUserToken)
			externalUser.GET("/:external_user_id/stats", GetExternalUserStats)
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
			expectedMsg:    "用户信息更新成功",
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