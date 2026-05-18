package controller

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/smartwalle/alipay/v3"
	"github.com/stretchr/testify/require"
)

func newRefundExecuteCtx(t *testing.T, body string, role int, adminId int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", role)
	ctx.Set("id", adminId)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/topup/refund",
		bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx, recorder
}

func mintValidConfirmToken(t *testing.T, tradeNo string, adminId int) (string, int64) {
	t.Helper()
	exp := time.Now().Add(refundConfirmTokenTTL).Unix()
	return signConfirmToken(tradeNo, adminId, exp), exp
}

func TestRefundExecuteRejectsNonRoot(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	seedRefundSuccessTopUp(t, db, "exec_non_root", PaymentMethodAlipay, user.Id)

	token, _ := mintValidConfirmToken(t, "exec_non_root", 99)
	body := `{"trade_no":"exec_non_root","confirm_token":"` + token + `","reason":"test"}`
	ctx, rec := newRefundExecuteCtx(t, body, common.RoleAdminUser, 99)
	RefundExecute(ctx)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestRefundExecuteRejectsBadToken(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	admin := seedRefundAdmin(t, db)
	seedRefundSuccessTopUp(t, db, "exec_bad_token", PaymentMethodAlipay, user.Id)

	body := `{"trade_no":"exec_bad_token","confirm_token":"v1.99999999999.deadbeef","reason":"test"}`
	ctx, rec := newRefundExecuteCtx(t, body, common.RoleRootUser, admin.Id)
	RefundExecute(ctx)

	require.Equal(t, http.StatusForbidden, rec.Code)

	// Topup state must NOT change on bad token.
	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "exec_bad_token").First(&loaded).Error)
	require.Equal(t, common.RefundStatusNone, loaded.RefundStatus)
}

func TestRefundExecuteRejectsTokenMintedForOtherAdmin(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	admin := seedRefundAdmin(t, db)
	seedRefundSuccessTopUp(t, db, "exec_wrong_admin", PaymentMethodAlipay, user.Id)

	// Token minted for a different admin id.
	token, _ := mintValidConfirmToken(t, "exec_wrong_admin", admin.Id+10)
	body := `{"trade_no":"exec_wrong_admin","confirm_token":"` + token + `","reason":"x"}`
	ctx, rec := newRefundExecuteCtx(t, body, common.RoleRootUser, admin.Id)
	RefundExecute(ctx)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestRefundExecuteRejectsExpiredToken(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	admin := seedRefundAdmin(t, db)
	seedRefundSuccessTopUp(t, db, "exec_expired", PaymentMethodAlipay, user.Id)

	exp := time.Now().Add(-time.Minute).Unix()
	token := signConfirmToken("exec_expired", admin.Id, exp)
	body := `{"trade_no":"exec_expired","confirm_token":"` + token + `","reason":"x"}`
	ctx, rec := newRefundExecuteCtx(t, body, common.RoleRootUser, admin.Id)
	RefundExecute(ctx)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestRefundExecuteRejectsAlreadyRefunded(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	admin := seedRefundAdmin(t, db)
	topUp := seedRefundSuccessTopUp(t, db, "exec_already_refunded", PaymentMethodAlipay, user.Id)
	topUp.RefundStatus = common.RefundStatusSuccess
	require.NoError(t, db.Save(topUp).Error)

	token, _ := mintValidConfirmToken(t, "exec_already_refunded", admin.Id)
	body := `{"trade_no":"exec_already_refunded","confirm_token":"` + token + `","reason":"x"}`
	ctx, rec := newRefundExecuteCtx(t, body, common.RoleRootUser, admin.Id)
	RefundExecute(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), common.RefundStatusSuccess)
}

func TestRefundExecuteAlipayHappyPath(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	admin := seedRefundAdmin(t, db)
	topUp := seedRefundSuccessTopUp(t, db, "exec_alipay_ok", PaymentMethodAlipay, user.Id)

	var called bool
	var capturedOutRequestNo string
	var capturedReason string
	var capturedRefundAmount string
	mock := &service.MockAlipayService{
		TradeRefundFunc: func(_ context.Context, outTradeNo, refundAmount, outRequestNo, refundReason string) (*alipay.TradeRefundRsp, error) {
			called = true
			capturedOutRequestNo = outRequestNo
			capturedReason = refundReason
			capturedRefundAmount = refundAmount
			return &alipay.TradeRefundRsp{TradeNo: "alipay_refund_tx_1"}, nil
		},
	}
	withAlipayService(t, mock)

	token, _ := mintValidConfirmToken(t, "exec_alipay_ok", admin.Id)
	body := `{"trade_no":"exec_alipay_ok","confirm_token":"` + token + `","reason":"用户申诉"}`
	ctx, rec := newRefundExecuteCtx(t, body, common.RoleRootUser, admin.Id)
	RefundExecute(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, called)
	require.NotEmpty(t, capturedOutRequestNo)
	require.Equal(t, "用户申诉", capturedReason)
	require.Equal(t, "100.00", capturedRefundAmount)
	require.Contains(t, rec.Body.String(), common.RefundStatusSuccess)

	// Refund state machine: pending -> success
	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "exec_alipay_ok").First(&loaded).Error)
	require.Equal(t, common.RefundStatusSuccess, loaded.RefundStatus)
	require.Equal(t, "alipay_refund_tx_1", loaded.RefundTradeNo)
	require.Equal(t, admin.Id, loaded.RefundAdminId)
	require.Equal(t, "用户申诉", loaded.RefundReason)
	require.Equal(t, int64(50000), loaded.RefundedQuota)
	require.Greater(t, loaded.RefundTime, int64(0))

	// Quota deducted from user.
	var u model.User
	require.NoError(t, db.First(&u, user.Id).Error)
	require.Equal(t, 500000-50000, u.Quota)

	// LogTypeRefund log recorded.
	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("user_id = ? AND type = ?", user.Id, model.LogTypeRefund).Count(&logCount).Error)
	require.Equal(t, int64(1), logCount, "expected one refund log entry")

	// out_request_no must be deterministic: exactly "RFD"+tradeNo so retries
	// produce the same id and Alipay can idempotently reject duplicates.
	require.Equal(t, "RFD"+topUp.TradeNo, capturedOutRequestNo)
}

func TestRefundExecuteAlipaySDKError(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	admin := seedRefundAdmin(t, db)
	topUp := seedRefundSuccessTopUp(t, db, "exec_alipay_err", PaymentMethodAlipay, user.Id)

	mock := &service.MockAlipayService{
		TradeRefundFunc: func(_ context.Context, _, _, _, _ string) (*alipay.TradeRefundRsp, error) {
			return nil, errors.New("alipay refund declined")
		},
	}
	withAlipayService(t, mock)

	token, _ := mintValidConfirmToken(t, "exec_alipay_err", admin.Id)
	body := `{"trade_no":"exec_alipay_err","confirm_token":"` + token + `","reason":"x"}`
	ctx, rec := newRefundExecuteCtx(t, body, common.RoleRootUser, admin.Id)
	RefundExecute(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "error")

	// Order must be in refund_failed, NOT refund_success.
	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "exec_alipay_err").First(&loaded).Error)
	require.Equal(t, common.RefundStatusFailed, loaded.RefundStatus)

	// User quota unchanged.
	var u model.User
	require.NoError(t, db.First(&u, user.Id).Error)
	require.Equal(t, 500000, u.Quota)

	// No refund log written.
	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("user_id = ? AND type = ?", topUp.UserId, model.LogTypeRefund).Count(&logCount).Error)
	require.Equal(t, int64(0), logCount)
}

func TestRefundExecuteWxpayHappyPathSyncPending(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	admin := seedRefundAdmin(t, db)
	seedRefundSuccessTopUp(t, db, "exec_wx_ok", PaymentMethodWxpay, user.Id)

	var capturedOutRefundNo string
	var capturedTotal int64
	var capturedRefund int64
	mock := &service.MockWechatPayService{
		RefundFunc: func(_ context.Context, outTradeNo, outRefundNo string, totalCents, refundCents int64, reason string) error {
			capturedOutRefundNo = outRefundNo
			capturedTotal = totalCents
			capturedRefund = refundCents
			return nil
		},
	}
	withWxpayService(t, mock)

	token, _ := mintValidConfirmToken(t, "exec_wx_ok", admin.Id)
	body := `{"trade_no":"exec_wx_ok","confirm_token":"` + token + `","reason":"误充值"}`
	ctx, rec := newRefundExecuteCtx(t, body, common.RoleRootUser, admin.Id)
	RefundExecute(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), common.RefundStatusPending)
	require.Equal(t, int64(10000), capturedTotal)
	require.Equal(t, int64(10000), capturedRefund)
	require.NotEmpty(t, capturedOutRefundNo)

	// State machine still pending; quota NOT yet deducted; no log yet.
	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "exec_wx_ok").First(&loaded).Error)
	require.Equal(t, common.RefundStatusPending, loaded.RefundStatus)
	require.Equal(t, admin.Id, loaded.RefundAdminId)
	require.Equal(t, capturedOutRefundNo, loaded.RefundTradeNo, "out_refund_no should be persisted for notify correlation")

	var u model.User
	require.NoError(t, db.First(&u, user.Id).Error)
	require.Equal(t, 500000, u.Quota, "wxpay refund is async; quota must NOT be deducted at submit time")

	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("user_id = ? AND type = ?", user.Id, model.LogTypeRefund).Count(&logCount).Error)
	require.Equal(t, int64(0), logCount, "no refund log until WeChat confirms")
}

func TestRefundExecuteWxpaySDKError(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	admin := seedRefundAdmin(t, db)
	seedRefundSuccessTopUp(t, db, "exec_wx_err", PaymentMethodWxpay, user.Id)

	mock := &service.MockWechatPayService{
		RefundFunc: func(_ context.Context, _, _ string, _, _ int64, _ string) error {
			return errors.New("wxpay refund declined")
		},
	}
	withWxpayService(t, mock)

	token, _ := mintValidConfirmToken(t, "exec_wx_err", admin.Id)
	body := `{"trade_no":"exec_wx_err","confirm_token":"` + token + `","reason":"x"}`
	ctx, rec := newRefundExecuteCtx(t, body, common.RoleRootUser, admin.Id)
	RefundExecute(ctx)

	require.Contains(t, rec.Body.String(), "error")

	// WeChat refund SDK errors are ambiguous (the provider may have accepted
	// the refund before the transport failed). The row stays in refund_pending
	// so that either (a) a delayed async SUCCESS notification finalises it,
	// or (b) ReconcileStaleRefundsPending flips it to refund_anomaly after
	// 24h for manual review. Moving it to refund_failed here would
	// silently block (a), losing the money. See fix in commit applying
	// review findings.
	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "exec_wx_err").First(&loaded).Error)
	require.Equal(t, common.RefundStatusPending, loaded.RefundStatus,
		"wxpay SDK error must keep refund_pending for cron reconcile, not flip to refund_failed")

	var u model.User
	require.NoError(t, db.First(&u, user.Id).Error)
	require.Equal(t, 500000, u.Quota)
}

func TestRefundExecuteUnsupportedPaymentMethod(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	admin := seedRefundAdmin(t, db)
	seedRefundSuccessTopUp(t, db, "exec_unsupported", "stripe", user.Id)

	token, _ := mintValidConfirmToken(t, "exec_unsupported", admin.Id)
	body := `{"trade_no":"exec_unsupported","confirm_token":"` + token + `","reason":"x"}`
	ctx, rec := newRefundExecuteCtx(t, body, common.RoleRootUser, admin.Id)
	RefundExecute(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "error")

	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "exec_unsupported").First(&loaded).Error)
	require.Equal(t, common.RefundStatusFailed, loaded.RefundStatus,
		"unsupported method must mark refund_failed so operators see why nothing happened")
}

func TestRefundExecuteIdempotentConcurrentCall(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	admin := seedRefundAdmin(t, db)
	topUp := seedRefundSuccessTopUp(t, db, "exec_idempotent", PaymentMethodAlipay, user.Id)

	// Pre-mark the order as refund_pending, simulating another replica that
	// already won the race. The handler should respond success without
	// hitting the SDK again.
	_, err := model.MarkRefundPending(model.DB, topUp.TradeNo, admin.Id, "earlier")
	require.NoError(t, err)

	var sdkCalled bool
	mock := &service.MockAlipayService{
		TradeRefundFunc: func(_ context.Context, _, _, _, _ string) (*alipay.TradeRefundRsp, error) {
			sdkCalled = true
			return &alipay.TradeRefundRsp{}, nil
		},
	}
	withAlipayService(t, mock)

	token, _ := mintValidConfirmToken(t, topUp.TradeNo, admin.Id)
	body := `{"trade_no":"` + topUp.TradeNo + `","confirm_token":"` + token + `","reason":"second click"}`
	ctx, rec := newRefundExecuteCtx(t, body, common.RoleRootUser, admin.Id)
	RefundExecute(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.False(t, sdkCalled, "SDK MUST NOT be called when the order is already refund_pending")
	require.Contains(t, rec.Body.String(), common.RefundStatusPending)
}
