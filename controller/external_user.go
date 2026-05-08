package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ==================== Request/Response Structs ====================

type SyncExternalUserRequest struct {
	ExternalUserId string `json:"external_user_id" binding:"required,min=1,max=100"`
	Username       string `json:"username" binding:"required,min=1,max=50"`
	DisplayName    string `json:"display_name" binding:"max=100"`
	Email          string `json:"email" binding:"omitempty,email,max=100"`
	Phone          string `json:"phone" binding:"omitempty,max=20"`
	WechatUnionId  string `json:"wechat_unionid" binding:"omitempty,max=100"`
	AlipayUserId   string `json:"alipay_userid" binding:"omitempty,max=100"`
	LoginType      string `json:"login_type" binding:"omitempty,oneof=email wechat alipay sms google"`
	AffCode        string `json:"aff_code" binding:"omitempty,max=32"`
	ExternalData   string `json:"external_data" binding:"omitempty"`
}

type ExternalUserTopUpRequest struct {
	ExternalUserId string  `json:"external_user_id" binding:"required,min=1,max=100"`
	AmountUSD      float64 `json:"amount_usd" binding:"required,min=0.01"`
	PaymentId      string  `json:"payment_id" binding:"required,min=1,max=200"`
}

type ExternalUserDeductRequest struct {
	ExternalUserId string  `json:"external_user_id" binding:"required,min=1,max=100"`
	AmountUSD      float64 `json:"amount_usd" binding:"required,min=0.01"`
	Reason         string  `json:"reason" binding:"required,min=1,max=200"`
	AdminNote      string  `json:"admin_note" binding:"omitempty,max=500"`
}

type ExternalUserTokenRequest struct {
	ExternalUserId  string   `json:"external_user_id" binding:"required,min=1,max=100"`
	TokenName       string   `json:"token_name" binding:"required,min=1,max=100"`
	AllocatedQuota  int      `json:"allocated_quota"` // 独立额度模式必填（非无限额度时）
	UnlimitedQuota  bool     `json:"unlimited_quota"` // 无限额度模式（消耗用户余额）
	ExpiresInDays   int      `json:"expires_in_days" binding:"omitempty,min=1,max=3650"`
	ModelLimits     []string `json:"model_limits" binding:"omitempty"`
	Group           string   `json:"group" binding:"omitempty,max=50"`
	CallbackUrl     string   `json:"callback_url" binding:"omitempty,max=500"`
	CallbackEnabled bool     `json:"callback_enabled"`
	CallbackSecret  string   `json:"callback_secret" binding:"omitempty,max=64"`
}

type VerifyTokenRequest struct {
	AccessKey string `json:"access_key" binding:"required"`
}

// ==================== Handlers ====================

// SyncExternalUser creates or updates an external user mapping.
// POST /api/user/external/sync
func SyncExternalUser(c *gin.Context) {
	var req SyncExternalUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请求参数错误: " + err.Error()})
		return
	}

	// Check if external_user_id already exists
	existingUser, err := model.GetUserByExternalId(req.ExternalUserId)
	if err == nil && existingUser.Id > 0 {
		// Update existing user
		existingUser.Username = req.Username
		existingUser.DisplayName = req.DisplayName
		if req.Email != "" {
			existingUser.Email = req.Email
		}
		existingUser.Phone = req.Phone
		existingUser.WechatUnionId = req.WechatUnionId
		existingUser.AlipayUserId = req.AlipayUserId
		existingUser.LoginType = getLoginType(req.LoginType)
		existingUser.ExternalData = req.ExternalData

		if err := model.DB.Save(existingUser).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "更新用户信息失败"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "用户信息同步成功",
			"data": gin.H{
				"user_id":          existingUser.Id,
				"external_user_id": existingUser.ExternalUserId,
				"is_new_user":      false,
			},
		})
		return
	}

	// Check username/email collision
	exist, err := model.CheckUserExistOrDeleted(req.Username, req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "系统错误，请稍后重试"})
		return
	}
	if exist {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "用户名或邮箱已存在"})
		return
	}

	// Create new user
	email := req.Email
	if email == "" {
		email = fmt.Sprintf("%s@external.local", req.ExternalUserId)
	}

	user := &model.User{
		Username:       req.Username,
		DisplayName:    req.DisplayName,
		Email:          email,
		Password:       common.GetRandomString(16),
		ExternalUserId: req.ExternalUserId,
		Phone:          req.Phone,
		WechatUnionId:  req.WechatUnionId,
		AlipayUserId:   req.AlipayUserId,
		LoginType:      getLoginType(req.LoginType),
		IsExternal:     true,
		ExternalData:   req.ExternalData,
		Role:           common.RoleCommonUser,
		Status:         common.UserStatusEnabled,
		Quota:          common.QuotaForNewUser,
	}
	if req.AffCode != "" {
		user.AffCode = req.AffCode
	} else {
		user.AffCode = common.GetRandomString(4)
	}

	if err := model.DB.Create(user).Error; err != nil {
		errorMsg := "创建用户失败"
		if strings.Contains(err.Error(), "Duplicate entry") || strings.Contains(err.Error(), "UNIQUE constraint failed") {
			if strings.Contains(err.Error(), "username") {
				errorMsg = "用户名已存在，请使用其他用户名"
			} else if strings.Contains(err.Error(), "email") {
				errorMsg = "邮箱已被使用，请使用其他邮箱"
			} else if strings.Contains(err.Error(), "aff_code") {
				errorMsg = "推荐码已被使用，请使用其他推荐码"
			}
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":      false,
			"message":      errorMsg,
			"error_detail": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "用户创建成功",
		"data": gin.H{
			"user_id":          user.Id,
			"external_user_id": user.ExternalUserId,
			"is_new_user":      true,
		},
	})
}

// ExternalUserTopUp adds quota to an external user.
// POST /api/user/external/topup
func ExternalUserTopUp(c *gin.Context) {
	var req ExternalUserTopUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请求参数错误: " + err.Error()})
		return
	}

	user, err := model.GetUserByExternalId(req.ExternalUserId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "用户不存在"})
		return
	}

	quotaToAdd := int(req.AmountUSD * common.QuotaPerUnit)

	tx := model.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Idempotency: TopUp.TradeNo has unique constraint — duplicate insert fails
	topUpRecord := &model.TopUp{
		UserId:       user.Id,
		Amount:       int64(req.AmountUSD * 100),
		Money:        req.AmountUSD,
		TradeNo:      req.PaymentId,
		CreateTime:   common.GetTimestamp(),
		CompleteTime: common.GetTimestamp(),
		Status:       "success",
	}
	if err := tx.Create(topUpRecord).Error; err != nil {
		tx.Rollback()
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "该支付ID已处理，请勿重复提交"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "创建充值记录失败"})
		return
	}

	// Atomic quota update
	if err := tx.Model(&model.User{}).Where("id = ?", user.Id).
		Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "充值失败"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "事务提交失败"})
		return
	}

	// Re-fetch user for response
	model.DB.First(user, user.Id)

	// Log (best-effort, outside transaction)
	topupLog := &model.Log{
		UserId:    user.Id,
		Username:  user.Username,
		CreatedAt: common.GetTimestamp(),
		Type:      model.LogTypeTopup,
		Content: fmt.Sprintf("外部用户充值成功，充值金额: $%.2f，增加额度: %d quota，支付ID: %s",
			req.AmountUSD, quotaToAdd, req.PaymentId),
		Quota: quotaToAdd,
	}
	_ = model.LOG_DB.Create(topupLog).Error

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "充值成功",
		"data": gin.H{
			"amount_usd":      req.AmountUSD,
			"quota_added":     quotaToAdd,
			"current_quota":   user.Quota,
			"current_balance": float64(user.Quota) / common.QuotaPerUnit,
			"payment_id":     req.PaymentId,
		},
	})
}

// ExternalUserDeduct subtracts quota from an external user.
// POST /api/user/external/deduct
func ExternalUserDeduct(c *gin.Context) {
	var req ExternalUserDeductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请求参数错误: " + err.Error()})
		return
	}

	user, err := model.GetUserByExternalId(req.ExternalUserId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "用户不存在"})
		return
	}

	quotaToDeduct := int(req.AmountUSD * common.QuotaPerUnit)

	// Atomic deduct with SQL-level balance check
	result := model.DB.Model(&model.User{}).
		Where("id = ? AND quota >= ?", user.Id, quotaToDeduct).
		Update("quota", gorm.Expr("quota - ?", quotaToDeduct))
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "扣除余额失败"})
		return
	}
	if result.RowsAffected == 0 {
		model.DB.First(user, user.Id)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": fmt.Sprintf("用户余额不足，当前余额: %d quota ($%.2f), 请求扣除: %d quota ($%.2f)",
				user.Quota, float64(user.Quota)/common.QuotaPerUnit, quotaToDeduct, req.AmountUSD),
		})
		return
	}

	model.DB.First(user, user.Id)

	deductLog := &model.Log{
		UserId:    user.Id,
		Username:  user.Username,
		CreatedAt: common.GetTimestamp(),
		Type:      model.LogTypeManage,
		Content: fmt.Sprintf("管理员扣除余额，扣除金额: $%.2f，扣除额度: %d quota，原因: %s，备注: %s",
			req.AmountUSD, quotaToDeduct, req.Reason, req.AdminNote),
		Quota: -quotaToDeduct,
	}
	_ = model.LOG_DB.Create(deductLog).Error

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "余额扣除成功",
		"data": gin.H{
			"amount_usd":      req.AmountUSD,
			"quota_deducted":  quotaToDeduct,
			"current_quota":   user.Quota,
			"current_balance": float64(user.Quota) / common.QuotaPerUnit,
			"reason":          req.Reason,
		},
	})
}

// CreateExternalUserToken creates a token with allocated quota.
// POST /api/user/external/token
func CreateExternalUserToken(c *gin.Context) {
	var req ExternalUserTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请求参数错误: " + err.Error()})
		return
	}

	user, err := model.GetUserByExternalId(req.ExternalUserId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "用户不存在"})
		return
	}

	// Validate group if specified
	if req.Group != "" && !ratio_setting.ContainsGroupRatio(req.Group) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("分组 %s 不存在", req.Group)})
		return
	}

	// Validate: non-unlimited mode must provide allocated_quota > 0
	if !req.UnlimitedQuota && req.AllocatedQuota <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "独立额度模式必须提供allocated_quota参数"})
		return
	}

	expiresInDays := req.ExpiresInDays
	if expiresInDays == 0 {
		expiresInDays = 365
	}

	tx := model.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Atomic deduct from user quota — only in independent quota mode
	if !req.UnlimitedQuota {
		deductResult := tx.Model(&model.User{}).
			Where("id = ? AND quota >= ?", user.Id, req.AllocatedQuota).
			Update("quota", gorm.Expr("quota - ?", req.AllocatedQuota))
		if deductResult.Error != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "扣减用户额度失败"})
			return
		}
		if deductResult.RowsAffected == 0 {
			tx.Rollback()
			model.DB.First(user, user.Id)
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": fmt.Sprintf("用户余额不足，当前余额: %d，请求分配: %d", user.Quota, req.AllocatedQuota),
			})
			return
		}
	}

	// Create token
	token := &model.Token{
		UserId:          user.Id,
		Key:             common.GetRandomString(32),
		Name:            req.TokenName,
		CreatedTime:     common.GetTimestamp(),
		AccessedTime:    common.GetTimestamp(),
		ExpiredTime:     common.GetTimestamp() + int64(expiresInDays*24*3600),
		Status:          common.TokenStatusEnabled,
		RemainQuota:     req.AllocatedQuota, // 0 for unlimited mode
		UnlimitedQuota:  req.UnlimitedQuota,
		Group:           req.Group,
		CallbackUrl:     req.CallbackUrl,
		CallbackEnabled: req.CallbackEnabled,
		CallbackSecret:  req.CallbackSecret,
	}

	// Set model limits if specified
	if len(req.ModelLimits) > 0 {
		token.ModelLimitsEnabled = true
		token.ModelLimits = strings.Join(req.ModelLimits, ",")
	}

	if err := tx.Create(token).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "创建Token失败"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "事务提交失败"})
		return
	}

	data := gin.H{
		"token_id":     token.Id,
		"access_key":   "sk-" + token.Key,
		"token_name":   token.Name,
		"expires_at":   token.ExpiredTime,
		"remain_quota": req.AllocatedQuota,
	}
	if req.UnlimitedQuota {
		data["unlimited_quota"] = true
	}
	if token.Group != "" {
		data["group"] = token.Group
	}
	if token.ModelLimitsEnabled {
		data["model_limits"] = req.ModelLimits
	}
	if token.CallbackEnabled && token.CallbackUrl != "" {
		data["callback_enabled"] = true
		data["callback_url_masked"] = maskCallbackUrl(token.CallbackUrl)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Token创建成功",
		"data":    data,
	})
}

// DeleteExternalUserToken soft-deletes a token. Remaining quota is NOT refunded.
// DELETE /api/user/external/:external_user_id/token/:token_id
func DeleteExternalUserToken(c *gin.Context) {
	externalUserId := c.Param("external_user_id")
	tokenIdStr := c.Param("token_id")

	if externalUserId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "缺少external_user_id参数"})
		return
	}
	tokenId, err := strconv.Atoi(tokenIdStr)
	if err != nil || tokenId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的token_id参数"})
		return
	}

	user, err := model.GetUserByExternalId(externalUserId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "用户不存在"})
		return
	}

	token := &model.Token{}
	if err := model.DB.Where("id = ? AND user_id = ?", tokenId, user.Id).First(token).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Token不存在或无权删除"})
		return
	}

	// Soft-delete token (no quota refund — remaining quota stays consumed)
	if err := model.DB.Delete(token).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "删除Token失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Token删除成功",
		"data": gin.H{
			"token_id":         tokenId,
			"external_user_id": externalUserId,
		},
	})
}

// GetExternalUserTokens lists all tokens for an external user.
// GET /api/user/external/:external_user_id/tokens
func GetExternalUserTokens(c *gin.Context) {
	externalUserId := c.Param("external_user_id")
	if externalUserId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "external_user_id 不能为空"})
		return
	}

	user, err := model.GetUserByExternalId(externalUserId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "用户不存在"})
		return
	}

	var tokens []model.Token
	if err := model.DB.Where("user_id = ?", user.Id).Order("created_time DESC").Find(&tokens).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询Token列表失败"})
		return
	}

	currentTime := common.GetTimestamp()
	tokenInfos := make([]gin.H, 0, len(tokens))

	for _, token := range tokens {
		maskedKey := fmt.Sprintf("sk-%s****%s", token.Key[:min(8, len(token.Key))], token.Key[max(0, len(token.Key)-4):])

		statusText := "启用"
		switch token.Status {
		case common.TokenStatusDisabled:
			statusText = "禁用"
		case common.TokenStatusExhausted:
			statusText = "额度耗尽"
		case common.TokenStatusExpired:
			statusText = "已过期"
		}

		isExpired := token.ExpiredTime != -1 && token.ExpiredTime < currentTime

		tokenInfos = append(tokenInfos, gin.H{
			"token_id":      token.Id,
			"token_name":    token.Name,
			"access_key":    maskedKey,
			"status":        token.Status,
			"status_text":   statusText,
			"remain_quota":  token.RemainQuota,
			"used_quota":    token.UsedQuota,
			"created_time":  token.CreatedTime,
			"expired_time":  token.ExpiredTime,
			"accessed_time": token.AccessedTime,
			"is_expired":    isExpired,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "查询成功",
		"data": gin.H{
			"external_user_id": externalUserId,
			"total_tokens":     len(tokens),
			"tokens":           tokenInfos,
		},
	})
}

// VerifyExternalUserToken checks if a token is valid.
// POST /api/user/external/token/verify
func VerifyExternalUserToken(c *gin.Context) {
	var req VerifyTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请求参数错误: " + err.Error()})
		return
	}

	tokenKey := strings.TrimPrefix(req.AccessKey, "sk-")
	token, err := model.GetTokenByKey(tokenKey, false)
	if err != nil && strings.HasPrefix(req.AccessKey, "sk-") {
		token, err = model.GetTokenByKey(req.AccessKey, false)
	}

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "验证完成",
			"data":    gin.H{"is_valid": false, "error_reason": "Token不存在"},
		})
		return
	}

	user := &model.User{}
	if err := model.DB.First(user, token.UserId).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "验证完成",
			"data":    gin.H{"is_valid": false, "error_reason": "关联用户不存在"},
		})
		return
	}

	currentTime := common.GetTimestamp()
	isValid := true
	errorReason := ""

	switch {
	case token.Status == common.TokenStatusDisabled:
		isValid, errorReason = false, "Token已被禁用"
	case token.Status == common.TokenStatusExhausted:
		isValid, errorReason = false, "Token额度已耗尽"
	case token.Status == common.TokenStatusExpired:
		isValid, errorReason = false, "Token已过期（状态标记）"
	case token.ExpiredTime != -1 && token.ExpiredTime < currentTime:
		isValid, errorReason = false, "Token已过期（时间到期）"
	case !token.UnlimitedQuota && token.RemainQuota <= 0:
		isValid, errorReason = false, "Token剩余额度不足"
	}

	statusText := "启用"
	switch token.Status {
	case common.TokenStatusDisabled:
		statusText = "禁用"
	case common.TokenStatusExhausted:
		statusText = "额度耗尽"
	case common.TokenStatusExpired:
		statusText = "已过期"
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "验证完成",
		"data": gin.H{
			"is_valid":         isValid,
			"token_id":         token.Id,
			"token_name":       token.Name,
			"external_user_id": user.ExternalUserId,
			"status":           token.Status,
			"status_text":      statusText,
			"remain_quota":     token.RemainQuota,
			"expired_time":     token.ExpiredTime,
			"is_expired":       token.ExpiredTime != -1 && token.ExpiredTime < currentTime,
			"error_reason":     errorReason,
		},
	})
}

// GetExternalUserLogs returns consumption logs for an external user.
// GET /api/user/external/:external_user_id/logs
func GetExternalUserLogs(c *gin.Context) {
	externalUserId := c.Param("external_user_id")
	if externalUserId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "用户ID参数缺失"})
		return
	}

	user, err := model.GetUserByExternalId(externalUserId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "用户不存在"})
		return
	}

	// Parse query params
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	modelName := c.Query("model_name")

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var startTimestamp, endTimestamp int64
	if startDate != "" {
		if t, err := time.Parse("2006-01-02", startDate); err == nil {
			startTimestamp = t.Unix()
		}
	}
	if endDate != "" {
		if t, err := time.Parse("2006-01-02", endDate); err == nil {
			endTimestamp = t.Add(24*time.Hour - time.Second).Unix()
		}
	}

	tx := model.LOG_DB.Where("user_id = ? AND (type = ? OR type = ? OR type = ?)",
		user.Id, model.LogTypeConsume, model.LogTypeTopup, model.LogTypeManage)
	if modelName != "" {
		tx = tx.Where("model_name LIKE ?", "%"+modelName+"%")
	}
	if startTimestamp > 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp > 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}

	var total int64
	if err := tx.Model(&model.Log{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询失败"})
		return
	}

	var logs []*model.Log
	offset := (page - 1) * pageSize
	if err := tx.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询失败"})
		return
	}

	var totalTokens int
	var totalSpend float64
	logItems := make([]gin.H, 0, len(logs))

	for _, log := range logs {
		tokens := log.PromptTokens + log.CompletionTokens
		totalTokens += tokens
		spend := float64(log.Quota) / common.QuotaPerUnit

		logType := "consume"
		if log.Type == model.LogTypeTopup {
			logType = "topup"
			spend = -spend
		} else if log.Type == model.LogTypeManage {
			logType = "deduct"
		}
		totalSpend += spend

		var tokenKey, tokenName string
		var tokenId int
		if log.TokenId > 0 {
			if t, err := model.GetTokenById(log.TokenId); err == nil {
				tokenId = t.Id
				tokenName = t.Name
				tokenKey = t.Key
				if !strings.HasPrefix(tokenKey, "sk-") {
					tokenKey = "sk-" + tokenKey
				}
			}
		}

		logItems = append(logItems, gin.H{
			"time":       time.Unix(log.CreatedAt, 0).Format("2006-01-02 15:04:05"),
			"username":   log.Username,
			"token_key":  tokenKey,
			"token_name": tokenName,
			"token_id":   tokenId,
			"tokens":     tokens,
			"type":       logType,
			"model":      log.ModelName,
			"spend":      spend,
		})
	}

	totalPage := int((total + int64(pageSize) - 1) / int64(pageSize))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"logs": logItems,
			"pagination": gin.H{
				"page":       page,
				"page_size":  pageSize,
				"total":      total,
				"total_page": totalPage,
			},
			"summary": gin.H{
				"total_tokens": totalTokens,
				"total_spend":  totalSpend,
			},
		},
	})
}

// GetExternalUserStats returns user info and balance capacity.
// GET /api/user/external/:external_user_id/stats
func GetExternalUserStats(c *gin.Context) {
	externalUserId := c.Param("external_user_id")
	if externalUserId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "用户ID参数缺失"})
		return
	}

	user, err := model.GetUserByExternalId(externalUserId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "用户不存在"})
		return
	}

	var tokens []model.Token
	model.DB.Where("user_id = ?", user.Id).Find(&tokens)

	tokenInfos := make([]gin.H, 0, len(tokens))
	for _, token := range tokens {
		tokenInfos = append(tokenInfos, gin.H{
			"id":           token.Id,
			"name":         token.Name,
			"key":          "sk-" + token.Key,
			"status":       token.Status,
			"expired_time": token.ExpiredTime,
		})
	}

	// Use user's group for balance capacity calculation
	userGroup := user.Group
	if userGroup == "" {
		userGroup = "default"
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"user_info": gin.H{
				"external_user_id": user.ExternalUserId,
				"username":         user.Username,
				"display_name":     user.DisplayName,
				"group":            userGroup,
				"current_quota":    user.Quota,
				"current_balance":  float64(user.Quota) / common.QuotaPerUnit,
				"used_quota":       user.UsedQuota,
				"total_requests":   user.RequestCount,
				"balance_capacity": calculateBalanceCapacity(user.Quota, userGroup),
			},
			"tokens":      tokenInfos,
			"recent_logs": []interface{}{},
			"model_usage": map[string]interface{}{},
		},
	})
}

// GetExternalUserModels returns available models and pricing.
// GET /api/user/external/models
func GetExternalUserModels(c *gin.Context) {
	group := c.DefaultQuery("group", "default")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"models":         calculateBalanceCapacity(int(common.QuotaPerUnit), group), // show pricing for $1
			"quota_per_unit": common.QuotaPerUnit,
			"currency":       "USD",
			"group":          group,
			"group_ratio":    ratio_setting.GetGroupRatio(group),
		},
	})
}

// DeleteExternalUser deactivates an external user (soft delete).
// DELETE /api/user/external/:external_user_id
func DeleteExternalUser(c *gin.Context) {
	externalUserId := c.Param("external_user_id")
	if externalUserId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "external_user_id 不能为空"})
		return
	}

	user, tokensDisabled, err := DeactivateExternalUser(externalUserId)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "用户不存在") {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "用户不存在"})
			return
		}
		if strings.Contains(errMsg, "用户已注销") {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "用户已注销"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "注销用户失败: " + errMsg})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "用户已注销",
		"data": gin.H{
			"user_id":                   user.Id,
			"original_external_user_id": externalUserId,
			"deleted_external_user_id":  user.ExternalUserId,
			"tokens_disabled":           tokensDisabled,
			"deleted_at":                time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// ==================== Helper Functions ====================

func getLoginType(loginType string) string {
	switch loginType {
	case "email", "wechat", "alipay", "sms", "google":
		return loginType
	default:
		return "email"
	}
}

func maskCallbackUrl(url string) string {
	if idx := strings.Index(url, "//"); idx != -1 {
		afterSlash := url[idx+2:]
		if pathIdx := strings.Index(afterSlash, "/"); pathIdx != -1 {
			return url[:idx+2+pathIdx] + "/***"
		}
		return url
	}
	return "***"
}

func calculateBalanceCapacity(quota int, group string) map[string]interface{} {
	capacity := make(map[string]interface{})
	if quota <= 0 {
		return capacity
	}

	groupRatio := ratio_setting.GetGroupRatio(group)

	// Query enabled channels
	var channels []struct {
		Models    string `json:"models"`
		TestModel string `json:"test_model"`
	}
	if err := model.DB.Table("channels").
		Select("models, test_model").
		Where("status = ?", 1).
		Find(&channels).Error; err != nil {
		return capacity
	}

	modelSet := make(map[string]bool)
	testModels := make(map[string]bool)
	for _, ch := range channels {
		if ch.TestModel != "" {
			tm := strings.TrimSpace(ch.TestModel)
			if tm != "" {
				modelSet[tm] = true
				testModels[tm] = true
			}
		}
		if ch.Models != "" {
			var models []string
			if err := common.Unmarshal([]byte(ch.Models), &models); err != nil {
				models = strings.Split(ch.Models, ",")
			}
			for _, m := range models {
				m = strings.TrimSpace(m)
				if m != "" {
					modelSet[m] = true
				}
			}
		}
	}

	modelCount := 0

	// Process test models first, then others
	var modelList []string
	for m := range testModels {
		modelList = append(modelList, m)
	}
	for m := range modelSet {
		if !testModels[m] {
			modelList = append(modelList, m)
		}
	}

	for _, modelName := range modelList {
		modelRatio, exists, _ := ratio_setting.GetModelRatio(modelName)
		if !exists {
			continue
		}
		completionRatio := ratio_setting.GetCompletionRatio(modelName)
		basePrice := modelRatio * 0.002
		inputQuotaPer1K := int(groupRatio*modelRatio*1000 + 0.5)
		if inputQuotaPer1K <= 0 {
			continue
		}
		maxInputTokens1K := quota / inputQuotaPer1K
		if maxInputTokens1K > 0 {
			info := map[string]interface{}{
				"input_tokens_1k":    maxInputTokens1K,
				"model_ratio":        modelRatio,
				"completion_ratio":   completionRatio,
				"group_ratio":        groupRatio,
				"base_price_usd":     basePrice,
				"quota_per_1k_input": inputQuotaPer1K,
				"pricing_note": fmt.Sprintf("输入：%d quota/1K tokens，输出：%d quota/1K tokens",
					inputQuotaPer1K, int(float64(inputQuotaPer1K)*completionRatio)),
			}
			if testModels[modelName] {
				info["is_default_model"] = true
			}
			capacity[modelName] = info
			modelCount++
			if modelCount >= 8 {
				break
			}
		}
	}

	capacity["_summary"] = map[string]interface{}{
		"total_balance_usd": float64(quota) / common.QuotaPerUnit,
		"total_quota":       quota,
		"quota_per_usd":     common.QuotaPerUnit,
		"group":             group,
		"group_ratio":       groupRatio,
		"billing_formula":   "消耗quota = 分组倍率 × 模型倍率 × (输入tokens + 输出tokens × 补全倍率)",
		"models_available":  modelCount,
		"note":              "实际消费取决于输入和输出token数量，此处仅显示输入token的估算",
	}

	return capacity
}

// DeactivateExternalUser soft-deletes an external user and disables all tokens.
func DeactivateExternalUser(externalUserId string) (*model.User, int, error) {
	if externalUserId == "" {
		return nil, 0, fmt.Errorf("external_user_id 为空")
	}

	user, err := model.GetUserByExternalId(externalUserId)
	if err != nil {
		return nil, 0, fmt.Errorf("用户不存在: %w", err)
	}

	// Check if already deactivated
	if user.Status == common.UserStatusDisabled {
		var deletedUser model.User
		if err := model.DB.Unscoped().Where("id = ? AND deleted_at IS NOT NULL", user.Id).First(&deletedUser).Error; err == nil {
			return nil, 0, fmt.Errorf("用户已注销")
		}
	}

	timestamp := common.GetTimestamp()

	// Release unique fields
	user.Username = fmt.Sprintf("deleted_%s_%d", user.Username, timestamp)
	user.ExternalUserId = fmt.Sprintf("deleted_%s_%d", externalUserId, timestamp)
	user.AccessToken = nil
	user.AffCode = fmt.Sprintf("del_%d", timestamp) // avoid unique constraint

	// Clear linked fields
	user.WechatOpenId = ""
	user.WechatUnionId = ""
	user.AlipayUserId = ""
	user.Phone = ""
	user.Email = ""
	user.Status = common.UserStatusDisabled

	tx := model.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Save(user).Error; err != nil {
		tx.Rollback()
		return nil, 0, fmt.Errorf("更新用户失败: %w", err)
	}

	// Disable all tokens
	result := tx.Model(&model.Token{}).Where("user_id = ? AND status = ?", user.Id, common.TokenStatusEnabled).
		Update("status", common.TokenStatusDisabled)
	if result.Error != nil {
		tx.Rollback()
		return nil, 0, fmt.Errorf("禁用Token失败: %w", result.Error)
	}

	if err := tx.Delete(user).Error; err != nil {
		tx.Rollback()
		return nil, 0, fmt.Errorf("软删除用户失败: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, 0, fmt.Errorf("事务提交失败: %w", err)
	}

	return user, int(result.RowsAffected), nil
}
