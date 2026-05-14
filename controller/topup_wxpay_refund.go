package controller

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// wxpayRefundStatusSuccess is the SDK literal for a successful refund
// (refunddomestic.STATUS_SUCCESS). Mirrored here as an untyped constant so
// we do not have to import the SDK enums into the controller.
const wxpayRefundStatusSuccess = "SUCCESS"

// WxpayRefundNotify completes the asynchronous WeChat refund flow.
//
// POST /api/user/wxpay/refund/notify (application/json, signed + AES-GCM encrypted)
//
// Public route: WeChat servers cannot authenticate against our session/JWT,
// so the authentication boundary is the SDK's RSA signature verification.
// We MUST reject anything that fails decryption — otherwise an attacker
// could forge refund completions.
//
// State transitions:
//   - SUCCESS + order in refund_pending  -> CompleteRefund + DecreaseUserQuota + RecordLog
//   - SUCCESS + order already refund_success -> idempotent ack
//   - anything else -> MarkRefundFailed with the SDK status as the reason
//
// Idempotency is enforced by the state-machine guard: CompleteRefund updates
// unconditionally, but we only run quota deduction + log once per row by
// checking RefundStatus before acting.
func WxpayRefundNotify(c *gin.Context) {
	svc, err := wechatPayServiceProvider()
	if err != nil || svc == nil {
		log.Printf("wxpay refund notify: service unavailable: %v", err)
		wxpayFail(c, http.StatusInternalServerError, "service unavailable")
		return
	}

	ctx := context.Background()
	result, err := svc.DecryptRefundNotification(ctx, c.Request)
	if err != nil || result == nil {
		log.Printf("wxpay refund notify: decrypt failed: %v", err)
		wxpayFail(c, http.StatusUnauthorized, "decrypt failed")
		return
	}

	tradeNo := result.OutTradeNo
	if tradeNo == "" {
		log.Printf("wxpay refund notify: missing out_trade_no")
		wxpayFail(c, http.StatusBadRequest, "missing out_trade_no")
		return
	}

	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil {
		log.Printf("wxpay refund notify: top-up not found tradeNo=%s", tradeNo)
		wxpayFail(c, http.StatusNotFound, "topup not found")
		return
	}

	if result.RefundStatus != wxpayRefundStatusSuccess {
		// Non-success terminal states (CLOSED/ABNORMAL) — record failure.
		// PROCESSING is technically intermediate, but WeChat does not retry
		// these as actionable notifications, so treating them as failed and
		// letting ops re-issue is the safest default.
		reason := fmt.Sprintf("wxpay refund non-success status=%s", result.RefundStatus)
		log.Printf("wxpay refund notify: %s tradeNo=%s", reason, tradeNo)
		if topUp.RefundStatus == common.RefundStatusPending {
			if err := model.MarkRefundFailed(model.DB, tradeNo, reason); err != nil {
				log.Printf("wxpay refund notify: MarkRefundFailed err=%v tradeNo=%s", err, tradeNo)
			}
		}
		// Ack success to stop retries: we have observed and recorded the
		// outcome. Re-driving via the cron job will recover if needed.
		wxpaySuccess(c)
		return
	}

	// SUCCESS path. If we already completed (idempotent re-delivery), ack.
	if topUp.RefundStatus == common.RefundStatusSuccess {
		log.Printf("wxpay refund notify: idempotent skip tradeNo=%s (already refund_success)", tradeNo)
		wxpaySuccess(c)
		return
	}

	// Only act when we are transitioning out of refund_pending. If the row
	// is in an unexpected state (e.g. status not success, or refund_status
	// is empty/failed), still flip to success per WeChat's authoritative
	// answer but log loudly so ops can investigate the upstream sequence.
	if topUp.RefundStatus != common.RefundStatusPending {
		log.Printf("wxpay refund notify: unexpected refund_status=%q tradeNo=%s; trusting upstream SUCCESS",
			topUp.RefundStatus, tradeNo)
	}

	refundedQuota := computeRefundQuota(topUp)
	refundTradeNo := result.OutRefundNo
	if result.RefundId != "" {
		refundTradeNo = result.RefundId
	}

	if err := model.CompleteRefund(model.DB, tradeNo, refundTradeNo, refundedQuota); err != nil {
		log.Printf("wxpay refund notify: CompleteRefund failed tradeNo=%s err=%v", tradeNo, err)
		wxpayFail(c, http.StatusInternalServerError, "db error")
		return
	}

	if refundedQuota > 0 {
		if err := model.DecreaseUserQuota(topUp.UserId, int(refundedQuota)); err != nil {
			// TODO(refund-reconcile): refund_status is now success but quota
			// deduction failed. Surface via log; the response stays SUCCESS so
			// WeChat does not retry-storm. Ops reconciles via this log line.
			log.Printf("wxpay refund notify: DecreaseUserQuota FAILED tradeNo=%s userId=%d quota=%d err=%v",
				tradeNo, topUp.UserId, refundedQuota, err)
		}
	}

	reason := topUp.RefundReason
	model.RecordLog(topUp.UserId, model.LogTypeRefund,
		fmt.Sprintf("管理员退款，订单号：%s，原因：%s，退还额度：%s",
			tradeNo, reason, logger.LogQuota(int(refundedQuota))))

	wxpaySuccess(c)
}
