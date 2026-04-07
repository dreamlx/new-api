package model

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// SendConsumeCallback sends an async consumption notification to the token's callback URL.
func SendConsumeCallback(userId int, tokenId int, log *Log) {
	token, err := GetTokenById(tokenId)
	if err != nil || !token.CallbackEnabled || token.CallbackUrl == "" {
		return
	}

	user, err := GetUserById(userId, false)
	if err != nil || user.ExternalUserId == "" {
		return
	}

	fullKey := token.Key
	if !strings.HasPrefix(fullKey, "sk-") {
		fullKey = "sk-" + fullKey
	}

	callbackData := map[string]interface{}{
		"event":     "token_consumed",
		"timestamp": time.Now().Unix(),

		"external_user_id": user.ExternalUserId,
		"token_id":         tokenId,
		"token_key":        fullKey,
		"token_name":       token.Name,

		"model":             log.ModelName,
		"prompt_tokens":     log.PromptTokens,
		"completion_tokens": log.CompletionTokens,
		"quota_consumed":    log.Quota,
		"amount_usd":        float64(log.Quota) / float64(common.QuotaPerUnit),

		"log_id":     log.Id,
		"request_id": fmt.Sprintf("log_%d", log.Id),
	}

	jsonData, err := common.Marshal(callbackData)
	if err != nil {
		common.SysLog(fmt.Sprintf("callback marshal failed: tokenId=%d, error=%s", tokenId, err.Error()))
		return
	}

	req, err := http.NewRequest("POST", token.CallbackUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		common.SysLog(fmt.Sprintf("callback create request failed: tokenId=%d, url=%s, error=%s",
			tokenId, token.CallbackUrl, err.Error()))
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "New-API-Callback/1.0")

	if token.CallbackSecret != "" {
		signature := generateHMACSignature(jsonData, token.CallbackSecret)
		req.Header.Set("X-Callback-Signature", signature)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		common.SysLog(fmt.Sprintf("callback request failed: tokenId=%d, url=%s, error=%s",
			tokenId, token.CallbackUrl, err.Error()))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		common.SysLog(fmt.Sprintf("callback success: tokenId=%d, status=%d", tokenId, resp.StatusCode))
	} else {
		common.SysLog(fmt.Sprintf("callback failed: tokenId=%d, status=%d", tokenId, resp.StatusCode))
	}
}

// GenerateHMACSignatureForTest is an exported alias for testing.
func GenerateHMACSignatureForTest(data []byte, secret string) string {
	return generateHMACSignature(data, secret)
}

func generateHMACSignature(data []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
