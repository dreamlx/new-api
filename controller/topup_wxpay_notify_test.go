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
	"gorm.io/gorm"
)

// seedWxpayPendingTopUp inserts a user + pending wxpay TopUp row.
func seedWxpayPendingTopUp(t *testing.T, db *gorm.DB, tradeNo string, payAmountCents int64) *model.TopUp {
	t.Helper()
	user := seedWxpayUser(t, db)
	topUp := &model.TopUp{
		UserId:         user.Id,
		Amount:         100,
		Money:          1.0,
		TradeNo:        tradeNo,
		PaymentMethod:  PaymentMethodWxpay,
		CreateTime:     1,
		Status:         common.TopUpStatusPending,
		PayAmountCents: payAmountCents,
	}
	require.NoError(t, db.Create(topUp).Error)
	return topUp
}

func postWxpayNotify(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/wxpay/notify",
		strings.NewReader(`{}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	WxpayNotify(ctx)
	return recorder
}

func TestWxpayNotifyHappyPath(t *testing.T) {
	db := setupWxpayControllerTestDB(t)
	withWxpayEnabled(t, true)
	topUp := seedWxpayPendingTopUp(t, db, "wxpay_notify_001", 10000)

	mock := &service.MockWechatPayService{
		DecryptNotificationFunc: func(_ context.Context, _ *http.Request) (*service.NotificationResult, error) {
			return &service.NotificationResult{
				OutTradeNo:    "wxpay_notify_001",
				TransactionId: "wx_tx_001",
				TradeState:    "SUCCESS",
				AmountTotal:   10000,
				PaidAt:        1700000000,
				Raw:           `{"out_trade_no":"wxpay_notify_001"}`,
			}, nil
		},
	}
	withWxpayService(t, mock)

	recorder := postWxpayNotify(t)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "SUCCESS")

	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "wxpay_notify_001").First(&loaded).Error)
	require.Equal(t, common.TopUpStatusSuccess, loaded.Status)
	require.Equal(t, "wx_tx_001", loaded.ProviderTxId)
	require.Contains(t, loaded.CallbackRaw, "wxpay_notify_001")

	var user model.User
	require.NoError(t, db.First(&user, topUp.UserId).Error)
	expectedQuota := 100 + int(float64(topUp.Amount)*common.QuotaPerUnit)
	require.Equal(t, expectedQuota, user.Quota)
}

func TestWxpayNotifyDuplicateCallback(t *testing.T) {
	db := setupWxpayControllerTestDB(t)
	withWxpayEnabled(t, true)
	topUp := seedWxpayPendingTopUp(t, db, "wxpay_notify_dup", 10000)

	mock := &service.MockWechatPayService{
		DecryptNotificationFunc: func(_ context.Context, _ *http.Request) (*service.NotificationResult, error) {
			return &service.NotificationResult{
				OutTradeNo:    "wxpay_notify_dup",
				TransactionId: "wx_tx_dup",
				TradeState:    "SUCCESS",
				AmountTotal:   10000,
				PaidAt:        1700000000,
				Raw:           `{"out_trade_no":"wxpay_notify_dup"}`,
			}, nil
		},
	}
	withWxpayService(t, mock)

	rec1 := postWxpayNotify(t)
	require.Equal(t, http.StatusOK, rec1.Code)
	require.Contains(t, rec1.Body.String(), "SUCCESS")

	rec2 := postWxpayNotify(t)
	require.Equal(t, http.StatusOK, rec2.Code)
	require.Contains(t, rec2.Body.String(), "SUCCESS")

	var user model.User
	require.NoError(t, db.First(&user, topUp.UserId).Error)
	expectedQuota := 100 + int(float64(topUp.Amount)*common.QuotaPerUnit)
	require.Equal(t, expectedQuota, user.Quota, "quota must only be granted once")

	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "wxpay_notify_dup").First(&loaded).Error)
	require.Contains(t, loaded.CallbackRaw, "wxpay_notify_dup")
}

func TestWxpayNotifyAmountMismatch(t *testing.T) {
	db := setupWxpayControllerTestDB(t)
	withWxpayEnabled(t, true)
	topUp := seedWxpayPendingTopUp(t, db, "wxpay_notify_mismatch", 10000)

	mock := &service.MockWechatPayService{
		DecryptNotificationFunc: func(_ context.Context, _ *http.Request) (*service.NotificationResult, error) {
			return &service.NotificationResult{
				OutTradeNo:    "wxpay_notify_mismatch",
				TransactionId: "wx_tx_mismatch",
				TradeState:    "SUCCESS",
				AmountTotal:   5000,
				PaidAt:        1700000000,
			}, nil
		},
	}
	withWxpayService(t, mock)

	recorder := postWxpayNotify(t)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "FAIL")

	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "wxpay_notify_mismatch").First(&loaded).Error)
	require.Equal(t, common.TopUpStatusAnomaly, loaded.Status)
	require.Contains(t, loaded.CallbackRaw, "mismatch")

	var user model.User
	require.NoError(t, db.First(&user, topUp.UserId).Error)
	require.Equal(t, 100, user.Quota, "quota must not be granted on amount mismatch")
}

func TestWxpayNotifyBadSignature(t *testing.T) {
	db := setupWxpayControllerTestDB(t)
	withWxpayEnabled(t, true)
	topUp := seedWxpayPendingTopUp(t, db, "wxpay_notify_badsig", 10000)

	mock := &service.MockWechatPayService{
		DecryptNotificationFunc: func(_ context.Context, _ *http.Request) (*service.NotificationResult, error) {
			return nil, errWxpayMockFailure
		},
	}
	withWxpayService(t, mock)

	recorder := postWxpayNotify(t)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Contains(t, recorder.Body.String(), "FAIL")

	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "wxpay_notify_badsig").First(&loaded).Error)
	require.Equal(t, common.TopUpStatusPending, loaded.Status, "status must not change on bad signature")

	var user model.User
	require.NoError(t, db.First(&user, topUp.UserId).Error)
	require.Equal(t, 100, user.Quota)
}

func TestWxpayNotifyTopUpNotFound(t *testing.T) {
	setupWxpayControllerTestDB(t)
	withWxpayEnabled(t, true)

	mock := &service.MockWechatPayService{
		DecryptNotificationFunc: func(_ context.Context, _ *http.Request) (*service.NotificationResult, error) {
			return &service.NotificationResult{
				OutTradeNo:    "wxpay_notify_missing",
				TransactionId: "wx_tx_missing",
				TradeState:    "SUCCESS",
				AmountTotal:   10000,
				PaidAt:        1700000000,
			}, nil
		},
	}
	withWxpayService(t, mock)

	recorder := postWxpayNotify(t)
	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Body.String(), "FAIL")
}

func TestWxpayNotifyNonSuccessTradeState(t *testing.T) {
	db := setupWxpayControllerTestDB(t)
	withWxpayEnabled(t, true)
	topUp := seedWxpayPendingTopUp(t, db, "wxpay_notify_notpay", 10000)

	mock := &service.MockWechatPayService{
		DecryptNotificationFunc: func(_ context.Context, _ *http.Request) (*service.NotificationResult, error) {
			return &service.NotificationResult{
				OutTradeNo:    "wxpay_notify_notpay",
				TransactionId: "",
				TradeState:    "NOTPAY",
				AmountTotal:   10000,
			}, nil
		},
	}
	withWxpayService(t, mock)

	recorder := postWxpayNotify(t)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "SUCCESS")

	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "wxpay_notify_notpay").First(&loaded).Error)
	require.Equal(t, common.TopUpStatusPending, loaded.Status, "status must not change on non-SUCCESS trade_state")

	var user model.User
	require.NoError(t, db.First(&user, topUp.UserId).Error)
	require.Equal(t, 100, user.Quota)
}
