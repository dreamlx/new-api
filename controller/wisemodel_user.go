package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// createWisemodelToken 为用户创建 wisemodel token，并写入 tokens 表。
// 并发场景下若 duplicate key 则忽略错误。
func createWisemodelToken(user *model.User, wisemodelKey string) error {
	newToken := &model.Token{
		UserId:          user.Id,
		Key:             wisemodelKey,
		Name:            "wisemodel-token",
		Status:          common.TokenStatusEnabled,
		CreatedTime:     common.GetTimestamp(),
		AccessedTime:    common.GetTimestamp(),
		ExpiredTime:    -1,
		UnlimitedQuota: true,
	}
	err := newToken.Insert()
	if err != nil {
		// duplicate key：token 已存在（可能是 disabled 状态），重新启用
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "UNIQUE") {
			return model.EnableTokenByKey(wisemodelKey)
		}
		return err
	}
	return nil
}

// WisemodelBind 用户绑定接口
// POST /api/wisemodel/user/bind
func WisemodelBind(c *gin.Context) {
	var req struct {
		Phone        string `json:"phone" binding:"required"`
		WisemodelKey string `json:"wisemodel_key" binding:"required"`
		Username     string `json:"username" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{
			"message": "参数错误: " + err.Error(),
			"success": false,
		})
		return
	}

	// 根据手机号查找用户
	user := model.GetUserByPhone(req.Phone)

	if user == nil {
		// 用户不存在，创建新用户
		user = &model.User{
			Phone:          req.Phone,
			Username:        req.Username,
			DisplayName:     req.Username,
			WisemodelKey:    req.WisemodelKey,
			AffCode:         common.GetRandomString(16),
			ExternalUserId:  "wm_" + req.Phone,
			Role:            common.RoleCommonUser,
			Status:          common.UserStatusEnabled,
			Quota:           0,
		}

		// 生成随机密码（Wisemodel用户不使用密码登录）
		randomPassword, err := common.GenerateKey()
		if err != nil {
			c.JSON(500, gin.H{
				"message": "生成密码失败",
				"success": false,
			})
			return
		}

		hashedPassword, err := common.Password2Hash(randomPassword)
		if err != nil {
			c.JSON(500, gin.H{
				"message": "加密密码失败",
				"success": false,
			})
			return
		}
		user.Password = hashedPassword

		err = model.DB.Create(user).Error
		if err != nil {
			c.JSON(500, gin.H{
				"message": "创建用户失败: " + err.Error(),
				"success": false,
			})
			return
		}

		// 创建 wisemodel token
		if err := createWisemodelToken(user, req.WisemodelKey); err != nil {
			c.JSON(500, gin.H{
				"message": "创建Token失败: " + err.Error(),
				"success": false,
			})
			return
		}
	} else {
		// 用户已存在，禁用旧 token（仅当 key 发生变化时），更新 key，创建新 token
		if user.WisemodelKey != "" && user.WisemodelKey != req.WisemodelKey {
			// 忽略禁用旧 token 的错误（旧 key 可能已不存在）
			_ = model.DisableTokenByKey(user.WisemodelKey)
		}

		user.WisemodelKey = req.WisemodelKey
		err := model.DB.Save(user).Error
		if err != nil {
			c.JSON(500, gin.H{
				"message": "更新用户失败: " + err.Error(),
				"success": false,
			})
			return
		}

		if err := createWisemodelToken(user, req.WisemodelKey); err != nil {
			c.JSON(500, gin.H{
				"message": "创建Token失败: " + err.Error(),
				"success": false,
			})
			return
		}
	}

	c.JSON(200, gin.H{
		"message":    "绑定成功",
		"success":    true,
		"access_key": req.WisemodelKey,
	})
}

// DeleteWisemodelKey 删除Wisemodel-key接口
// POST /api/wisemodel/user/delete_wisemodel_key
func DeleteWisemodelKey(c *gin.Context) {
	var req struct {
		Phone string `json:"phone" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{
			"message": "参数错误: " + err.Error(),
			"success": false,
		})
		return
	}

	user := model.GetUserByPhone(req.Phone)
	if user == nil {
		c.JSON(404, gin.H{
			"message": "用户不存在",
			"success": false,
		})
		return
	}

	// 禁用关联 token
	if user.WisemodelKey != "" {
		_ = model.DisableTokenByKey(user.WisemodelKey)
	}

	// 清空wisemodel_key
	user.WisemodelKey = ""
	err := model.DB.Save(user).Error
	if err != nil {
		c.JSON(500, gin.H{
			"message": "删除失败: " + err.Error(),
			"success": false,
		})
		return
	}

	c.JSON(200, gin.H{
		"message": "删除成功",
		"success": true,
	})
}

// UpdateWisemodelKey 更新Wisemodel-key接口
// POST /api/wisemodel/user/update_wisemodel_key
func UpdateWisemodelKey(c *gin.Context) {
	var req struct {
		Phone  string `json:"phone" binding:"required"`
		NewKey string `json:"new_key" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{
			"message": "参数错误: " + err.Error(),
			"success": false,
		})
		return
	}

	user := model.GetUserByPhone(req.Phone)
	if user == nil {
		c.JSON(404, gin.H{
			"message": "用户不存在",
			"success": false,
		})
		return
	}

	// 禁用旧 token
	if user.WisemodelKey != "" {
		_ = model.DisableTokenByKey(user.WisemodelKey)
	}

	// 更新wisemodel_key
	user.WisemodelKey = req.NewKey
	err := model.DB.Save(user).Error
	if err != nil {
		c.JSON(500, gin.H{
			"message": "更新失败: " + err.Error(),
			"success": false,
		})
		return
	}

	// 创建新 token
	if err := createWisemodelToken(user, req.NewKey); err != nil {
		c.JSON(500, gin.H{
			"message": "创建Token失败: " + err.Error(),
			"success": false,
		})
		return
	}

	c.JSON(200, gin.H{
		"message": "更新成功",
		"success": true,
	})
}

// UpdatePhone 更新手机号接口
// POST /api/wisemodel/user/update_phone
func UpdatePhone(c *gin.Context) {
	var req struct {
		OldPhone string `json:"old_phone" binding:"required"`
		NewPhone string `json:"new_phone" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{
			"message": "参数错误: " + err.Error(),
			"success": false,
		})
		return
	}

	// 查找旧手机号用户
	user := model.GetUserByPhone(req.OldPhone)
	if user == nil {
		c.JSON(404, gin.H{
			"message": "用户不存在",
			"success": false,
		})
		return
	}

	// 检查新手机号是否已被使用
	existingUser := model.GetUserByPhone(req.NewPhone)
	if existingUser != nil && existingUser.Id != user.Id {
		c.JSON(400, gin.H{
			"message": "新手机号已被使用",
			"success": false,
		})
		return
	}

	// 更新手机号
	user.Phone = req.NewPhone
	err := model.DB.Save(user).Error
	if err != nil {
		c.JSON(500, gin.H{
			"message": "更新失败: " + err.Error(),
			"success": false,
		})
		return
	}

	c.JSON(200, gin.H{
		"message": "更新成功",
		"success": true,
	})
}

// DeleteWisemodelUser 取消授权接口（删除Wisemodel用户信息）
// POST /api/wisemodel/user/delete_wisemodel_user
func DeleteWisemodelUser(c *gin.Context) {
	var req struct {
		Phone string `json:"phone" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{
			"message": "参数错误: " + err.Error(),
			"success": false,
		})
		return
	}

	user := model.GetUserByPhone(req.Phone)
	if user == nil {
		c.JSON(404, gin.H{
			"message": "用户不存在",
			"success": false,
		})
		return
	}

	// 检查是否有付费订单
	hasPaid, err := model.HasPaidPackages(user.Id)
	if err != nil {
		c.JSON(500, gin.H{
			"message": "检查订单失败: " + err.Error(),
			"success": false,
		})
		return
	}

	if hasPaid {
		c.JSON(403, gin.H{
			"message": "存在付费订单，无法删除",
			"success": false,
		})
		return
	}

	// 禁用关联 token
	if user.WisemodelKey != "" {
		_ = model.DisableTokenByKey(user.WisemodelKey)
	}

	// 删除所有资源包记录
	err = model.DeleteWisemodelPackagesByUserId(user.Id)
	if err != nil {
		c.JSON(500, gin.H{
			"message": "删除资源包失败: " + err.Error(),
			"success": false,
		})
		return
	}

	// 清空wisemodel相关字段
	user.WisemodelKey = ""
	err = model.DB.Save(user).Error
	if err != nil {
		c.JSON(500, gin.H{
			"message": "删除失败: " + err.Error(),
			"success": false,
		})
		return
	}

	c.JSON(200, gin.H{
		"message": "删除成功",
		"success": true,
	})
}
