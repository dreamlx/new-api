package service

import (
	"net/http"
	"net/http/httptest"
	"one-api/common"
	"one-api/model"
	relaycommon "one-api/relay/common"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestCompleteBillingFlow 测试完整的计费流程
func TestCompleteBillingFlow(t *testing.T) {
	// 初始化测试数据库
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("连接测试数据库失败: %v", err)
	}

	// 初始化数据库相关变量
	model.InitDB()

	// 重新设置为测试数据库
	model.DB = db

	// 自动迁移表结构
	db.AutoMigrate(&model.User{}, &model.Token{})

	// 禁用Redis以避免测试中的nil pointer错误
	common.RedisEnabled = false

	// 创建测试用户
	user := &model.User{
		Username: "test_billing_user",
		Password: "password",
		Email:    "billing@example.com",
		Quota:    10000, // 用户有10000额度
	}

	err = user.Insert(0)
	if err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}

	// 手动设置用户额度为10000
	err = model.IncreaseUserQuota(user.Id, 10000, true)
	if err != nil {
		t.Fatalf("设置用户额度失败: %v", err)
	}

	defer func() {
		// 清理测试数据
		model.DB.Where("username = ?", user.Username).Delete(&model.User{})
	}()

	// 创建测试Token
	token := &model.Token{
		UserId:      user.Id,
		Key:         "sk-testbilling1234567890123456789012",
		Name:        "test_billing_token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: 5000, // Token有5000额度（这个值应该被忽略）
	}

	err = token.Insert()
	if err != nil {
		t.Fatalf("创建测试Token失败: %v", err)
	}
	defer func() {
		model.DB.Where("key = ?", token.Key).Delete(&model.Token{})
	}()

	// 创建RelayInfo
	relayInfo := &relaycommon.RelayInfo{
		UserId:         user.Id,
		TokenId:        token.Id,
		TokenKey:       token.Key,
		TokenUnlimited: false,
		UserQuota:      10000,
	}

	// 测试1: 验证预扣费流程
	t.Run("预扣费流程测试", func(t *testing.T) {
		// 创建Gin上下文
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("POST", "/test", nil)

		// 预扣费500额度
		apiErr := PreConsumeQuota(c, 500, relayInfo)
		if apiErr != nil {
			t.Fatalf("预扣费失败: %v", apiErr)
		}

		// 检查用户余额是否正确减少
		userQuota, err := model.GetUserQuota(user.Id, true)
		if err != nil {
			t.Fatalf("获取用户余额失败: %v", err)
		}
		if userQuota != 9500 {
			t.Errorf("预扣费后用户余额不正确，期望9500，实际%d", userQuota)
		}

		// 验证Token仍然可以通过验证（因为用户还有余额）
		_, err = model.ValidateUserToken(token.Key)
		if err != nil {
			t.Errorf("预扣费后Token验证失败: %v", err)
		}
	})

	// 测试2: 验证实际扣费流程
	t.Run("实际扣费流程测试", func(t *testing.T) {
		// 模拟实际使用了300额度，需要退还200额度
		actualQuota := 300
		preConsumedQuota := 500
		quotaDelta := actualQuota - preConsumedQuota // -200，表示需要退还200

		// 执行实际扣费（这会退还多扣的额度）
		err := PostConsumeQuota(relayInfo, quotaDelta, preConsumedQuota, false)
		if err != nil {
			t.Fatalf("实际扣费失败: %v", err)
		}

		// 检查用户余额：10000 - 300 = 9700
		userQuota, err := model.GetUserQuota(user.Id, true)
		if err != nil {
			t.Fatalf("获取用户余额失败: %v", err)
		}
		expectedQuota := 10000 - actualQuota
		if userQuota != expectedQuota {
			t.Errorf("实际扣费后用户余额不正确，期望%d，实际%d", expectedQuota, userQuota)
		}
	})

	// 测试3: 验证余额不足时的行为
	t.Run("余额不足测试", func(t *testing.T) {
		// 创建Gin上下文
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("POST", "/test", nil)

		// 尝试预扣费超过余额的金额（当前余额9700，尝试扣费10000）
		apiErr := PreConsumeQuota(c, 10000, relayInfo)
		if apiErr == nil {
			t.Errorf("余额不足时应该返回错误")
		}

		// 检查用户余额没有变化
		userQuota, err := model.GetUserQuota(user.Id, true)
		if err != nil {
			t.Fatalf("获取用户余额失败: %v", err)
		}
		if userQuota != 9700 {
			t.Errorf("余额不足时用户余额不应该变化，期望9700，实际%d", userQuota)
		}
	})

	// 测试4: 验证余额耗尽时Token验证失败
	t.Run("余额耗尽Token验证测试", func(t *testing.T) {
		// 耗尽用户余额
		err := model.DecreaseUserQuota(user.Id, 9700)
		if err != nil {
			t.Fatalf("耗尽用户余额失败: %v", err)
		}

		// 验证用户余额为0
		userQuota, err := model.GetUserQuota(user.Id, true)
		if err != nil {
			t.Fatalf("获取用户余额失败: %v", err)
		}
		if userQuota != 0 {
			t.Errorf("用户余额应该为0，实际%d", userQuota)
		}

		// Token验证应该失败
		_, err = model.ValidateUserToken(token.Key)
		if err == nil {
			t.Errorf("余额耗尽时Token验证应该失败")
		}
	})

	// 测试5: 验证充值后Token立即可用
	t.Run("充值后Token立即可用测试", func(t *testing.T) {
		// 为用户充值5000
		err := model.IncreaseUserQuota(user.Id, 5000, true)
		if err != nil {
			t.Fatalf("用户充值失败: %v", err)
		}

		// 验证用户余额
		userQuota, err := model.GetUserQuota(user.Id, true)
		if err != nil {
			t.Fatalf("获取用户余额失败: %v", err)
		}
		if userQuota != 5000 {
			t.Errorf("充值后用户余额不正确，期望5000，实际%d", userQuota)
		}

		// Token应该立即可以通过验证
		_, err = model.ValidateUserToken(token.Key)
		if err != nil {
			t.Errorf("充值后Token验证失败: %v", err)
		}

		// 创建Gin上下文测试预扣费
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("POST", "/test", nil)

		// 应该能够成功预扣费
		relayInfo.UserQuota = 5000 // 更新RelayInfo中的用户余额
		apiErr := PreConsumeQuota(c, 1000, relayInfo)
		if apiErr != nil {
			t.Errorf("充值后预扣费失败: %v", apiErr)
		}
	})
}
