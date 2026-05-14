package controller

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// Refund flow constants.
const (
	// refundConfirmTokenTTL is how long a confirm_token issued by /prepare
	// remains valid. Short enough to neutralise leaked tokens, long enough
	// for an admin to read the confirmation dialog and click execute.
	refundConfirmTokenTTL = 5 * time.Minute

	// refundConfirmTokenVersion lets us evolve the token format later
	// without ambiguous parsing.
	refundConfirmTokenVersion = "v1"
)

// RefundPrepareRequest is the JSON body for POST /api/topup/refund/prepare.
type RefundPrepareRequest struct {
	TradeNo string `json:"trade_no"`
}

// signConfirmToken returns an HMAC-SHA256-signed token binding the refund
// confirmation to a specific {trade_no, admin_id, expires_at} triple.
//
// We sign rather than persist because a stateless signature gives us:
//   - cheap horizontal scale: any replica can mint and verify;
//   - automatic expiry without a cleanup job (the verifier rejects stale
//     tokens by comparing the embedded expires_at against now);
//   - tamper-evidence: a single byte flip in trade_no/admin_id/expires_at
//     invalidates the MAC.
//
// The key is the per-deployment common.CryptoSecret, which is seeded from
// CRYPTO_SECRET or falls back to SESSION_SECRET on startup. Using this
// already-shared secret means a co-deployed replica fleet shares the same
// verification key without extra config.
func signConfirmToken(tradeNo string, adminId int, expiresAt int64) string {
	payload := refundConfirmTokenPayload(tradeNo, adminId, expiresAt)
	mac := hmac.New(sha256.New, []byte(common.CryptoSecret))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return refundConfirmTokenVersion + "." + strconv.FormatInt(expiresAt, 10) + "." + sig
}

// refundConfirmTokenPayload is the canonical input string fed to HMAC.
// Field order MUST be stable across mint and verify.
func refundConfirmTokenPayload(tradeNo string, adminId int, expiresAt int64) string {
	return refundConfirmTokenVersion + "|" + tradeNo + "|" + strconv.Itoa(adminId) + "|" + strconv.FormatInt(expiresAt, 10)
}

// verifyConfirmToken validates a token previously issued by signConfirmToken.
// Returns nil on success, a descriptive error otherwise. Constant-time
// comparison is used for the signature to avoid trivial timing oracles, even
// though the surrounding handler is root-only.
func verifyConfirmToken(token string, tradeNo string, adminId int, now int64) error {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return fmt.Errorf("malformed token")
	}
	if parts[0] != refundConfirmTokenVersion {
		return fmt.Errorf("unsupported token version")
	}
	expiresAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return fmt.Errorf("malformed expires_at")
	}
	if now > expiresAt {
		return fmt.Errorf("token expired")
	}
	expected := signConfirmToken(tradeNo, adminId, expiresAt)
	// The token string we just rebuilt has the same prefix and expires_at as
	// the input, so direct constant-time comparison of the full strings is
	// equivalent to comparing just the signatures.
	if subtle.ConstantTimeCompare([]byte(expected), []byte(token)) != 1 {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

// requireRootRole returns the admin's user id when the calling context has
// RoleRootUser, or false otherwise. Centralising the check ensures every
// money-out endpoint applies the same rule.
func requireRootRole(c *gin.Context) (int, bool) {
	role := c.GetInt("role")
	if role != common.RoleRootUser {
		return 0, false
	}
	id := c.GetInt("id")
	if id == 0 {
		return 0, false
	}
	return id, true
}

// RefundPrepare issues a short-lived confirm_token bound to a specific TopUp
// + the calling admin. The follow-up /api/topup/refund verifies this token
// instead of re-trusting the admin's repeat click — a stolen browser tab
// cannot mint a token because token signing requires the server-side
// CryptoSecret.
//
// POST /api/topup/refund/prepare
//
//	{"trade_no": "USRxx..."}
func RefundPrepare(c *gin.Context) {
	adminId, ok := requireRootRole(c)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"message": "error", "data": "无权限"})
		return
	}

	var req RefundPrepareRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TradeNo == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	topUp := model.GetTopUpByTradeNo(req.TradeNo)
	if topUp == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "data": "订单不存在"})
		return
	}

	if topUp.Status != common.TopUpStatusSuccess {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "订单未支付，无法退款"})
		return
	}

	if topUp.RefundStatus == common.RefundStatusPending || topUp.RefundStatus == common.RefundStatusSuccess {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "订单已发起退款"})
		return
	}

	username, _ := model.GetUsernameById(topUp.UserId, true)

	expiresAt := time.Now().Add(refundConfirmTokenTTL).Unix()
	token := signConfirmToken(topUp.TradeNo, adminId, expiresAt)

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"trade_no":      topUp.TradeNo,
			"amount":        topUp.Amount,
			"money":         topUp.Money,
			"user_id":       topUp.UserId,
			"username":      username,
			"confirm_token": token,
			"expires_at":    expiresAt,
		},
	})
}
