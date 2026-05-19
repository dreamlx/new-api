package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// staleRefundPendingTTL is the maximum time a refund_pending row may remain
// unresolved before ReconcileStaleRefundsPending flips it to refund_anomaly
// for manual investigation. WeChat Pay's spec retries refund callbacks for up
// to 16 hours; we use 24 hours to allow comfortable headroom over that window.
const staleRefundPendingTTL = 24 * time.Hour

// expiryAlipayProvider / expiryWechatProvider are indirections so the sweep
// can be tested with mocks without touching the production factory. In
// production both delegate to the shared NewXxxServiceFromSettings factory
// (also used by the controller-layer providers) so service construction
// stays consistent across HTTP and background paths.
var expiryAlipayProvider = func() (AlipayService, error) {
	return NewAlipayServiceFromSettings()
}

var expiryWechatProvider = func() (WechatPayService, error) {
	return NewWechatPayServiceFromSettings()
}

// CloseExpiredPendingTopUps scans for pending TopUps whose ExpireTime has
// passed and where RefundStatus is empty (no refund in flight). For each, it
// calls the provider's Close API and marks the row as "expired". Returns the
// counts of successful local closures and SDK failures.
//
// Safe to call concurrently — uses a conditional UPDATE keyed on status to
// ensure only one replica wins the close.
func CloseExpiredPendingTopUps(ctx context.Context) (closedOk, closeFailed int, err error) {
	db := model.DB
	if db == nil {
		return 0, 0, errors.New("model.DB is not initialised")
	}

	now := time.Now().Unix()
	var rows []model.TopUp
	err = db.Where("status = ? AND expire_time > 0 AND expire_time < ? AND refund_status = ?",
		common.TopUpStatusPending, now, common.RefundStatusNone).
		Find(&rows).Error
	if err != nil {
		return 0, 0, fmt.Errorf("scan expired pending topups: %w", err)
	}

	for _, row := range rows {
		// Respect cancellation between iterations; in-flight SDK calls have
		// their own context.
		if ctx.Err() != nil {
			return closedOk, closeFailed, ctx.Err()
		}

		sdkErr := callProviderClose(ctx, row.PaymentMethod, row.TradeNo)
		if sdkErr != nil {
			closeFailed++
			// We still mark the row expired locally below. WHY: leaving the
			// row pending lets the user retry payment after the provider has
			// (likely) already auto-closed the order — that produces a
			// "trade already closed" failure at pay time, plus we cannot
			// retry indefinitely. Closing locally bounds the user-visible
			// pending state to one expiry window.
			common.SysError(fmt.Sprintf("expiry sweep: provider close failed tradeNo=%s method=%s err=%v",
				row.TradeNo, row.PaymentMethod, sdkErr))
		}

		result := db.Model(&model.TopUp{}).
			Where("trade_no = ? AND status = ?", row.TradeNo, common.TopUpStatusPending).
			Update("status", common.TopUpStatusExpired)
		if result.Error != nil {
			common.SysError(fmt.Sprintf("expiry sweep: local close UPDATE failed tradeNo=%s err=%v",
				row.TradeNo, result.Error))
			closeFailed++
			continue
		}
		if result.RowsAffected == 0 {
			// Another replica or an inbound notify moved the row first.
			common.SysLog(fmt.Sprintf("expiry sweep: tradeNo=%s already moved, skipping", row.TradeNo))
			continue
		}
		if sdkErr == nil {
			closedOk++
			common.SysLog(fmt.Sprintf("expiry sweep: closed tradeNo=%s method=%s", row.TradeNo, row.PaymentMethod))
		}
	}

	return closedOk, closeFailed, nil
}

// callProviderClose dispatches to the SDK Close API matching the payment
// method. Unknown methods are silently skipped (return nil) — those are
// providers we do not own (e.g. legacy epay, stripe).
func callProviderClose(ctx context.Context, paymentMethod string, tradeNo string) error {
	switch paymentMethod {
	case "alipay":
		svc, err := expiryAlipayProvider()
		if err != nil {
			return err
		}
		_, err = svc.TradeClose(ctx, tradeNo)
		return err
	case "wxpay":
		svc, err := expiryWechatProvider()
		if err != nil {
			return err
		}
		return svc.CloseOrder(ctx, tradeNo)
	default:
		return nil
	}
}

// ReconcileStaleRefundsPending finds refund_pending rows older than
// staleRefundPendingTTL and flips them to refund_anomaly so operators can
// reconcile against the provider's dashboard. Returns the count of rows
// flipped.
//
// This guards against silently dropped WeChat refund notifications (the
// payment provider retries for ~16h then gives up); without this sweep such
// rows would remain refund_pending forever.
func ReconcileStaleRefundsPending(ctx context.Context) (markedAnomaly int, err error) {
	db := model.DB
	if db == nil {
		return 0, errors.New("model.DB is not initialised")
	}

	cutoff := time.Now().Add(-staleRefundPendingTTL).Unix()
	var rows []model.TopUp
	err = db.Where("refund_status = ? AND refund_request_time > 0 AND refund_request_time < ?",
		common.RefundStatusPending, cutoff).
		Find(&rows).Error
	if err != nil {
		return 0, fmt.Errorf("scan stale refund_pending: %w", err)
	}

	for _, row := range rows {
		if ctx.Err() != nil {
			return markedAnomaly, ctx.Err()
		}
		reason := "refund stuck in pending > 24h, manual reconciliation required"
		if markErr := model.MarkRefundAnomaly(db, row.TradeNo, reason); markErr != nil {
			common.SysError(fmt.Sprintf("refund reconcile: mark anomaly failed tradeNo=%s err=%v",
				row.TradeNo, markErr))
			continue
		}
		markedAnomaly++
		common.SysLog(fmt.Sprintf("refund reconcile: tradeNo=%s flipped to refund_anomaly (stale > 24h)", row.TradeNo))
	}

	return markedAnomaly, nil
}
