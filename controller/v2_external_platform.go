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

// PlatformQuotaForNewUser is the default quota for auto-created platform users (~$1M).
// This ensures Trust Quota Bypass works with upstream BillingSession.
const PlatformQuotaForNewUser = 499999500000

// ==================== Request Structs ====================

type V2TokenAuthorizeRequest struct {
	// PlatformId is OPTIONAL and no longer authoritative: the platform identity
	// is derived from PlatformAuth (X-Platform-Id/X-Platform-Sk). When present it
	// is validated to match the authenticated platform, so an older client that
	// still sends it keeps working, but a mismatched value is rejected.
	PlatformId string                 `json:"platform_id" binding:"omitempty,max=100"`
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

	// platform_id is authoritative from the authenticated platform (PlatformAuth),
	// never trusted from the body. A body platform_id, if present, must match —
	// this blocks an authenticated platform from minting tokens under another
	// platform's identity.
	platform := middleware.PlatformFromContext(c)
	if platform == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "unauthorized"})
		return
	}
	platformId := platform.PlatformId
	if req.PlatformId != "" && req.PlatformId != platformId {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "platform_id 与鉴权平台不一致"})
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
	platformUsername := "platform_" + platformId
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
				"platform_id": platformId,
				"created_at":  time.Unix(existingToken.CreatedTime, 0).UTC().Format(time.RFC3339),
			},
		})
		return
	}

	// Build metadata name
	tokenName := fmt.Sprintf("v2_%s", platformId)
	if req.Metadata != nil {
		if uid, ok := req.Metadata["platform_user_id"]; ok {
			tokenName = fmt.Sprintf("v2_%s_%v", platformId, uid)
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
			Content:   fmt.Sprintf("V2平台Token授权，platform: %s, metadata: %s", platformId, metadataStr),
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
			"platform_id": platformId,
			"created_at":  time.Unix(token.CreatedTime, 0).UTC().Format(time.RFC3339),
		},
	})
}

// V2RevokeToken revokes a token owned by the authenticated platform. New API
// uses DISABLE (not delete) as the revocation primitive: the row and its
// log/billing trail are preserved, the key stops working at relay immediately
// (cache is invalidated), and the action is reversible (re-enable). This keeps
// the V2 logs endpoint's token_key enrichment intact for historical entries.
//
// Ownership is enforced via the platform's shadow user — a platform can only
// revoke tokens it owns. Idempotent: revoking an already-disabled token is a
// success and reports status "already_disabled".
//
// DELETE /api/v2/external/tokens/:id
func V2RevokeToken(c *gin.Context) {
	platform := middleware.PlatformFromContext(c)
	if platform == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "unauthorized"})
		return
	}
	shadowUserId := middleware.ShadowUserIdFromContext(c)
	if shadowUserId <= 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "platform shadow user 未初始化"})
		return
	}

	tokenId, err := strconv.Atoi(c.Param("id"))
	if err != nil || tokenId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的 token id"})
		return
	}

	// Scope by shadow user id: a platform may only revoke its own tokens.
	token, err := model.GetTokenByIds(tokenId, shadowUserId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Token 不存在或不属于该平台"})
		return
	}

	alreadyDisabled := token.Status == common.TokenStatusDisabled
	if !alreadyDisabled {
		if err := model.DisableTokenByKey(token.Key); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "禁用 Token 失败: " + err.Error()})
			return
		}
	}

	status := "disabled"
	if alreadyDisabled {
		status = "already_disabled"
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Token 已禁用",
		"data": gin.H{
			"token_id":    tokenId,
			"platform_id": platform.PlatformId,
			"status":      status,
		},
	})
}

// V2GetPlatformLogs returns consumption logs for the authenticated platform.
// The platform identity and its shadow user come from PlatformAuth context —
// there is no platform_id path/query param. Tokens minted via V2AuthorizeToken
// are owned by that same shadow user, so logs are scoped by shadow user id.
// GET /api/v2/external/logs
func V2GetPlatformLogs(c *gin.Context) {
	platform := middleware.PlatformFromContext(c)
	if platform == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "unauthorized"})
		return
	}
	platformId := platform.PlatformId
	shadowUserId := middleware.ShadowUserIdFromContext(c)
	if shadowUserId <= 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "platform shadow user 未初始化"})
		return
	}

	// Parse query params
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	tokenKeyFilter := c.Query("token_key")
	tokenIdStr := c.Query("token_id")
	afterIdStr := c.Query("after_id")
	modelName := c.Query("model_name")
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
	tx := model.LOG_DB.Where("user_id = ? AND type = ?", shadowUserId, model.LogTypeConsume)

	// Optional token_id filter (takes precedence over token_key)
	if tokenIdStr != "" {
		if tokenId, err := strconv.Atoi(tokenIdStr); err == nil && tokenId > 0 {
			tx = tx.Where("token_id = ?", tokenId)
		}
	} else if tokenKeyFilter != "" {
		tokenKeyBody := strings.TrimPrefix(tokenKeyFilter, "sk-")
		filterToken, err := model.GetTokenByKey(tokenKeyBody, true)
		if err != nil || filterToken == nil || filterToken.UserId != shadowUserId {
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

	// Optional after_id filter (incremental pull)
	if afterIdStr != "" {
		if afterId, err := strconv.Atoi(afterIdStr); err == nil && afterId > 0 {
			tx = tx.Where("id > ?", afterId)
		}
	}
	if modelName != "" {
		tx = tx.Where("model_name LIKE ?", "%"+modelName+"%")
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
	orderClause := "created_at DESC"
	if afterIdStr != "" {
		orderClause = "id ASC"
	}
	if err := tx.Order(orderClause).Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询失败"})
		return
	}

	if err := model.PopulateChannelNames(logs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询失败"})
		return
	}

	// Build response
	var totalPromptTokens, totalCompletionTokens, totalQuotaConsumed, totalCacheTokens, totalCacheCreationTokens int
	uniqueTokens := make(map[int]bool)
	logItems := make([]gin.H, 0, len(logs))

	for _, log := range logs {
		totalPromptTokens += log.PromptTokens
		totalCompletionTokens += log.CompletionTokens
		totalQuotaConsumed += log.Quota

		// Prompt-cache hit count is stored in the per-log Other JSON
		// (service/log_info_generate.go writes other["cache_tokens"]). Surface it
		// so LH can expose cache-hit stats in its customer-facing usage reporting.
		cacheTokens := extractCacheTokens(log.Other)
		totalCacheTokens += cacheTokens

		// #22 — cache_creation_tokens: tokens written to the prompt cache (Anthropic
		// bills these at 1.25× prompt price; LH must surface them to bill the
		// premium). Same Other-JSON pattern as cache_tokens; absent on non-Anthropic
		// models and older logs → 0.
		cacheCreationTokens := extractCacheCreationTokens(log.Other)
		totalCacheCreationTokens += cacheCreationTokens

		// Resolve full token_key
		var tokenKey string
		if log.TokenId > 0 {
			uniqueTokens[log.TokenId] = true
			if t, err := model.GetTokenById(log.TokenId); err == nil {
				tokenKey = "sk-" + t.Key
			}
		}

		logItems = append(logItems, gin.H{
			"log_id":                 log.Id,
			"request_id":             log.RequestId,
			"created_at":             log.CreatedAt,
			"time":                   time.Unix(log.CreatedAt, 0).UTC().Format(time.RFC3339),
			"token_id":               log.TokenId,
			"token_key":              tokenKey,
			"model_name":             log.ModelName,
			"channel_id":             log.ChannelId,
			"channel_name":           log.ChannelName,
			"prompt_tokens":          log.PromptTokens,
			"completion_tokens":      log.CompletionTokens,
			"cache_tokens":           cacheTokens,
			"cache_creation_tokens":  cacheCreationTokens,
			"total_tokens":           log.PromptTokens + log.CompletionTokens,
			"quota_cost":             log.Quota,
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
				"total_requests":             len(logItems),
				"total_prompt_tokens":        totalPromptTokens,
				"total_completion_tokens":    totalCompletionTokens,
				"total_cache_tokens":         totalCacheTokens,
				"total_cache_creation_tokens": totalCacheCreationTokens,
				"total_tokens":               totalPromptTokens + totalCompletionTokens,
				"total_quota_consumed":       totalQuotaConsumed,
				"unique_tokens":              len(uniqueTokens),
			},
		},
	})
}

// ==================== Helpers ====================

// extractCacheTokens reads the prompt-cache hit count from a log's Other JSON
// blob (key "cache_tokens", written by service/log_info_generate.go). Returns 0
// when absent or unparseable — cache reporting is best-effort observability and
// must never break the logs endpoint. JSON numbers decode to float64.
func extractCacheTokens(other string) int {
	if other == "" {
		return 0
	}
	m, err := common.StrToMap(other)
	if err != nil || m == nil {
		return 0
	}
	switch v := m["cache_tokens"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

// extractCacheCreationTokens reads the cache-write token count from a log's
// Other JSON blob (key "cache_creation_tokens", written by
// service/log_info_generate.go). Anthropic bills these at 1.25× prompt price;
// LH surfaces the count to apply the premium (#22 / lh-enterprise #588). Returns
// 0 when absent or unparseable — best-effort observability, must never break
// the logs endpoint. Mirrors extractCacheTokens; JSON numbers decode to float64.
func extractCacheCreationTokens(other string) int {
	if other == "" {
		return 0
	}
	m, err := common.StrToMap(other)
	if err != nil || m == nil {
		return 0
	}
	switch v := m["cache_creation_tokens"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func getOrCreatePlatformUser(username, platformId string) (*model.User, error) {
	var user model.User
	if err := model.DB.Where("username = ?", username).First(&user).Error; err == nil {
		return &user, nil
	}

	email := fmt.Sprintf("%s@platform.local", platformId)
	user = model.User{
		Username:    username,
		DisplayName: "Platform: " + platformId,
		Email:       email,
		Password:    common.GetRandomString(32),
		AffCode:     common.GetRandomString(16),
		IsExternal:  true,
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Quota:       PlatformQuotaForNewUser,
	}
	user.SetExternalUserId("platform_" + platformId)

	if err := model.DB.Create(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
