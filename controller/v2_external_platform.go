package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// ==================== Request Structs ====================

// V2TokenAuthorizeRequest is the body schema for POST /api/v2/external/tokens/authorize.
// platform_id is intentionally absent: caller identity is established by the
// PlatformAuth middleware via X-Platform-Id / X-Platform-Sk headers. Any
// platform_id sent in the body is ignored.
type V2TokenAuthorizeRequest struct {
	TokenKey string                 `json:"token_key" binding:"required,min=10,max=200"`
	Metadata map[string]interface{} `json:"metadata"`
}

// ==================== Handlers ====================

// V2AuthorizeToken registers a platform-generated token key in New API and
// returns the internal token_id. Idempotent: re-registering an existing key
// (under the same platform) returns the same token_id with status="exists".
//
// POST /api/v2/external/tokens/authorize
// Headers: X-Platform-Id, X-Platform-Sk (validated by middleware.PlatformAuth)
// Body:    { token_key, metadata? }
func V2AuthorizeToken(c *gin.Context) {
	platform := middleware.PlatformFromContext(c)
	shadowUserId := middleware.ShadowUserIdFromContext(c)
	if platform == nil || shadowUserId <= 0 {
		// Should never happen: middleware guarantees both. Fail closed.
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "platform context missing"})
		return
	}

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

	// Idempotent path: token already registered? Verify it belongs to this platform.
	existingToken, err := model.GetTokenByKey(tokenKeyBody, true)
	if err == nil && existingToken != nil {
		if existingToken.UserId != shadowUserId {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "该密钥已被其他平台注册"})
			return
		}

		// Self-healing: if the token was disabled or had a custom quota set
		// externally, restore the V2 invariant (enabled + unlimited).
		if existingToken.Status != common.TokenStatusEnabled || !existingToken.UnlimitedQuota {
			model.DB.Model(existingToken).Updates(map[string]interface{}{
				"status":          common.TokenStatusEnabled,
				"unlimited_quota": true,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "密钥已存在",
			"data": gin.H{
				"token_id":    existingToken.Id,
				"status":      "exists",
				"platform_id": platform.PlatformId,
				"created_at":  time.Unix(existingToken.CreatedTime, 0).UTC().Format(time.RFC3339),
			},
		})
		return
	}

	// Build a descriptive token name. Optional metadata.platform_user_id
	// (caller's own user identifier) is appended to aid downstream reconciliation.
	tokenName := fmt.Sprintf("v2_%s", platform.PlatformId)
	if req.Metadata != nil {
		if uid, ok := req.Metadata["platform_user_id"]; ok {
			tokenName = fmt.Sprintf("v2_%s_%v", platform.PlatformId, uid)
		}
	}

	// Audit metadata; best-effort, stored as JSON in the manage log line.
	var metadataStr string
	if req.Metadata != nil {
		if bs, err := common.Marshal(req.Metadata); err == nil {
			metadataStr = string(bs)
		}
	}

	token := &model.Token{
		UserId:         shadowUserId,
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

	// Best-effort audit log; non-fatal on failure.
	if metadataStr != "" {
		_ = model.LOG_DB.Create(&model.Log{
			UserId:    shadowUserId,
			Username:  "platform_" + platform.PlatformId,
			CreatedAt: common.GetTimestamp(),
			Type:      model.LogTypeManage,
			Content:   fmt.Sprintf("V2平台Token授权，platform: %s, metadata: %s", platform.PlatformId, metadataStr),
			TokenId:   token.Id,
		}).Error
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "密钥授权成功",
		"data": gin.H{
			"token_id":    token.Id,
			"status":      "authorized",
			"platform_id": platform.PlatformId,
			"created_at":  time.Unix(token.CreatedTime, 0).UTC().Format(time.RFC3339),
		},
	})
}

// V2GetPlatformLogs returns consumption logs for the authenticated platform's
// shadow user. The plaintext sk is never returned; logs reference tokens by
// integer token_id (Token.Id), which is globally unique and safe to expose
// once auth is enforced (the user_id filter prevents cross-platform reads).
//
// GET /api/v2/external/logs
// Headers: X-Platform-Id, X-Platform-Sk
// Query:   page, page_size, start_date, end_date, token_id (optional filter)
func V2GetPlatformLogs(c *gin.Context) {
	platform := middleware.PlatformFromContext(c)
	shadowUserId := middleware.ShadowUserIdFromContext(c)
	if platform == nil || shadowUserId <= 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "platform context missing"})
		return
	}

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	tokenIdFilter := c.Query("token_id")
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

	tx := model.LOG_DB.Where("user_id = ? AND type = ?", shadowUserId, model.LogTypeConsume)

	// Optional token_id filter: validate the token belongs to this platform's
	// shadow user. If the token id is malformed or unrelated, return empty
	// results (consistent with the pre-rewrite semantics for token_key filter).
	var filterTokenId int
	if tokenIdFilter != "" {
		tid, err := strconv.Atoi(tokenIdFilter)
		if err != nil || tid <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "token_id 必须为正整数"})
			return
		}
		var filterToken model.Token
		if err := model.DB.Where("id = ? AND user_id = ?", tid, shadowUserId).First(&filterToken).Error; err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"platform_id": platform.PlatformId,
					"logs":        []interface{}{},
					"pagination":  gin.H{"page": page, "page_size": pageSize, "total": 0, "total_pages": 0},
					"summary":     gin.H{"total_requests": 0, "total_tokens": 0, "total_quota_consumed": 0},
				},
			})
			return
		}
		filterTokenId = filterToken.Id
		tx = tx.Where("token_id = ?", filterTokenId)
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

	// Aggregate on an independent query chain so the pagination SELECT below
	// is not contaminated by GORM statement state from the aggregate SELECT.
	var agg struct {
		TotalQuota            int
		TotalPromptTokens     int
		TotalCompletionTokens int
		UniqueTokens          int
	}
	aggDB := model.LOG_DB.Model(&model.Log{}).
		Where("user_id = ? AND type = ?", shadowUserId, model.LogTypeConsume)
	if filterTokenId > 0 {
		aggDB = aggDB.Where("token_id = ?", filterTokenId)
	}
	if startTimestamp > 0 {
		aggDB = aggDB.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp > 0 {
		aggDB = aggDB.Where("created_at <= ?", endTimestamp)
	}
	aggDB.Select(
		"COALESCE(SUM(quota), 0) AS total_quota, " +
			"COALESCE(SUM(prompt_tokens), 0) AS total_prompt_tokens, " +
			"COALESCE(SUM(completion_tokens), 0) AS total_completion_tokens, " +
			"COUNT(DISTINCT token_id) AS unique_tokens",
	).Scan(&agg)

	var logs []*model.Log
	offset := (page - 1) * pageSize
	if err := tx.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询失败"})
		return
	}

	// Build log items. token_id (integer) replaces the previous token_key (sk-..)
	// — sk plaintext is never reconstructed or returned here.
	logItems := make([]gin.H, 0, len(logs))
	for _, log := range logs {
		logItems = append(logItems, gin.H{
			"log_id":            log.Id,
			"time":              time.Unix(log.CreatedAt, 0).UTC().Format(time.RFC3339),
			"token_id":          log.TokenId,
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
			"platform_id": platform.PlatformId,
			"logs":        logItems,
			"pagination": gin.H{
				"page":        page,
				"page_size":   pageSize,
				"total":       total,
				"total_pages": totalPages,
			},
			"summary": gin.H{
				"total_requests":          int(total),
				"total_prompt_tokens":     agg.TotalPromptTokens,
				"total_completion_tokens": agg.TotalCompletionTokens,
				"total_tokens":            agg.TotalPromptTokens + agg.TotalCompletionTokens,
				"total_quota_consumed":    agg.TotalQuota,
				"unique_tokens":           agg.UniqueTokens,
			},
		},
	})
}
