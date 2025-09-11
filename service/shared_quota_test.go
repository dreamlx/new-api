package service

import (
	"one-api/common"
	"one-api/model"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestTokenSharedUserQuota 测试Token共享用户余额的功能
func TestTokenSharedUserQuota(t *testing.T) {
	// 初始化测试数据库
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("连接测试数据库失败: %v", err)
	}

	// 初始化数据库相关变量（包括commonKeyCol）
	model.InitDB()

	// 重新设置为测试数据库（因为InitDB会重新连接数据库）
	model.DB = db

	// 自动迁移表结构
	db.AutoMigrate(&model.User{}, &model.Token{})

	// 禁用Redis以避免测试中的nil pointer错误
	common.RedisEnabled = false

	// 创建测试用户
	user := &model.User{
		Username: "test_shared_quota_user",
		Password: "password",
		Email:    "test@example.com",
		Quota:    1000, // 用户有1000额度
	}

	err = user.Insert(0)
	if err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}

	// 手动设置用户额度为1000（因为Insert方法会覆盖为默认值）
	err = model.IncreaseUserQuota(user.Id, 1000, true)
	if err != nil {
		t.Fatalf("设置用户额度失败: %v", err)
	}

	defer func() {
		// 清理测试数据
		model.DB.Where("username = ?", user.Username).Delete(&model.User{})
	}()

	// 创建两个Token，都属于同一个用户
	token1 := &model.Token{
		UserId:      user.Id,
		Key:         "sk-test1234567890123456789012345678",
		Name:        "test_token_1",
		Status:      common.TokenStatusEnabled,
		RemainQuota: 500, // Token1有500额度（这个值应该被忽略）
	}

	token2 := &model.Token{
		UserId:      user.Id,
		Key:         "sk-test2234567890123456789012345678",
		Name:        "test_token_2",
		Status:      common.TokenStatusEnabled,
		RemainQuota: 300, // Token2有300额度（这个值应该被忽略）
	}

	err = token1.Insert()
	if err != nil {
		t.Fatalf("创建测试Token1失败: %v", err)
	}
	defer func() {
		model.DB.Where("key = ?", token1.Key).Delete(&model.Token{})
	}()

	err = token2.Insert()
	if err != nil {
		t.Fatalf("创建测试Token2失败: %v", err)
	}
	defer func() {
		model.DB.Where("key = ?", token2.Key).Delete(&model.Token{})
	}()

	// 测试1: 验证Token验证时检查的是用户余额而不是Token余额
	t.Run("Token验证检查用户余额", func(t *testing.T) {
		// Token1应该能通过验证，因为用户有1000余额
		validatedToken1, err := model.ValidateUserToken(token1.Key)
		if err != nil {
			t.Errorf("Token1验证失败: %v", err)
		}
		if validatedToken1.Id != token1.Id {
			t.Errorf("返回的Token不正确")
		}

		// Token2也应该能通过验证，因为它们共享用户余额
		validatedToken2, err := model.ValidateUserToken(token2.Key)
		if err != nil {
			t.Errorf("Token2验证失败: %v", err)
		}
		if validatedToken2.Id != token2.Id {
			t.Errorf("返回的Token不正确")
		}
	})

	// 测试2: 验证扣费时只扣减用户余额
	t.Run("扣费只影响用户余额", func(t *testing.T) {
		// 扣减200额度
		err := model.DecreaseUserQuota(user.Id, 200)
		if err != nil {
			t.Fatalf("扣减用户额度失败: %v", err)
		}

		// 检查用户余额是否正确减少
		userQuota, err := model.GetUserQuota(user.Id, true)
		if err != nil {
			t.Fatalf("获取用户余额失败: %v", err)
		}
		if userQuota != 800 {
			t.Errorf("用户余额不正确，期望800，实际%d", userQuota)
		}

		// 两个Token都应该仍然能通过验证（因为用户还有800余额）
		_, err = model.ValidateUserToken(token1.Key)
		if err != nil {
			t.Errorf("扣费后Token1验证失败: %v", err)
		}

		_, err = model.ValidateUserToken(token2.Key)
		if err != nil {
			t.Errorf("扣费后Token2验证失败: %v", err)
		}
	})

	// 测试3: 验证用户余额耗尽时所有Token都不可用
	t.Run("用户余额耗尽时所有Token不可用", func(t *testing.T) {
		// 耗尽用户余额
		err := model.DecreaseUserQuota(user.Id, 800)
		if err != nil {
			t.Fatalf("耗尽用户余额失败: %v", err)
		}

		// 检查用户余额是否为0
		userQuota, err := model.GetUserQuota(user.Id, true)
		if err != nil {
			t.Fatalf("获取用户余额失败: %v", err)
		}
		if userQuota != 0 {
			t.Errorf("用户余额不正确，期望0，实际%d", userQuota)
		}

		// 两个Token都应该验证失败
		_, err = model.ValidateUserToken(token1.Key)
		if err == nil {
			t.Errorf("用户余额耗尽时Token1应该验证失败")
		}

		_, err = model.ValidateUserToken(token2.Key)
		if err == nil {
			t.Errorf("用户余额耗尽时Token2应该验证失败")
		}
	})
}
