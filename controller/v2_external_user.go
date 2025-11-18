package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"one-api/model"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// V2TokenAuthorizeRequest v2密钥授权请求结构
type V2TokenAuthorizeRequest struct {
	PlatformId   string                 `json:"platform_id" binding:"required,min=1,max=50"`
	TokenKey     string                 `json:"token_key" binding:"required,min=1,max=200"`
	InitialQuota int                    `json:"initial_quota" binding:"required,min=0"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// V2TokenAuthorizeResponse v2密钥授权响应结构
type V2TokenAuthorizeResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		TokenKey     string  `json:"token_key"`
		CurrentQuota int     `json:"current_quota"`
		PreviousQuota int    `json:"previous_quota,omitempty"`
		QuotaAdded   int     `json:"quota_added,omitempty"`
		QuotaUsd     float64 `json:"quota_usd"`
		Status       string  `json:"status"`
		CreatedAt    string  `json:"created_at,omitempty"`
		UpdatedAt    string  `json:"updated_at,omitempty"`
		ProxyUserId  int     `json:"proxy_user_id"`
	} `json:"data"`
}

// V2PlatformLogResponse v2平台日志响应结构
type V2PlatformLogResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		PlatformId string `json:"platform_id"`
		DateRange  struct {
			StartDate string `json:"start_date"`
			EndDate   string `json:"end_date"`
		} `json:"date_range"`
		Logs []struct {
			LogId          string `json:"log_id"`
			Time           string `json:"time"`
			TokenKey       string `json:"token_key"`
			ModelName      string `json:"model_name"`
			PromptTokens   int    `json:"prompt_tokens"`
			CompletionTokens int    `json:"completion_tokens"`
			TotalTokens    int    `json:"total_tokens"`
			QuotaCost      int    `json:"quota_cost"`
		} `json:"logs"`
		Pagination struct {
			Page       int  `json:"page"`
			PageSize   int  `json:"page_size"`
			TotalItems int  `json:"total_items"`
			TotalPages int  `json:"total_pages"`
			HasNext    bool `json:"has_next"`
			HasPrev    bool `json:"has_prev"`
		} `json:"pagination"`
		Summary struct {
			TotalRequests         int `json:"total_requests"`
			TotalPromptTokens     int `json:"total_prompt_tokens"`
			TotalCompletionTokens int `json:"total_completion_tokens"`
			TotalTokens           int `json:"total_tokens"`
			TotalQuotaConsumed    int `json:"total_quota_consumed"`
			UniqueTokens          int `json:"unique_tokens"`
		} `json:"summary"`
	} `json:"data"`
}

// V2TokenAuthorize 授权密钥接口
func V2TokenAuthorize(c *gin.Context) {
	var req V2TokenAuthorizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":   false,
			"message":   "参数错误：" + err.Error(),
			"error_code": "INVALID_PARAMETER",
		})
		return
	}

	// 验证platform_id格式
	if !isValidPlatformId(req.PlatformId) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":   false,
			"message":   "平台ID格式无效：只能包含字母、数字、下划线和中划线",
			"error_code": "INVALID_PARAMETER",
		})
		return
	}

	// 验证token_key格式
	if !isValidTokenKey(req.TokenKey) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":     false,
			"message":     "密钥格式无效：必须是sk-{连续字符串}格式，sk-后面不能包含短横线",
			"error_code":  "INVALID_PARAMETER",
			"valid_example": "sk-a99416b67cb54e178e9ffe8a55c255ae",
			"invalid_example": "sk-2-a99416b67cb54e178e9ffe8a55c255ae",
		})
		return
	}

	// 获取或创建平台用户
	platformUsername := fmt.Sprintf("platform_%s", req.PlatformId)
	user, err := model.GetOrCreatePlatformUser(platformUsername)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":   false,
			"message":   "平台用户创建失败：" + err.Error(),
			"error_code": "INTERNAL_ERROR",
		})
		return
	}

	// 需要导入 "one-api/common"
	// 检查token是否已存在（去掉 sk- 前缀后查询）
	tokenKeyToCheck := strings.TrimPrefix(req.TokenKey, "sk-")
	existingToken, err := model.GetTokenByKey(tokenKeyToCheck, true)
	if err == nil && existingToken != nil {
		// Token已存在，检查是否属于同一平台
		tokenUser, err := model.GetUserById(existingToken.UserId, true)
		if err == nil && tokenUser != nil && tokenUser.Username == platformUsername {
			// 同一平台，更新为无限额度模式
			oldQuota := existingToken.RemainQuota

			// V2平台Token统一设置为无限额度
			err = model.DB.Model(existingToken).Updates(map[string]interface{}{
				"unlimited_quota": true,
				"remain_quota":    0,
			}).Error
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success":   false,
					"message":   "Token更新失败：" + err.Error(),
					"error_code": "INTERNAL_ERROR",
				})
				return
			}

			// 记录额度变更日志
			topupLog := &model.Log{
				UserId:    user.Id,
				TokenName: req.TokenKey,
				Type:      model.LogTypeTopup,
				Content:   "v2平台密钥更新为无限额度模式",
				Quota:     0,
				Other:     fmt.Sprintf("platform_id:%s", req.PlatformId),
			}
			if metadata, err := json.Marshal(req.Metadata); err == nil {
				topupLog.Other += fmt.Sprintf(";metadata:%s", string(metadata))
			}
			model.LOG_DB.Create(topupLog)

			response := V2TokenAuthorizeResponse{
				Success: true,
				Message: "密钥已存在，已更新为无限额度模式",
			}
			response.Data.TokenKey = req.TokenKey
			response.Data.PreviousQuota = oldQuota
			response.Data.CurrentQuota = 0 // 无限额度模式下设为0
			response.Data.QuotaAdded = 0
			response.Data.Status = "updated_unlimited"
			response.Data.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			response.Data.ProxyUserId = user.Id

			c.JSON(http.StatusOK, response)
		} else {
			// Token属于不同平台
			c.JSON(http.StatusConflict, gin.H{
				"success":   false,
				"message":   "密钥已存在但属于不同平台",
				"error_code": "TOKEN_EXISTS",
			})
		}
		return
	}

	// 创建新token
	// 重要：去掉 sk- 前缀后存储，因为 middleware/auth.go 验证时会去掉 sk- 前缀
	// V2平台Token使用无限额度（平台自己管理计费）
	tokenKey := strings.TrimPrefix(req.TokenKey, "sk-")
	token := &model.Token{
		UserId:         user.Id,
		Key:            tokenKey,
		Name:           fmt.Sprintf("v2-platform-%s-token", req.PlatformId),
		UnlimitedQuota: true, // V2平台Token使用无限额度
		RemainQuota:    0,    // 无限额度时设为0
		UsedQuota:      0,
		CreatedTime:    time.Now().Unix(),
		Status:         1, // common.TokenStatusEnabled
	}

	err = model.DB.Create(token).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":   false,
			"message":   "Token创建失败：" + err.Error(),
			"error_code": "INTERNAL_ERROR",
		})
		return
	}

	// 记录授权日志
	authLog := &model.Log{
		UserId:    user.Id,
		TokenName: req.TokenKey,
		Type:      model.LogTypeTopup,
		Content:   fmt.Sprintf("v2平台密钥授权创建 %d quota", req.InitialQuota),
		Quota:     req.InitialQuota,
		Other:     fmt.Sprintf("platform_id:%s", req.PlatformId),
	}
	if metadata, err := json.Marshal(req.Metadata); err == nil {
		authLog.Other += fmt.Sprintf(";metadata:%s", string(metadata))
	}
	model.LOG_DB.Create(authLog)

	response := V2TokenAuthorizeResponse{
		Success: true,
		Message: "密钥授权成功",
	}
	response.Data.TokenKey = req.TokenKey
	response.Data.CurrentQuota = req.InitialQuota
	response.Data.QuotaUsd = float64(req.InitialQuota) / 500000.0 // $1 USD = 500,000 quota
	response.Data.Status = "authorized"
	response.Data.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	response.Data.ProxyUserId = user.Id

	c.JSON(http.StatusOK, response)
}

// V2GetPlatformLogs 获取平台消费流水接口
func V2GetPlatformLogs(c *gin.Context) {
	platformId := c.Param("platform_id")
	if platformId == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":   false,
			"message":   "platform_id参数不能为空",
			"error_code": "MISSING_PARAMETER",
		})
		return
	}

	// 验证platform_id格式
	if !isValidPlatformId(platformId) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":   false,
			"message":   "platform_id格式无效",
			"error_code": "INVALID_PARAMETER",
		})
		return
	}

	// 获取查询参数
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	pageStr := c.Query("page")
	pageSizeStr := c.Query("page_size")

	if startDate == "" || endDate == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":   false,
			"message":   "start_date和end_date参数不能为空",
			"error_code": "MISSING_PARAMETER",
		})
		return
	}

	// 解析日期
	startTime, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":   false,
			"message":   "start_date格式错误，请使用YYYY-MM-DD格式",
			"error_code": "INVALID_PARAMETER",
		})
		return
	}

	endTime, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":   false,
			"message":   "end_date格式错误，请使用YYYY-MM-DD格式",
			"error_code": "INVALID_PARAMETER",
		})
		return
	}

	// 设置结束时间为当天的23:59:59
	endTime = endTime.Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	// 分页参数
	page := 1
	pageSize := 20
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if pageSizeStr != "" {
		if s, err := strconv.Atoi(pageSizeStr); err == nil && s > 0 && s <= 100 {
			pageSize = s
		}
	}

	// 验证平台是否存在
	platformUsername := fmt.Sprintf("platform_%s", platformId)
	user, err := model.GetUserByUsername(platformUsername)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success":   false,
			"message":   "指定的平台不存在",
			"error_code": "PLATFORM_NOT_FOUND",
		})
		return
	}

	// 查询消费日志
	logs, total, err := model.GetPlatformConsumptionLogs(user.Id, startTime.Unix(), endTime.Unix(), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":   false,
			"message":   "查询平台消费日志失败：" + err.Error(),
			"error_code": "INTERNAL_ERROR",
		})
		return
	}

	// 构建响应
	response := V2PlatformLogResponse{
		Success: true,
		Message: "查询成功",
	}
	response.Data.PlatformId = platformId
	response.Data.DateRange.StartDate = startDate
	response.Data.DateRange.EndDate = endDate

	// 初始化logs数组为空数组（而非nil）
	response.Data.Logs = make([]struct {
		LogId            string `json:"log_id"`
		Time             string `json:"time"`
		TokenKey         string `json:"token_key"`
		ModelName        string `json:"model_name"`
		PromptTokens     int    `json:"prompt_tokens"`
		CompletionTokens int    `json:"completion_tokens"`
		TotalTokens      int    `json:"total_tokens"`
		QuotaCost        int    `json:"quota_cost"`
	}, 0)

	// 转换日志格式
	for _, log := range logs {
		// 获取实际的Token密钥（优先）
		var tokenKey string
		if token, err := model.GetTokenById(log.TokenId); err == nil && token != nil {
			// 显示完整的Token密钥（不脱敏，便于对方平台匹配识别）
			tokenKey = token.Key
			// 如果token.Key不包含sk-前缀，添加上
			if !strings.HasPrefix(tokenKey, "sk-") {
				tokenKey = "sk-" + tokenKey
			}
		} else if log.TokenName != "" {
			// 备选：使用TokenName（但这不是理想情况）
			tokenKey = log.TokenName
		} else {
			tokenKey = "unknown"
		}

		totalTokens := log.PromptTokens + log.CompletionTokens
		logEntry := struct {
			LogId           string `json:"log_id"`
			Time            string `json:"time"`
			TokenKey        string `json:"token_key"`
			ModelName       string `json:"model_name"`
			PromptTokens    int    `json:"prompt_tokens"`
			CompletionTokens int    `json:"completion_tokens"`
			TotalTokens     int    `json:"total_tokens"`
			QuotaCost       int    `json:"quota_cost"`
		}{
			LogId:           strconv.Itoa(log.Id),
			Time:            time.Unix(log.CreatedAt, 0).UTC().Format(time.RFC3339),
			TokenKey:        tokenKey,
			ModelName:       log.ModelName,
			PromptTokens:    log.PromptTokens,
			CompletionTokens: log.CompletionTokens,
			TotalTokens:     totalTokens,
			QuotaCost:       log.Quota,
		}
		response.Data.Logs = append(response.Data.Logs, logEntry)
	}

	// 分页信息
	totalInt := int(total)
	totalPages := (totalInt + pageSize - 1) / pageSize
	response.Data.Pagination.Page = page
	response.Data.Pagination.PageSize = pageSize
	response.Data.Pagination.TotalItems = totalInt
	response.Data.Pagination.TotalPages = totalPages
	response.Data.Pagination.HasNext = page < totalPages
	response.Data.Pagination.HasPrev = page > 1

	c.JSON(http.StatusOK, response)
}

// 辅助函数：验证platform_id格式
func isValidPlatformId(platformId string) bool {
	if len(platformId) == 0 || len(platformId) > 50 {
		return false
	}
	for _, r := range platformId {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// 辅助函数：验证token_key格式
// 要求：sk-{连续字符串}，不能有多个短横线
func isValidTokenKey(tokenKey string) bool {
	if !strings.HasPrefix(tokenKey, "sk-") {
		return false
	}
	// 去掉sk-前缀后，检查剩余部分
	remaining := strings.TrimPrefix(tokenKey, "sk-")
	if len(remaining) < 8 {
		return false // 至少8个字符
	}
	// 禁止包含短横线（因为middleware会按-分割并只取第一部分）
	if strings.Contains(remaining, "-") {
		return false
	}
	// 建议：只包含字母数字，长度32-48位
	// 但这里不做强制要求，只禁止短横线即可
	return true
}