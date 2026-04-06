package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// PlatformQuotaForNewUser is the default quota for auto-created platform users (~$1M).
// This ensures Trust Quota Bypass works with upstream BillingSession.
const PlatformQuotaForNewUser = 499999500000

// ==================== Request Structs ====================

type V2TokenAuthorizeRequest struct {
	PlatformId string                 `json:"platform_id" binding:"required,min=1,max=100"`
	TokenKey   string                 `json:"token_key" binding:"required,min=10,max=200"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// ==================== Handlers ====================

// V2AuthorizeToken registers a platform-generated token in New API.
// POST /api/v2/external/tokens/authorize
func V2AuthorizeToken(c *gin.Context) {
	var req V2TokenAuthorizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请求参数错误: " + err.Error()})
		return
	}

	if !strings.HasPrefix(req.TokenKey, "sk-") {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "token_key 必须以 sk- 开头"})
		return
	}

	// Validate token key format: no extra dashes after sk-
	tokenKeyBody := strings.TrimPrefix(req.TokenKey, "sk-")
	if strings.Contains(tokenKeyBody, "-") {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "token_key 格式错误：sk- 后不能包含 - 字符，推荐使用32-48位字母数字组合",
		})
		return
	}

	// Find or create platform user
	platformUsername := "platform_" + req.PlatformId
	user, err := getOrCreatePlatformUser(platformUsername, req.PlatformId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "创建平台用户失败: " + err.Error()})
		return
	}

	// Check if token already exists (idempotent)
	existingToken, err := model.GetTokenByKey(tokenKeyBody, true)
	if err == nil && existingToken != nil {
		// Token exists — verify it belongs to this platform
		if existingToken.UserId != user.Id {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "该密钥已被其他平台注册"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "密钥已存在",
			"data": gin.H{
				"token_key":   req.TokenKey,
				"token_id":    existingToken.Id,
				"status":      "exists",
				"platform_id": req.PlatformId,
				"created_at":  time.Unix(existingToken.CreatedTime, 0).UTC().Format(time.RFC3339),
			},
		})
		return
	}

	// Build metadata name
	tokenName := fmt.Sprintf("v2_%s", req.PlatformId)
	if req.Metadata != nil {
		if uid, ok := req.Metadata["platform_user_id"]; ok {
			tokenName = fmt.Sprintf("v2_%s_%v", req.PlatformId, uid)
		}
	}

	// Store metadata as external_data JSON
	var metadataStr string
	if req.Metadata != nil {
		if bs, err := common.Marshal(req.Metadata); err == nil {
			metadataStr = string(bs)
		}
	}

	token := &model.Token{
		UserId:         user.Id,
		Key:            tokenKeyBody,
		Name:           tokenName,
		CreatedTime:    common.GetTimestamp(),
		AccessedTime:   common.GetTimestamp(),
		ExpiredTime:    -1, // never expires
		Status:         common.TokenStatusEnabled,
		RemainQuota:    0,
		UnlimitedQuota: true,
	}

	if err := model.DB.Create(token).Error; err != nil {
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "密钥已存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "创建Token失败: " + err.Error()})
		return
	}

	// Store metadata in log for audit (best-effort)
	if metadataStr != "" {
		_ = model.LOG_DB.Create(&model.Log{
			UserId:    user.Id,
			Username:  user.Username,
			CreatedAt: common.GetTimestamp(),
			Type:      model.LogTypeManage,
			Content:   fmt.Sprintf("V2平台Token授权，platform: %s, metadata: %s", req.PlatformId, metadataStr),
			TokenId:   token.Id,
		}).Error
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "密钥授权成功",
		"data": gin.H{
			"token_key":   req.TokenKey,
			"token_id":    token.Id,
			"status":      "authorized",
			"platform_id": req.PlatformId,
			"created_at":  time.Unix(token.CreatedTime, 0).UTC().Format(time.RFC3339),
		},
	})
}

// V2GetPlatformLogs returns consumption logs for a platform.
// GET /api/v2/external/platforms/:platform_id/logs
func V2GetPlatformLogs(c *gin.Context) {
	platformId := c.Param("platform_id")
	if platformId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "platform_id 不能为空"})
		return
	}

	platformUsername := "platform_" + platformId
	var user model.User
	if err := model.DB.Where("username = ?", platformUsername).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "平台不存在"})
		return
	}

	// Parse query params
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	tokenKeyFilter := c.Query("token_key")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

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

	// Build query
	tx := model.LOG_DB.Where("user_id = ? AND type = ?", user.Id, model.LogTypeConsume)

	// Optional token_key filter
	if tokenKeyFilter != "" {
		tokenKeyBody := strings.TrimPrefix(tokenKeyFilter, "sk-")
		filterToken, err := model.GetTokenByKey(tokenKeyBody, true)
		if err != nil || filterToken == nil || filterToken.UserId != user.Id {
			// Token not found or doesn't belong to this platform — return empty results
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"platform_id": platformId,
					"logs":        []interface{}{},
					"pagination":  gin.H{"page": page, "page_size": pageSize, "total": 0, "total_pages": 0},
					"summary":     gin.H{"total_requests": 0, "total_tokens": 0, "total_quota_consumed": 0},
				},
			})
			return
		}
		tx = tx.Where("token_id = ?", filterToken.Id)
	}

	if startTimestamp > 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp > 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}

	// Count
	var total int64
	if err := tx.Model(&model.Log{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询失败"})
		return
	}

	// Fetch logs
	var logs []*model.Log
	offset := (page - 1) * pageSize
	if err := tx.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询失败"})
		return
	}

	// Build response
	var totalPromptTokens, totalCompletionTokens, totalQuotaConsumed int
	uniqueTokens := make(map[int]bool)
	logItems := make([]gin.H, 0, len(logs))

	for _, log := range logs {
		totalPromptTokens += log.PromptTokens
		totalCompletionTokens += log.CompletionTokens
		totalQuotaConsumed += log.Quota

		// Resolve full token_key
		var tokenKey string
		if log.TokenId > 0 {
			uniqueTokens[log.TokenId] = true
			if t, err := model.GetTokenById(log.TokenId); err == nil {
				tokenKey = "sk-" + t.Key
			}
		}

		logItems = append(logItems, gin.H{
			"log_id":            log.Id,
			"time":              time.Unix(log.CreatedAt, 0).UTC().Format(time.RFC3339),
			"token_key":         tokenKey,
			"model_name":        log.ModelName,
			"prompt_tokens":     log.PromptTokens,
			"completion_tokens": log.CompletionTokens,
			"total_tokens":      log.PromptTokens + log.CompletionTokens,
			"quota_cost":        log.Quota,
		})
	}

	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"platform_id": platformId,
			"logs":        logItems,
			"pagination": gin.H{
				"page":        page,
				"page_size":   pageSize,
				"total":       total,
				"total_pages": totalPages,
			},
			"summary": gin.H{
				"total_requests":          len(logItems),
				"total_prompt_tokens":     totalPromptTokens,
				"total_completion_tokens": totalCompletionTokens,
				"total_tokens":            totalPromptTokens + totalCompletionTokens,
				"total_quota_consumed":    totalQuotaConsumed,
				"unique_tokens":           len(uniqueTokens),
			},
		},
	})
}

// ==================== Helpers ====================

func getOrCreatePlatformUser(username, platformId string) (*model.User, error) {
	var user model.User
	if err := model.DB.Where("username = ?", username).First(&user).Error; err == nil {
		return &user, nil
	}

	email := fmt.Sprintf("%s@platform.local", platformId)
	user = model.User{
		Username:       username,
		DisplayName:    "Platform: " + platformId,
		Email:          email,
		Password:       common.GetRandomString(32),
		AffCode:        common.GetRandomString(16),
		ExternalUserId: "platform_" + platformId,
		IsExternal:     true,
		Role:           common.RoleCommonUser,
		Status:         common.UserStatusEnabled,
		Quota:          PlatformQuotaForNewUser,
	}

	if err := model.DB.Create(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
