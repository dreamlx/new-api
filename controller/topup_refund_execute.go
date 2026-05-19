package controller

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// RefundExecuteRequest is the JSON body for POST /api/topup/refund.
type RefundExecuteRequest struct {
	TradeNo      string `json:"trade_no"`
	ConfirmToken string `json:"confirm_token"`
	Reason       string `json:"reason"`
}

// computeRefundQuota returns the quota amount we need to revoke for this
// TopUp. If QuotaGranted was recorded (post-v2 orders) we trust it; legacy
// orders are derived from Amount * QuotaPerUnit using decimal math to match
// the original grant path exactly.
func computeRefundQuota(topUp *model.TopUp) int64 {
	if topUp.QuotaGranted > 0 {
		return topUp.QuotaGranted
	}
	dAmount := decimal.NewFromInt(topUp.Amount)
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	return dAmount.Mul(dQuotaPerUnit).IntPart()
}

// RefundExecute performs the actual refund after verifying the confirm_token
// minted by /prepare.
//
// POST /api/topup/refund
//
//	{"trade_no":"...","confirm_token":"...","reason":"..."}
//
// Order of operations (paranoid):
//  1. Auth (root).
//  2. Verify confirm_token signature, expiry, trade_no binding, admin binding.
//  3. LockOrder + re-read TopUp (state may have changed since /prepare).
//  4. MarkRefundPending via DB conditional update; rows==0 means another
//     replica/admin already started a refund, return idempotent success.
//  5. Dispatch to the provider SDK. For Alipay the SDK call is synchronous —
//     on success we CompleteRefund + DecreaseUserQuota + RecordLog inside
//     the same handler. For WeChat the SDK call only submits; the actual
//     completion (and quota revocation) happens in the refund_notify handler
//     once WeChat confirms via webhook. We deliberately do NOT deduct quota
//     here in the WeChat path: if the user spent the quota between submit
//     and notify, deducting now would mean a refund-in-progress could put
//     the account into a negative balance and then bounce back if WeChat
//     declines the refund. The notify handler is the authoritative success
//     boundary.
func RefundExecute(c *gin.Context) {
	adminId, ok := requireRootRole(c)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"message": "error", "data": "无权限"})
		return
	}

	var req RefundExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TradeNo == "" || req.ConfirmToken == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if err := verifyConfirmToken(req.ConfirmToken, req.TradeNo, adminId, time.Now().Unix()); err != nil {
		log.Printf("refund execute: token verify failed tradeNo=%s adminId=%d err=%v", req.TradeNo, adminId, err)
		c.JSON(http.StatusForbidden, gin.H{"message": "error", "data": "确认令牌无效或已过期"})
		return
	}

	LockOrder(req.TradeNo)
	defer UnlockOrder(req.TradeNo)

	topUp := model.GetTopUpByTradeNo(req.TradeNo)
	if topUp == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "data": "订单不存在"})
		return
	}

	if topUp.Status != common.TopUpStatusSuccess {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "订单未支付，无法退款"})
		return
	}

	if topUp.RefundStatus == common.RefundStatusSuccess {
		c.JSON(http.StatusOK, gin.H{
			"message": "success",
			"data": gin.H{
				"status":   common.RefundStatusSuccess,
				"trade_no": topUp.TradeNo,
			},
		})
		return
	}

	rowsAffected, err := model.MarkRefundPending(model.DB, topUp.TradeNo, adminId, req.Reason)
	if err != nil {
		log.Printf("refund execute: MarkRefundPending failed tradeNo=%s err=%v", topUp.TradeNo, err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "data": "数据库错误"})
		return
	}
	if rowsAffected == 0 {
		// Another replica/admin already moved this order into refund_pending
		// or refund_success. Treat as idempotent success: callers do not need
		// to know which replica won the race.
		c.JSON(http.StatusOK, gin.H{
			"message": "success",
			"data": gin.H{
				"status":   common.RefundStatusPending,
				"trade_no": topUp.TradeNo,
			},
		})
		return
	}

	// Persist the deterministic outRefundNo as refund_trade_no immediately so
	// any refund_pending row has a provider correlator from the moment it
	// enters that state. The conditional WHERE (refund_status=pending) is the
	// concurrency safety net: if another writer mutated the row out from
	// under us between MarkRefundPending and now, treat it as a race-lost
	// refund attempt and bail out. For alipay we may overwrite this value
	// later with the SDK-returned TradeNo (which is the canonical id at the
	// provider). For wxpay the outRefundNo itself is the correlator.
	outRefundNo := "RFD" + topUp.TradeNo
	res := model.DB.Model(&model.TopUp{}).
		Where("trade_no = ? AND refund_status = ?", topUp.TradeNo, common.RefundStatusPending).
		Update("refund_trade_no", outRefundNo)
	if res.Error != nil {
		log.Printf("refund execute: persist refund_trade_no failed tradeNo=%s err=%v", topUp.TradeNo, res.Error)
		_ = model.MarkRefundFailed(model.DB, topUp.TradeNo, fmt.Sprintf("persist refund_trade_no error: %v", res.Error))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "data": "数据库错误"})
		return
	}
	if res.RowsAffected == 0 {
		log.Printf("refund execute: refund_trade_no UPDATE matched 0 rows tradeNo=%s — concurrent mutation", topUp.TradeNo)
		_ = model.MarkRefundFailed(model.DB, topUp.TradeNo, "concurrent mutation after MarkRefundPending")
		c.JSON(http.StatusConflict, gin.H{"message": "error", "data": "订单状态已变更"})
		return
	}

	// Re-read so we have the freshly-stamped refund_admin_id / refund_reason
	// when dispatching to the provider.
	topUp = model.GetTopUpByTradeNo(topUp.TradeNo)
	if topUp == nil {
		// Should not happen since we hold the lock, but defensive.
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "data": "订单不存在"})
		return
	}

	ctx := c.Request.Context()
	switch topUp.PaymentMethod {
	case PaymentMethodAlipay:
		dispatchAlipayRefund(c, ctx, topUp, req.Reason)
	case PaymentMethodWxpay:
		dispatchWxpayRefund(c, ctx, topUp, req.Reason)
	default:
		// Mark failed so the row reflects reality; operators can re-issue
		// after fixing the data or implementing the missing provider.
		_ = model.MarkRefundFailed(model.DB, topUp.TradeNo, "unsupported payment method: "+topUp.PaymentMethod)
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "该支付方式暂不支持退款"})
	}
}

// dispatchAlipayRefund handles the synchronous Alipay refund path. On SDK
// success we complete the refund row, deduct user quota and write the audit
// log in a single response cycle.
func dispatchAlipayRefund(c *gin.Context, ctx context.Context, topUp *model.TopUp, reason string) {
	svc, err := alipayServiceProvider()
	if err != nil || svc == nil {
		log.Printf("refund execute: alipay service unavailable tradeNo=%s err=%v", topUp.TradeNo, err)
		_ = model.MarkRefundFailed(model.DB, topUp.TradeNo, "alipay service unavailable")
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "data": "支付服务暂不可用"})
		return
	}

	// Deterministic out_request_no: "RFD" + tradeNo. Stable across retries so
	// if a network blip causes a resubmit, Alipay idempotently rejects the
	// duplicate instead of treating it as a brand-new refund.
	outRequestNo := "RFD" + topUp.TradeNo
	refundAmount := service.CentsToMoneyStr(topUp.PayAmountCents)

	rsp, err := svc.TradeRefund(ctx, topUp.TradeNo, refundAmount, outRequestNo, reason)
	if err != nil {
		log.Printf("refund execute: alipay TradeRefund failed tradeNo=%s err=%v", topUp.TradeNo, err)
		_ = model.MarkRefundFailed(model.DB, topUp.TradeNo, fmt.Sprintf("alipay sdk error: %v", err))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "退款失败"})
		return
	}

	refundedQuota := computeRefundQuota(topUp)
	refundTradeNo := outRequestNo
	if rsp != nil && rsp.TradeNo != "" {
		refundTradeNo = rsp.TradeNo
	}

	rowsAffected, err := model.CompleteRefund(model.DB, topUp.TradeNo, refundTradeNo, refundedQuota)
	if err != nil {
		log.Printf("refund execute: CompleteRefund failed tradeNo=%s err=%v", topUp.TradeNo, err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "data": "状态更新失败"})
		return
	}
	if rowsAffected == 0 {
		// Conditional update did not match — another path (cron reconciler /
		// wxpay refund notify / second admin click) already moved the row out
		// of refund_pending. Treat as idempotent success without deducting
		// quota again.
		log.Printf("refund execute: CompleteRefund idempotent skip tradeNo=%s (already finalized)", topUp.TradeNo)
		c.JSON(http.StatusOK, gin.H{
			"message": "success",
			"data": gin.H{
				"status":   common.RefundStatusSuccess,
				"trade_no": topUp.TradeNo,
			},
		})
		return
	}

	if refundedQuota > 0 {
		if err := model.DecreaseUserQuota(topUp.UserId, int(refundedQuota)); err != nil {
			// TODO(refund-reconcile): status is already refund_success but
			// quota was not deducted; ops needs to reconcile. We do not
			// roll back the refund row because the money has already moved
			// at the provider — surface via log and ack the API.
			log.Printf("refund execute: DecreaseUserQuota FAILED tradeNo=%s userId=%d quota=%d err=%v",
				topUp.TradeNo, topUp.UserId, refundedQuota, err)
		}
	}

	model.RecordLog(topUp.UserId, model.LogTypeRefund,
		fmt.Sprintf("管理员退款，订单号：%s，原因：%s，退还额度：%s",
			topUp.TradeNo, reason, logger.LogQuota(int(refundedQuota))))

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"status":   common.RefundStatusSuccess,
			"trade_no": topUp.TradeNo,
		},
	})
}

// dispatchWxpayRefund submits an asynchronous WeChat refund. WeChat only
// guarantees that the refund has been accepted at this point — final state
// (and quota deduction) is the responsibility of WxpayRefundNotify when
// WeChat calls us back. We deliberately keep the order in refund_pending.
func dispatchWxpayRefund(c *gin.Context, ctx context.Context, topUp *model.TopUp, reason string) {
	svc, err := wechatPayServiceProvider()
	if err != nil || svc == nil {
		log.Printf("refund execute: wxpay service unavailable tradeNo=%s err=%v", topUp.TradeNo, err)
		_ = model.MarkRefundFailed(model.DB, topUp.TradeNo, "wxpay service unavailable")
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "data": "支付服务暂不可用"})
		return
	}

	// outRefundNo (deterministic "RFD"+tradeNo) was already persisted as
	// refund_trade_no in RefundExecute right after MarkRefundPending. We
	// recompute it here so the SDK call uses the same value.
	outRefundNo := "RFD" + topUp.TradeNo

	if err := svc.Refund(ctx, topUp.TradeNo, outRefundNo, topUp.PayAmountCents, topUp.PayAmountCents, reason); err != nil {
		// The SDK error here is ambiguous: WeChat may have accepted the
		// refund before the transport failed. Marking refund_failed would
		// later block the (legitimate) async SUCCESS notification due to
		// the fail-closed gating in WxpayRefundNotify, silently losing the
		// money. Instead leave the row in refund_pending so that either
		//   (a) the async notify arrives and finalises it, or
		//   (b) ReconcileStaleRefundsPending flips it to refund_anomaly
		//       after 24h for manual operator review.
		log.Printf("refund execute: wxpay Refund SDK error tradeNo=%s err=%v (kept refund_pending for cron reconcile)",
			topUp.TradeNo, err)
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "退款请求已提交但确认失败，请稍后查看订单状态"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"status":   common.RefundStatusPending,
			"trade_no": topUp.TradeNo,
		},
	})
}
