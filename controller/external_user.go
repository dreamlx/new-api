package controller

import (
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/model"

	"github.com/gin-gonic/gin"
)

// 外部用户同步请求结构
type SyncExternalUserRequest struct {
	ExternalUserId string `json:"external_user_id" binding:"required,min=1,max=100"`
	Username       string `json:"username" binding:"required,min=1,max=50"`
	DisplayName    string `json:"display_name" binding:"max=100"`
	Email          string `json:"email" binding:"omitempty,email,max=100"`
	Phone          string `json:"phone" binding:"omitempty,max=20"`
	WechatOpenId   string `json:"wechat_openid" binding:"omitempty,max=100"`
	WechatUnionId  string `json:"wechat_unionid" binding:"omitempty,max=100"`
	AlipayUserId   string `json:"alipay_userid" binding:"omitempty,max=100"`
	LoginType      string `json:"login_type" binding:"omitempty,oneof=email wechat alipay sms"`
	ExternalData   string `json:"external_data" binding:"omitempty"`
}

// 外部用户同步响应结构
type SyncExternalUserResponse struct {
	Success bool `json:"success"`
	Message string `json:"message"`
	Data    struct {
		UserId         int    `json:"user_id"`
		ExternalUserId string `json:"external_user_id"`
		IsNewUser      bool   `json:"is_new_user"`
	} `json:"data"`
}

// 外部用户充值请求结构
type ExternalUserTopUpRequest struct {
	ExternalUserId string  `json:"external_user_id" binding:"required,min=1,max=100"`
	AmountUSD      float64 `json:"amount_usd" binding:"required,min=0.01"`
	PaymentId      string  `json:"payment_id" binding:"required,min=1,max=200"`
}

// 外部用户充值响应结构
type ExternalUserTopUpResponse struct {
	Success bool `json:"success"`
	Message string `json:"message"`
	Data    struct {
		AmountUSD       float64 `json:"amount_usd"`
		QuotaAdded      int     `json:"quota_added"`
		CurrentQuota    int     `json:"current_quota"`
		CurrentBalance  float64 `json:"current_balance"`
		PaymentId       string  `json:"payment_id"`
	} `json:"data"`
}

// 外部用户Token创建请求结构
type ExternalUserTokenRequest struct {
	ExternalUserId string `json:"external_user_id" binding:"required,min=1,max=100"`
	TokenName      string `json:"token_name" binding:"required,min=1,max=100"`
	ExpiresInDays  int    `json:"expires_in_days" binding:"omitempty,min=1,max=3650"`
}

// 外部用户Token创建响应结构
type ExternalUserTokenResponse struct {
	Success bool `json:"success"`
	Message string `json:"message"`
	Data    struct {
		TokenId      int    `json:"token_id"`
		AccessKey    string `json:"access_key"`
		TokenName    string `json:"token_name"`
		ExpiresAt    int64  `json:"expires_at"`
		RemainQuota  int    `json:"remain_quota"`
	} `json:"data"`
}

// 同步外部用户到New API系统
func SyncExternalUser(c *gin.Context) {
	var req SyncExternalUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 检查external_user_id是否已存在
	existingUser := &model.User{}
	result := model.DB.Where("external_user_id = ?", req.ExternalUserId).First(existingUser)
	
	var user *model.User
	var isNewUser bool
	
	if result.Error != nil {
		// 用户不存在，创建新用户
		isNewUser = true
		
		// 生成虚拟邮箱（如果没有提供邮箱）
		email := req.Email
		if email == "" {
			email = fmt.Sprintf("%s@external.local", req.ExternalUserId)
		}
		
		// 生成默认密码（外部用户不需要密码登录）
		defaultPassword := common.GetRandomString(16)
		
		user = &model.User{
			Username:       req.Username,
			DisplayName:    req.DisplayName,
			Email:          email,
			Password:       defaultPassword,
			ExternalUserId: req.ExternalUserId,
			Phone:          req.Phone,
			WechatOpenId:   req.WechatOpenId,
			WechatUnionId:  req.WechatUnionId,
			AlipayUserId:   req.AlipayUserId,
			LoginType:      getLoginType(req.LoginType),
			IsExternal:     true,
			ExternalData:   req.ExternalData,
			Role:           common.RoleCommonUser,
			Status:         common.UserStatusEnabled,
			Quota:          common.QuotaForNewUser,
		}
		
		if err := model.DB.Create(user).Error; err != nil {
			common.SysError("创建外部用户失败: " + err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "创建用户失败",
			})
			return
		}
		
		common.SysLog(fmt.Sprintf("外部用户创建成功: %s (ID: %d)", req.ExternalUserId, user.Id))
	} else {
		// 用户已存在，更新用户信息
		isNewUser = false
		user = existingUser
		
		// 更新允许的字段
		updates := map[string]interface{}{
			"display_name":    req.DisplayName,
			"phone":           req.Phone,
			"wechat_openid":   req.WechatOpenId,
			"wechat_unionid":  req.WechatUnionId,
			"alipay_userid":   req.AlipayUserId,
			"external_data":   req.ExternalData,
		}
		
		// 只在提供了邮箱时更新邮箱
		if req.Email != "" {
			updates["email"] = req.Email
		}
		
		// 只在提供了登录类型时更新
		if req.LoginType != "" {
			updates["login_type"] = getLoginType(req.LoginType)
		}
		
		if err := model.DB.Model(user).Updates(updates).Error; err != nil {
			common.SysError("更新外部用户失败: " + err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "更新用户信息失败",
			})
			return
		}
		
		common.SysLog(fmt.Sprintf("外部用户更新成功: %s (ID: %d)", req.ExternalUserId, user.Id))
	}

	// 构造响应
	response := SyncExternalUserResponse{
		Success: true,
		Message: func() string {
			if isNewUser {
				return "用户创建成功"
			}
			return "用户信息更新成功"
		}(),
	}
	response.Data.UserId = user.Id
	response.Data.ExternalUserId = user.ExternalUserId
	response.Data.IsNewUser = isNewUser

	c.JSON(http.StatusOK, response)
}

// 为外部用户充值
func ExternalUserTopUp(c *gin.Context) {
	var req ExternalUserTopUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 查找外部用户
	user := &model.User{}
	if err := model.DB.Where("external_user_id = ?", req.ExternalUserId).First(user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "用户不存在",
		})
		return
	}

	// 计算要增加的quota（$1 USD = 500,000 quota）
	quotaToAdd := int(req.AmountUSD * common.QuotaPerUnit)
	
	// 更新用户quota
	if err := model.DB.Model(user).Update("quota", user.Quota+quotaToAdd).Error; err != nil {
		common.SysError("充值更新quota失败: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "充值失败",
		})
		return
	}

	// 创建充值记录
	topUpRecord := &model.TopUp{
		UserId:       user.Id,
		Amount:       int64(req.AmountUSD * 100), // 以分为单位存储
		Money:        req.AmountUSD,
		TradeNo:      req.PaymentId,
		CreateTime:   common.GetTimestamp(),
		CompleteTime: common.GetTimestamp(),
		Status:       "success",
	}
	
	if err := model.DB.Create(topUpRecord).Error; err != nil {
		common.SysError("创建充值记录失败: " + err.Error())
		// 这里不返回错误，因为quota已经更新成功
	}

	// 重新获取用户信息
	model.DB.First(user, user.Id)
	
	common.SysLog(fmt.Sprintf("外部用户充值成功: %s, 金额: $%.2f, 增加quota: %d", 
		req.ExternalUserId, req.AmountUSD, quotaToAdd))

	// 构造响应
	response := ExternalUserTopUpResponse{
		Success: true,
		Message: "充值成功",
	}
	response.Data.AmountUSD = req.AmountUSD
	response.Data.QuotaAdded = quotaToAdd
	response.Data.CurrentQuota = user.Quota
	response.Data.CurrentBalance = float64(user.Quota) / common.QuotaPerUnit
	response.Data.PaymentId = req.PaymentId

	c.JSON(http.StatusOK, response)
}

// 为外部用户创建Token
func CreateExternalUserToken(c *gin.Context) {
	var req ExternalUserTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 查找外部用户
	user := &model.User{}
	if err := model.DB.Where("external_user_id = ?", req.ExternalUserId).First(user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "用户不存在",
		})
		return
	}

	// 设置默认过期时间（365天）
	expiresInDays := req.ExpiresInDays
	if expiresInDays == 0 {
		expiresInDays = 365
	}

	// 创建Token
	token := &model.Token{
		UserId:        user.Id,
		Key:           common.GetRandomString(32),
		Name:          req.TokenName,
		CreatedTime:   common.GetTimestamp(),
		AccessedTime:  common.GetTimestamp(),
		ExpiredTime:   common.GetTimestamp() + int64(expiresInDays*24*3600),
		Status:        common.TokenStatusEnabled,
		RemainQuota:   user.Quota,
		UnlimitedQuota: false,
	}

	if err := model.DB.Create(token).Error; err != nil {
		common.SysError("创建Token失败: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "创建Token失败",
		})
		return
	}

	common.SysLog(fmt.Sprintf("为外部用户创建Token成功: %s, Token名称: %s", 
		req.ExternalUserId, req.TokenName))

	// 构造响应
	response := ExternalUserTokenResponse{
		Success: true,
		Message: "Token创建成功",
	}
	response.Data.TokenId = token.Id
	response.Data.AccessKey = "sk-" + token.Key
	response.Data.TokenName = token.Name
	response.Data.ExpiresAt = token.ExpiredTime
	response.Data.RemainQuota = user.Quota

	c.JSON(http.StatusOK, response)
}

// 获取外部用户统计信息
func GetExternalUserStats(c *gin.Context) {
	externalUserId := c.Param("external_user_id")
	if externalUserId == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "external_user_id参数缺失",
		})
		return
	}

	// 查找外部用户
	user := &model.User{}
	if err := model.DB.Where("external_user_id = ?", externalUserId).First(user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "用户不存在",
		})
		return
	}

	// 获取用户的Tokens
	var tokens []model.Token
	model.DB.Where("user_id = ?", user.Id).Find(&tokens)

	// 构造Token信息
	tokenInfos := make([]map[string]interface{}, 0)
	for _, token := range tokens {
		tokenInfo := map[string]interface{}{
			"id":           token.Id,
			"name":         token.Name,
			"key":          "sk-" + token.Key[:8] + "..." + token.Key[len(token.Key)-8:],
			"status":       token.Status,
			"expired_time": token.ExpiredTime,
		}
		tokenInfos = append(tokenInfos, tokenInfo)
	}

	// 计算可购买的模型数量
	balanceCapacity := calculateBalanceCapacity(user.Quota)

	// 构造响应
	response := gin.H{
		"success": true,
		"data": gin.H{
			"user_info": gin.H{
				"external_user_id": user.ExternalUserId,
				"username":         user.Username,
				"display_name":     user.DisplayName,
				"current_quota":    user.Quota,
				"current_balance":  float64(user.Quota) / common.QuotaPerUnit,
				"used_quota":       user.UsedQuota,
				"total_requests":   user.RequestCount,
				"balance_capacity": balanceCapacity,
			},
			"tokens":       tokenInfos,
			"recent_logs":  []interface{}{}, // 可以后续实现
			"model_usage":  map[string]interface{}{}, // 可以后续实现
		},
	}

	c.JSON(http.StatusOK, response)
}

// 获取模型列表和价格
func GetExternalUserModels(c *gin.Context) {
	// 这里可以根据实际需要获取模型列表
	// 暂时返回一些示例模型
	sampleModels := map[string]map[string]interface{}{
		"gpt-4": {
			"name":         "gpt-4",
			"price_per_1k": 0.03,
			"quota_per_1k": 15000,
			"description":  "GPT-4 模型",
			"billing_type": "tokens",
		},
		"gpt-3.5-turbo": {
			"name":         "gpt-3.5-turbo",
			"price_per_1k": 0.001,
			"quota_per_1k": 500,
			"description":  "GPT-3.5 Turbo 模型",
			"billing_type": "tokens",
		},
	}

	response := gin.H{
		"success": true,
		"data": gin.H{
			"models":         sampleModels,
			"quota_per_unit": common.QuotaPerUnit,
			"currency":       "USD",
		},
	}

	c.JSON(http.StatusOK, response)
}

// 辅助函数：获取登录类型
func getLoginType(loginType string) string {
	validTypes := []string{"email", "wechat", "alipay", "sms"}
	for _, valid := range validTypes {
		if loginType == valid {
			return loginType
		}
	}
	return "email" // 默认为邮箱登录
}

// 辅助函数：计算余额可购买的模型容量
func calculateBalanceCapacity(quota int) map[string]interface{} {
	capacity := make(map[string]interface{})
	
	// 示例计算
	if quota > 0 {
		capacity["gpt-4"] = map[string]interface{}{
			"tokens_1k": quota / 15000,
			"price":     0.03,
		}
		capacity["gpt-3.5-turbo"] = map[string]interface{}{
			"tokens_1k": quota / 500,
			"price":     0.001,
		}
	}
	
	return capacity
}