package model

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"github.com/QuantumNous/new-api/common"
	"strings"
	"time"
)

// SendConsumeCallback 异步发送消费回调通知
// 当Token消费发生后，如果Token配置了回调URL，则发送通知到CEC等下游平台
func SendConsumeCallback(userId int, tokenId int, log *Log) {
	// 1. 查询Token配置
	token, err := GetTokenById(tokenId)
	if err != nil || !token.CallbackEnabled || token.CallbackUrl == "" {
		return // Token不存在或未启用回调，静默返回
	}

	// 2. 获取用户external_user_id
	user, err := GetUserById(userId, false)
	if err != nil || user.ExternalUserId == "" {
		return // 非外部用户，跳过回调
	}

	// 3. 获取完整Token密钥
	fullKey := token.Key
	if !strings.HasPrefix(fullKey, "sk-") {
		fullKey = "sk-" + fullKey
	}

	// 4. 构造回调数据（只包含单次LLM请求消费信息）
	callbackData := map[string]interface{}{
		"event":     "token_consumed",
		"timestamp": time.Now().Unix(),

		// 用户和Token信息
		"external_user_id": user.ExternalUserId,
		"token_id":         tokenId,
		"token_key":        fullKey,      // 完整Token密钥，便于CEC分组统计
		"token_name":       token.Name,

		// LLM请求消费详情
		"model":             log.ModelName,
		"prompt_tokens":     log.PromptTokens,
		"completion_tokens": log.CompletionTokens,
		"quota_consumed":    log.Quota,
		"amount_usd":        float64(log.Quota) / float64(common.QuotaPerUnit),

		// 追踪信息
		"log_id":     log.Id,
		"request_id": fmt.Sprintf("log_%d", log.Id),
	}

	// 5. 序列化为JSON
	jsonData, err := json.Marshal(callbackData)
	if err != nil {
		common.SysLog(fmt.Sprintf("callback marshal failed: tokenId=%d, error=%s", tokenId, err.Error()))
		return
	}

	// 6. 创建HTTP POST请求
	req, err := http.NewRequest("POST", token.CallbackUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		common.SysLog(fmt.Sprintf("callback create request failed: tokenId=%d, url=%s, error=%s",
			tokenId, token.CallbackUrl, err.Error()))
		return
	}

	// 7. 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "New-API-Callback/1.0")

	// 8. 添加HMAC签名（如果配置了secret）
	if token.CallbackSecret != "" {
		signature := generateHMACSignature(jsonData, token.CallbackSecret)
		req.Header.Set("X-Callback-Signature", signature)
	}

	// 9. 发送请求（超时3秒）
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// 记录失败日志，但不影响主流程
		common.SysLog(fmt.Sprintf("callback request failed: tokenId=%d, url=%s, error=%s",
			tokenId, token.CallbackUrl, err.Error()))
		return
	}
	defer resp.Body.Close()

	// 10. 记录结果
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		common.SysLog(fmt.Sprintf("callback success: tokenId=%d, status=%d", tokenId, resp.StatusCode))
	} else {
		common.SysLog(fmt.Sprintf("callback failed: tokenId=%d, status=%d", tokenId, resp.StatusCode))
	}
}

// generateHMACSignature 生成HMAC-SHA256签名
func generateHMACSignature(data []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
