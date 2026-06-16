package controller

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupWisemodelTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	testDB := setupTestDB()
	model.DB = testDB
	model.LOG_DB = testDB

	// Migrate WiseModel tables
	testDB.AutoMigrate(&model.WisemodelPackage{})

	wm := router.Group("/api/wisemodel")
	{
		wm.POST("/user/bind", WisemodelBind)
		wm.POST("/user/delete_wisemodel_key", DeleteWisemodelKey)
		wm.POST("/user/update_wisemodel_key", UpdateWisemodelKey)
		wm.POST("/user/update_phone", UpdatePhone)
		wm.POST("/user/delete_wisemodel_user", DeleteWisemodelUser)
		wm.POST("/orders/record", CreateOrder)
		wm.POST("/user/package_usage", GetPackageUsage)
	}

	return router
}

func doWmRequest(router *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	return doRequest(router, method, path, body)
}

func parseWmResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	return resp
}

// ==================== Bind ====================

func TestWisemodelBind(t *testing.T) {
	router := setupWisemodelTestRouter()

	t.Run("新用户绑定成功", func(t *testing.T) {
		w := doWmRequest(router, "POST", "/api/wisemodel/user/bind", map[string]interface{}{
			"phone":         "13800000001",
			"wisemodel_key": "wisemodel-V1testkey001",
			"username":      "wm_user_001",
		})
		assert.Equal(t, 200, w.Code)
		resp := parseWmResponse(t, w)
		assert.Equal(t, true, resp["success"])
		assert.Equal(t, "绑定成功", resp["message"])
		assert.Equal(t, "wisemodel-V1testkey001", resp["access_key"])

		// Verify user created
		user := model.GetUserByPhone("13800000001")
		require.NotNil(t, user)
		assert.Equal(t, "wisemodel-V1testkey001", user.WisemodelKey)

		// Verify token created
		token, err := model.GetTokenByKey("wisemodel-V1testkey001", true)
		require.NoError(t, err)
		assert.True(t, token.UnlimitedQuota)
		assert.Equal(t, common.TokenStatusEnabled, token.Status)
	})

	t.Run("已有用户更新key", func(t *testing.T) {
		w := doWmRequest(router, "POST", "/api/wisemodel/user/bind", map[string]interface{}{
			"phone":         "13800000001",
			"wisemodel_key": "wisemodel-V1newkey002",
			"username":      "wm_user_001",
		})
		assert.Equal(t, 200, w.Code)

		// Old token should be disabled
		oldToken, _ := model.GetTokenByKey("wisemodel-V1testkey001", true)
		if oldToken != nil {
			assert.Equal(t, common.TokenStatusDisabled, oldToken.Status)
		}

		// New token should be enabled
		newToken, err := model.GetTokenByKey("wisemodel-V1newkey002", true)
		require.NoError(t, err)
		assert.Equal(t, common.TokenStatusEnabled, newToken.Status)
	})

	t.Run("缺少参数", func(t *testing.T) {
		w := doWmRequest(router, "POST", "/api/wisemodel/user/bind", map[string]interface{}{
			"phone": "13800000002",
		})
		assert.Equal(t, 400, w.Code)
	})
}

// ==================== Delete Key ====================

func TestDeleteWisemodelKey(t *testing.T) {
	router := setupWisemodelTestRouter()

	// Setup: bind a user
	doWmRequest(router, "POST", "/api/wisemodel/user/bind", map[string]interface{}{
		"phone": "13800000010", "wisemodel_key": "wisemodel-V1delkey", "username": "delkey_user",
	})

	t.Run("删除成功", func(t *testing.T) {
		w := doWmRequest(router, "POST", "/api/wisemodel/user/delete_wisemodel_key", map[string]interface{}{
			"phone": "13800000010",
		})
		assert.Equal(t, 200, w.Code)

		user := model.GetUserByPhone("13800000010")
		assert.Equal(t, "", user.WisemodelKey)

		token, _ := model.GetTokenByKey("wisemodel-V1delkey", true)
		if token != nil {
			assert.Equal(t, common.TokenStatusDisabled, token.Status)
		}
	})

	t.Run("用户不存在", func(t *testing.T) {
		w := doWmRequest(router, "POST", "/api/wisemodel/user/delete_wisemodel_key", map[string]interface{}{
			"phone": "19999999999",
		})
		assert.Equal(t, 404, w.Code)
	})
}

// ==================== Update Key ====================

func TestUpdateWisemodelKey(t *testing.T) {
	router := setupWisemodelTestRouter()

	doWmRequest(router, "POST", "/api/wisemodel/user/bind", map[string]interface{}{
		"phone": "13800000020", "wisemodel_key": "wisemodel-V1oldkey", "username": "updkey_user",
	})

	t.Run("更新成功", func(t *testing.T) {
		w := doWmRequest(router, "POST", "/api/wisemodel/user/update_wisemodel_key", map[string]interface{}{
			"phone":   "13800000020",
			"new_key": "wisemodel-V1updatedkey",
		})
		assert.Equal(t, 200, w.Code)

		user := model.GetUserByPhone("13800000020")
		assert.Equal(t, "wisemodel-V1updatedkey", user.WisemodelKey)

		newToken, err := model.GetTokenByKey("wisemodel-V1updatedkey", true)
		require.NoError(t, err)
		assert.Equal(t, common.TokenStatusEnabled, newToken.Status)
	})

	t.Run("用户不存在", func(t *testing.T) {
		w := doWmRequest(router, "POST", "/api/wisemodel/user/update_wisemodel_key", map[string]interface{}{
			"phone": "19999999999", "new_key": "wisemodel-V1xxx",
		})
		assert.Equal(t, 404, w.Code)
	})
}

// ==================== Update Phone ====================

func TestUpdatePhone(t *testing.T) {
	t.Run("更新成功", func(t *testing.T) {
		router := setupWisemodelTestRouter()
		doWmRequest(router, "POST", "/api/wisemodel/user/bind", map[string]interface{}{
			"phone": "13800000030", "wisemodel_key": "wisemodel-V1phone", "username": "phone_user",
		})

		w := doWmRequest(router, "POST", "/api/wisemodel/user/update_phone", map[string]interface{}{
			"old_phone": "13800000030",
			"new_phone": "13800000031",
		})
		assert.Equal(t, 200, w.Code)

		assert.Nil(t, model.GetUserByPhone("13800000030"))
		assert.NotNil(t, model.GetUserByPhone("13800000031"))
	})

	t.Run("新手机号已占用", func(t *testing.T) {
		router2 := setupWisemodelTestRouter()
		// Create two users on SAME router/DB
		w1 := doWmRequest(router2, "POST", "/api/wisemodel/user/bind", map[string]interface{}{
			"phone": "13800000033", "wisemodel_key": "wisemodel-V1phoneA", "username": "phone_userA",
		})
		require.Equal(t, 200, w1.Code)
		w2 := doWmRequest(router2, "POST", "/api/wisemodel/user/bind", map[string]interface{}{
			"phone": "13800000034", "wisemodel_key": "wisemodel-V1phoneB", "username": "phone_userB",
		})
		require.Equal(t, 200, w2.Code)

		w := doWmRequest(router2, "POST", "/api/wisemodel/user/update_phone", map[string]interface{}{
			"old_phone": "13800000033",
			"new_phone": "13800000034",
		})
		assert.Equal(t, 400, w.Code)
		resp := parseWmResponse(t, w)
		assert.Contains(t, resp["message"], "已被使用")
	})

	t.Run("用户不存在", func(t *testing.T) {
		router := setupWisemodelTestRouter()
		w := doWmRequest(router, "POST", "/api/wisemodel/user/update_phone", map[string]interface{}{
			"old_phone": "19999999999", "new_phone": "19999999998",
		})
		assert.Equal(t, 404, w.Code)
	})
}

// ==================== Delete User ====================

func TestDeleteWisemodelUser(t *testing.T) {
	t.Run("无付费包-删除成功", func(t *testing.T) {
		router := setupWisemodelTestRouter()
		doWmRequest(router, "POST", "/api/wisemodel/user/bind", map[string]interface{}{
			"phone": "13800000040", "wisemodel_key": "wisemodel-V1delu", "username": "delu_user",
		})

		// Add a free package
		validUntil := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
		doWmRequest(router, "POST", "/api/wisemodel/orders/record", map[string]interface{}{
			"order_id": "ORD_DEL_001", "package_count": 1,
			"packages": []map[string]interface{}{
				{"id": "PKG_DEL_001", "points": 1000000, "amount": 0.01, "phone": "13800000040",
					"is_free": true, "valid_until": validUntil, "created_at": time.Now().Format(time.RFC3339)},
			},
		})

		w := doWmRequest(router, "POST", "/api/wisemodel/user/delete_wisemodel_user", map[string]interface{}{
			"phone": "13800000040",
		})
		assert.Equal(t, 200, w.Code)

		// User still exists but key cleared
		user := model.GetUserByPhone("13800000040")
		require.NotNil(t, user)
		assert.Equal(t, "", user.WisemodelKey)

		// Packages deleted
		pkgs, _ := model.GetWisemodelPackagesByUserId(user.Id)
		assert.Len(t, pkgs, 0)
	})

	t.Run("有付费包-拒绝删除", func(t *testing.T) {
		router := setupWisemodelTestRouter()
		doWmRequest(router, "POST", "/api/wisemodel/user/bind", map[string]interface{}{
			"phone": "13800000041", "wisemodel_key": "wisemodel-V1paid", "username": "paid_user",
		})

		validUntil := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
		doWmRequest(router, "POST", "/api/wisemodel/orders/record", map[string]interface{}{
			"order_id": "ORD_PAID_001", "package_count": 1,
			"packages": []map[string]interface{}{
				{"id": "PKG_PAID_001", "points": 1000000, "amount": 10.0, "phone": "13800000041",
					"is_free": false, "valid_until": validUntil, "created_at": time.Now().Format(time.RFC3339)},
			},
		})

		w := doWmRequest(router, "POST", "/api/wisemodel/user/delete_wisemodel_user", map[string]interface{}{
			"phone": "13800000041",
		})
		assert.Equal(t, 403, w.Code)
		resp := parseWmResponse(t, w)
		assert.Contains(t, resp["message"], "付费订单")
	})

	t.Run("用户不存在", func(t *testing.T) {
		router := setupWisemodelTestRouter()
		w := doWmRequest(router, "POST", "/api/wisemodel/user/delete_wisemodel_user", map[string]interface{}{
			"phone": "19999999999",
		})
		assert.Equal(t, 404, w.Code)
	})
}

// ==================== Create Order ====================

func TestCreateOrder(t *testing.T) {
	router := setupWisemodelTestRouter()

	// Setup user
	doWmRequest(router, "POST", "/api/wisemodel/user/bind", map[string]interface{}{
		"phone": "13800000050", "wisemodel_key": "wisemodel-V1order", "username": "order_user",
	})

	validUntil := time.Now().Add(365 * 24 * time.Hour).Format(time.RFC3339)
	createdAt := time.Now().Format(time.RFC3339)

	t.Run("积分模式充值成功", func(t *testing.T) {
		w := doWmRequest(router, "POST", "/api/wisemodel/orders/record", map[string]interface{}{
			"order_id": "ORD001", "package_count": 1,
			"packages": []map[string]interface{}{
				{"id": "PKG_PTS_001", "points": 2000000, "amount": 2.0, "phone": "13800000050",
					"is_free": false, "model_names": "deepseek-chat,minimax-m2",
					"valid_until": validUntil, "created_at": createdAt},
			},
		})
		assert.Equal(t, 200, w.Code)

		// Verify quota added: 2000000 points = 2000000 quota
		user := model.GetUserByPhone("13800000050")
		assert.Equal(t, 2000000, user.Quota)

		// remain_quota must be initialized to the granted amount (single ledger).
		var pkg model.WisemodelPackage
		require.NoError(t, model.DB.Where("original_package_id = ?", "PKG_PTS_001").First(&pkg).Error)
		assert.Equal(t, pkg.QuotaGranted, pkg.RemainQuota)
		assert.Equal(t, int64(2000000), pkg.RemainQuota)
	})

	t.Run("Tokens模式充值成功", func(t *testing.T) {
		w := doWmRequest(router, "POST", "/api/wisemodel/orders/record", map[string]interface{}{
			"order_id": "ORD002", "package_count": 1,
			"packages": []map[string]interface{}{
				{"id": "PKG_TKN_001", "tokens": 1000000, "amount": 1.0, "phone": "13800000050",
					"valid_until": validUntil, "created_at": createdAt},
			},
		})
		assert.Equal(t, 200, w.Code)

		// Additional 1000000 tokens = 1000000 quota
		user := model.GetUserByPhone("13800000050")
		assert.Equal(t, 3000000, user.Quota)
	})

	t.Run("package_count不匹配", func(t *testing.T) {
		w := doWmRequest(router, "POST", "/api/wisemodel/orders/record", map[string]interface{}{
			"order_id": "ORD003", "package_count": 2,
			"packages": []map[string]interface{}{
				{"id": "PKG_X", "points": 1000, "amount": 0.01, "phone": "13800000050",
					"valid_until": validUntil, "created_at": createdAt},
			},
		})
		assert.Equal(t, 400, w.Code)
		resp := parseWmResponse(t, w)
		assert.Contains(t, resp["message"], "不一致")
	})

	t.Run("无points也无tokens", func(t *testing.T) {
		w := doWmRequest(router, "POST", "/api/wisemodel/orders/record", map[string]interface{}{
			"order_id": "ORD004", "package_count": 1,
			"packages": []map[string]interface{}{
				{"id": "PKG_EMPTY", "amount": 0.01, "phone": "13800000050",
					"valid_until": validUntil, "created_at": createdAt},
			},
		})
		assert.Equal(t, 400, w.Code)
		resp := parseWmResponse(t, w)
		assert.Contains(t, resp["message"], "必须提供points或tokens")
	})

	t.Run("valid_until是过去时间", func(t *testing.T) {
		pastTime := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
		w := doWmRequest(router, "POST", "/api/wisemodel/orders/record", map[string]interface{}{
			"order_id": "ORD005", "package_count": 1,
			"packages": []map[string]interface{}{
				{"id": "PKG_PAST", "points": 1000, "amount": 0.01, "phone": "13800000050",
					"valid_until": pastTime, "created_at": createdAt},
			},
		})
		assert.Equal(t, 400, w.Code)
		resp := parseWmResponse(t, w)
		assert.Contains(t, resp["message"], "未来时间")
	})

	t.Run("用户不存在", func(t *testing.T) {
		w := doWmRequest(router, "POST", "/api/wisemodel/orders/record", map[string]interface{}{
			"order_id": "ORD006", "package_count": 1,
			"packages": []map[string]interface{}{
				{"id": "PKG_NOUSER", "points": 1000, "amount": 0.01, "phone": "19999999999",
					"valid_until": validUntil, "created_at": createdAt},
			},
		})
		assert.Equal(t, 404, w.Code)
	})
}

// ==================== Package Usage ====================

func TestGetPackageUsage(t *testing.T) {
	router := setupWisemodelTestRouter()

	// Setup user + package
	doWmRequest(router, "POST", "/api/wisemodel/user/bind", map[string]interface{}{
		"phone": "13800000060", "wisemodel_key": "wisemodel-V1usage", "username": "usage_user",
	})

	validUntil := time.Now().Add(365 * 24 * time.Hour).Format(time.RFC3339)
	doWmRequest(router, "POST", "/api/wisemodel/orders/record", map[string]interface{}{
		"order_id": "ORD_USAGE", "package_count": 1,
		"packages": []map[string]interface{}{
			{"id": "PKG_USAGE_001", "points": 1000000, "amount": 1.0, "phone": "13800000060",
				"model_names": "deepseek-chat", "valid_until": validUntil,
				"created_at": time.Now().Format(time.RFC3339)},
		},
	})

	t.Run("查询成功-有包", func(t *testing.T) {
		w := doWmRequest(router, "POST", "/api/wisemodel/user/package_usage", map[string]interface{}{
			"phone": "13800000060",
		})
		assert.Equal(t, 200, w.Code)
		resp := parseWmResponse(t, w)
		assert.Equal(t, float64(200), resp["code"])
		assert.Equal(t, "success", resp["msg"])

		data := resp["data"].([]interface{})
		assert.Len(t, data, 1)

		pkg := data[0].(map[string]interface{})
		assert.Equal(t, float64(1000000), pkg["points"])
		assert.Equal(t, float64(1000000), pkg["remain_points"]) // no consumption
		assert.Equal(t, float64(0), pkg["amount"])

		models := pkg["available_models"].([]interface{})
		assert.Len(t, models, 1)
		assert.Equal(t, "deepseek-chat", models[0])
	})

	t.Run("查询成功-无包", func(t *testing.T) {
		router2 := setupWisemodelTestRouter()
		doWmRequest(router2, "POST", "/api/wisemodel/user/bind", map[string]interface{}{
			"phone": "13800000061", "wisemodel_key": "wisemodel-V1empty", "username": "empty_user",
		})

		w := doWmRequest(router2, "POST", "/api/wisemodel/user/package_usage", map[string]interface{}{
			"phone": "13800000061",
		})
		assert.Equal(t, 200, w.Code)
		resp := parseWmResponse(t, w)
		data := resp["data"].([]interface{})
		assert.Len(t, data, 0)
	})

	t.Run("用户不存在", func(t *testing.T) {
		router3 := setupWisemodelTestRouter()
		w := doWmRequest(router3, "POST", "/api/wisemodel/user/package_usage", map[string]interface{}{
			"phone": "19999999999",
		})
		assert.Equal(t, 404, w.Code)
	})
}

// ==================== Integration: Bind + Order + Usage ====================

func TestWisemodelIntegrationFlow(t *testing.T) {
	router := setupWisemodelTestRouter()

	// 1. Bind user
	w := doWmRequest(router, "POST", "/api/wisemodel/user/bind", map[string]interface{}{
		"phone": "13800000070", "wisemodel_key": "wisemodel-V1integ", "username": "integ_user",
	})
	assert.Equal(t, 200, w.Code)

	// 2. Create order with 2 packages
	validUntil := time.Now().Add(365 * 24 * time.Hour).Format(time.RFC3339)
	w = doWmRequest(router, "POST", "/api/wisemodel/orders/record", map[string]interface{}{
		"order_id": "ORD_INTEG", "package_count": 2,
		"packages": []map[string]interface{}{
			{"id": "PKG_INTEG_A", "points": 2000000, "amount": 2.0, "phone": "13800000070",
				"model_names": "deepseek-chat,deepseek-reasoner", "valid_until": validUntil,
				"created_at": time.Now().Format(time.RFC3339)},
			{"id": "PKG_INTEG_B", "tokens": 5000000, "amount": 5.0, "phone": "13800000070",
				"is_free": false, "valid_until": validUntil,
				"created_at": time.Now().Format(time.RFC3339)},
		},
	})
	assert.Equal(t, 200, w.Code)

	// 3. Verify quota: 2000000 points + 5000000 tokens = 7000000 quota
	user := model.GetUserByPhone("13800000070")
	assert.Equal(t, 7000000, user.Quota)

	// 4. Check package usage
	w = doWmRequest(router, "POST", "/api/wisemodel/user/package_usage", map[string]interface{}{
		"phone": "13800000070",
	})
	assert.Equal(t, 200, w.Code)
	resp := parseWmResponse(t, w)
	data := resp["data"].([]interface{})
	assert.Len(t, data, 2)

	// 5. Simulate consumption logs for attribution
	user = model.GetUserByPhone("13800000070")
	pkgs, _ := model.GetWisemodelPackagesByUserId(user.Id)
	require.Len(t, pkgs, 2)

	// Insert a log attributed to first package
	model.LOG_DB.Create(&model.Log{
		UserId: user.Id, Username: user.Username,
		CreatedAt: common.GetTimestamp(), Type: model.LogTypeConsume,
		ModelName: "deepseek-chat", Quota: 5000,
		PromptTokens: 500, CompletionTokens: 200,
		WisemodelPackageId: pkgs[0].PackageId,
	})
	// Charge the single ledger as the gate would (display now reads remain_quota).
	ok, derr := model.TryDeductPackageRemain(pkgs[0].PackageId, 5000)
	require.NoError(t, derr)
	require.True(t, ok)

	// 6. Re-query usage — should show consumption on first package
	w = doWmRequest(router, "POST", "/api/wisemodel/user/package_usage", map[string]interface{}{
		"phone": "13800000070",
	})
	assert.Equal(t, 200, w.Code)
	resp = parseWmResponse(t, w)
	data = resp["data"].([]interface{})

	// Find the package with consumption
	var foundConsumption bool
	for _, d := range data {
		pkg := d.(map[string]interface{})
		if amt, ok := pkg["amount"]; ok && amt.(float64) > 0 {
			foundConsumption = true
			// details should contain the model breakdown
			details := pkg["details"].([]interface{})
			assert.True(t, len(details) > 0)
			detail := details[0].(map[string]interface{})
			assert.Equal(t, "deepseek-chat", detail["model_name"])
		}
	}
	assert.True(t, foundConsumption, "should find package with consumption")

	// 7. Delete user (should fail — has paid packages)
	w = doWmRequest(router, "POST", "/api/wisemodel/user/delete_wisemodel_user", map[string]interface{}{
		"phone": "13800000070",
	})
	assert.Equal(t, 403, w.Code)
}

// ==================== Edge: Bind idempotency ====================

func TestWisemodelBindIdempotent(t *testing.T) {
	router := setupWisemodelTestRouter()

	body := map[string]interface{}{
		"phone": "13800000080", "wisemodel_key": "wisemodel-V1idem", "username": "idem_user",
	}

	// First bind
	w := doWmRequest(router, "POST", "/api/wisemodel/user/bind", body)
	assert.Equal(t, 200, w.Code)

	// Second bind with same key — should succeed (duplicate token handled)
	w = doWmRequest(router, "POST", "/api/wisemodel/user/bind", body)
	assert.Equal(t, 200, w.Code)

	// Verify only one user exists
	var count int64
	model.DB.Model(&model.User{}).Where("phone = ?", "13800000080").Count(&count)
	assert.Equal(t, int64(1), count)
}

// ==================== Edge: Multi-package order ====================

func TestCreateOrderMultiPackage(t *testing.T) {
	router := setupWisemodelTestRouter()

	doWmRequest(router, "POST", "/api/wisemodel/user/bind", map[string]interface{}{
		"phone": "13800000090", "wisemodel_key": "wisemodel-V1multi", "username": "multi_user",
	})

	validUntil := time.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339)
	w := doWmRequest(router, "POST", "/api/wisemodel/orders/record", map[string]interface{}{
		"order_id": "ORD_MULTI", "package_count": 3,
		"packages": []map[string]interface{}{
			{"id": "M1", "points": 500000, "amount": 0.01, "phone": "13800000090", "is_free": true,
				"valid_until": validUntil, "created_at": time.Now().Format(time.RFC3339)},
			{"id": "M2", "points": 500000, "amount": 0.01, "phone": "13800000090", "is_free": true,
				"valid_until": validUntil, "created_at": time.Now().Format(time.RFC3339)},
			{"id": "M3", "tokens": 1000000, "amount": 0.01, "phone": "13800000090", "is_free": true,
				"valid_until": validUntil, "created_at": time.Now().Format(time.RFC3339)},
		},
	})
	assert.Equal(t, 200, w.Code)

	user := model.GetUserByPhone("13800000090")
	// 500000 points + 500000 points + 1000000 tokens = 2000000 quota
	assert.Equal(t, 2000000, user.Quota)

	pkgs, _ := model.GetWisemodelPackagesByUserId(user.Id)
	assert.Len(t, pkgs, 3)
}
