package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func postWxpayRefundNotify(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/wxpay/refund/notify",
		strings.NewReader(`{}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	WxpayRefundNotify(ctx)
	return recorder
}

// seedRefundPendingTopUp creates a successful TopUp already advanced to the
// refund_pending stage (i.e. the /api/topup/refund handler ran successfully
// and is now waiting for WeChat's async callback).
func seedRefundPendingTopUp(t *testing.T, tradeNo string, userId int) *model.TopUp {
	t.Helper()
	topUp := &model.TopUp{
		UserId:         userId,
		Amount:         100,
		Money:          100.0,
		TradeNo:        tradeNo,
		PaymentMethod:  PaymentMethodWxpay,
		CreateTime:     1,
		Status:         common.TopUpStatusSuccess,
		PayAmountCents: 10000,
		QuotaGranted:   50000,
		RefundStatus:   common.RefundStatusPending,
		RefundAdminId:  2,
		RefundReason:   "test reason",
	}
	require.NoError(t, model.DB.Create(topUp).Error)
	return topUp
}

func TestWxpayRefundNotifyHappyPath(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	tradeNo := "wxpay_refund_ok"
	seedRefundPendingTopUp(t, tradeNo, user.Id)

	mock := &service.MockWechatPayService{
		DecryptRefundNotificationFunc: func(_ context.Context, _ *http.Request) (*service.RefundNotificationResult, error) {
			return &service.RefundNotificationResult{
				OutTradeNo:   tradeNo,
				OutRefundNo:  "RFD" + tradeNo + "1700000000",
				RefundId:     "wx_refund_id_42",
				RefundStatus: "SUCCESS",
				RefundAmount: 10000,
				SuccessTime:  1700000000,
			}, nil
		},
	}
	withWxpayService(t, mock)

	rec := postWxpayRefundNotify(t)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "SUCCESS")

	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", tradeNo).First(&loaded).Error)
	require.Equal(t, common.RefundStatusSuccess, loaded.RefundStatus)
	require.Equal(t, "wx_refund_id_42", loaded.RefundTradeNo)
	require.Equal(t, int64(50000), loaded.RefundedQuota)
	require.Greater(t, loaded.RefundTime, int64(0))

	// User quota deducted now (at notify time, not submit time).
	var u model.User
	require.NoError(t, db.First(&u, user.Id).Error)
	require.Equal(t, 500000-50000, u.Quota)

	// Exactly one refund log entry.
	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("user_id = ? AND type = ?", user.Id, model.LogTypeRefund).Count(&logCount).Error)
	require.Equal(t, int64(1), logCount)
}

func TestWxpayRefundNotifyIdempotent(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	tradeNo := "wxpay_refund_dup"
	topUp := seedRefundPendingTopUp(t, tradeNo, user.Id)
	// Pre-mark as already complete.
	topUp.RefundStatus = common.RefundStatusSuccess
	topUp.RefundedQuota = 50000
	require.NoError(t, db.Save(topUp).Error)
	require.NoError(t, model.DecreaseUserQuota(user.Id, 50000))

	mock := &service.MockWechatPayService{
		DecryptRefundNotificationFunc: func(_ context.Context, _ *http.Request) (*service.RefundNotificationResult, error) {
			return &service.RefundNotificationResult{
				OutTradeNo:   tradeNo,
				OutRefundNo:  "RFD",
				RefundStatus: "SUCCESS",
				RefundAmount: 10000,
			}, nil
		},
	}
	withWxpayService(t, mock)

	rec := postWxpayRefundNotify(t)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "SUCCESS")

	// Quota unchanged the second time around.
	var u model.User
	require.NoError(t, db.First(&u, user.Id).Error)
	require.Equal(t, 500000-50000, u.Quota, "quota must only be deducted once")

	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("user_id = ? AND type = ?", user.Id, model.LogTypeRefund).Count(&logCount).Error)
	require.Equal(t, int64(0), logCount, "no new log on idempotent skip")
}

func TestWxpayRefundNotifyFailedStatus(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	tradeNo := "wxpay_refund_failed"
	seedRefundPendingTopUp(t, tradeNo, user.Id)

	mock := &service.MockWechatPayService{
		DecryptRefundNotificationFunc: func(_ context.Context, _ *http.Request) (*service.RefundNotificationResult, error) {
			return &service.RefundNotificationResult{
				OutTradeNo:   tradeNo,
				OutRefundNo:  "RFD",
				RefundStatus: "ABNORMAL",
			}, nil
		},
	}
	withWxpayService(t, mock)

	rec := postWxpayRefundNotify(t)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "SUCCESS")

	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", tradeNo).First(&loaded).Error)
	require.Equal(t, common.RefundStatusFailed, loaded.RefundStatus)

	// Quota untouched on failure path.
	var u model.User
	require.NoError(t, db.First(&u, user.Id).Error)
	require.Equal(t, 500000, u.Quota)
}

func TestWxpayRefundNotifyBadSignature(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	tradeNo := "wxpay_refund_badsig"
	seedRefundPendingTopUp(t, tradeNo, user.Id)

	mock := &service.MockWechatPayService{
		DecryptRefundNotificationFunc: func(_ context.Context, _ *http.Request) (*service.RefundNotificationResult, error) {
			return nil, errWxpayMockFailure
		},
	}
	withWxpayService(t, mock)

	rec := postWxpayRefundNotify(t)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Contains(t, rec.Body.String(), "FAIL")

	// Nothing changed.
	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", tradeNo).First(&loaded).Error)
	require.Equal(t, common.RefundStatusPending, loaded.RefundStatus)
}

func TestWxpayRefundNotifyUnknownTradeNo(t *testing.T) {
	setupRefundTestDB(t)

	mock := &service.MockWechatPayService{
		DecryptRefundNotificationFunc: func(_ context.Context, _ *http.Request) (*service.RefundNotificationResult, error) {
			return &service.RefundNotificationResult{
				OutTradeNo:   "unknown_trade",
				RefundStatus: "SUCCESS",
			}, nil
		},
	}
	withWxpayService(t, mock)

	rec := postWxpayRefundNotify(t)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "FAIL")
}

func TestWxpayRefundNotifyMissingOutTradeNo(t *testing.T) {
	setupRefundTestDB(t)

	mock := &service.MockWechatPayService{
		DecryptRefundNotificationFunc: func(_ context.Context, _ *http.Request) (*service.RefundNotificationResult, error) {
			return &service.RefundNotificationResult{
				RefundStatus: "SUCCESS",
			}, nil
		},
	}
	withWxpayService(t, mock)

	rec := postWxpayRefundNotify(t)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestWxpayRefundNotifySuccessRejectsNonSuccessTopUp verifies fail-closed
// gating: if the parent topup is not in the success state (e.g. anomaly,
// pending), the handler MUST NOT proceed with CompleteRefund + quota
// deduction even when WeChat reports SUCCESS. The handler still acks 200
// SUCCESS to stop WeChat's retry storm but ops sees the warning in logs.
func TestWxpayRefundNotifySuccessRejectsNonSuccessTopUp(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	tradeNo := "wxpay_refund_bad_parent_status"

	// Seed a topup whose parent status is anomaly (not success) but whose
	// refund_status is pending — mimicking a corrupted/raced row.
	topUp := &model.TopUp{
		UserId:         user.Id,
		Amount:         100,
		Money:          100.0,
		TradeNo:        tradeNo,
		PaymentMethod:  PaymentMethodWxpay,
		CreateTime:     1,
		Status:         common.TopUpStatusAnomaly,
		PayAmountCents: 10000,
		QuotaGranted:   50000,
		RefundStatus:   common.RefundStatusPending,
		RefundAdminId:  2,
		RefundReason:   "test reason",
	}
	require.NoError(t, db.Create(topUp).Error)

	mock := &service.MockWechatPayService{
		DecryptRefundNotificationFunc: func(_ context.Context, _ *http.Request) (*service.RefundNotificationResult, error) {
			return &service.RefundNotificationResult{
				OutTradeNo:   tradeNo,
				OutRefundNo:  "RFD" + tradeNo,
				RefundId:     "wx_refund_id_42",
				RefundStatus: "SUCCESS",
				RefundAmount: 10000,
				SuccessTime:  1700000000,
			}, nil
		},
	}
	withWxpayService(t, mock)

	rec := postWxpayRefundNotify(t)
	// Ack 200 SUCCESS so WeChat stops retrying.
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "SUCCESS")

	// Refund state must NOT have flipped to success and quota must NOT have
	// been deducted.
	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", tradeNo).First(&loaded).Error)
	require.Equal(t, common.RefundStatusPending, loaded.RefundStatus,
		"RefundStatus must remain pending; gating must refuse to advance")
	require.Equal(t, int64(0), loaded.RefundedQuota)
	require.Equal(t, int64(0), loaded.RefundTime)

	var u model.User
	require.NoError(t, db.First(&u, user.Id).Error)
	require.Equal(t, 500000, u.Quota, "quota must not be deducted when parent topup is not in success state")

	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("user_id = ? AND type = ?", user.Id, model.LogTypeRefund).Count(&logCount).Error)
	require.Equal(t, int64(0), logCount, "no refund log when gating refuses the notify")
}

// TestWxpayRefundNotifySuccessRejectsNonPendingRefundStatus verifies the
// second fail-closed guard: if refund_status is not pending (e.g. empty
// or refund_failed) we MUST NOT deduct quota even when WeChat reports
// SUCCESS, because no admin actually initiated this refund through our
// own /api/topup/refund flow.
func TestWxpayRefundNotifySuccessRejectsNonPendingRefundStatus(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	tradeNo := "wxpay_refund_bad_refund_status"

	// Parent topup IS success, but refund_status is refund_failed (no admin
	// re-initiated the refund). A SUCCESS notify in this state is suspect.
	topUp := &model.TopUp{
		UserId:         user.Id,
		Amount:         100,
		Money:          100.0,
		TradeNo:        tradeNo,
		PaymentMethod:  PaymentMethodWxpay,
		CreateTime:     1,
		Status:         common.TopUpStatusSuccess,
		PayAmountCents: 10000,
		QuotaGranted:   50000,
		RefundStatus:   common.RefundStatusFailed,
	}
	require.NoError(t, db.Create(topUp).Error)

	mock := &service.MockWechatPayService{
		DecryptRefundNotificationFunc: func(_ context.Context, _ *http.Request) (*service.RefundNotificationResult, error) {
			return &service.RefundNotificationResult{
				OutTradeNo:   tradeNo,
				OutRefundNo:  "RFD" + tradeNo,
				RefundId:     "wx_refund_id_99",
				RefundStatus: "SUCCESS",
				RefundAmount: 10000,
				SuccessTime:  1700000000,
			}, nil
		},
	}
	withWxpayService(t, mock)

	rec := postWxpayRefundNotify(t)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "SUCCESS")

	// RefundStatus must stay refund_failed (unchanged), no quota deducted.
	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", tradeNo).First(&loaded).Error)
	require.Equal(t, common.RefundStatusFailed, loaded.RefundStatus,
		"RefundStatus must not flip from refund_failed when no admin initiated the refund")
	require.Equal(t, int64(0), loaded.RefundedQuota)
	require.Equal(t, int64(0), loaded.RefundTime)

	var u model.User
	require.NoError(t, db.First(&u, user.Id).Error)
	require.Equal(t, 500000, u.Quota,
		"quota must not be deducted when refund was not in pending state")

	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("user_id = ? AND type = ?", user.Id, model.LogTypeRefund).Count(&logCount).Error)
	require.Equal(t, int64(0), logCount)
}
