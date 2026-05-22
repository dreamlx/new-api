package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/smartwalle/alipay/v3"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedAlipayPendingTopUp inserts a user + pending alipay TopUp row.
func seedAlipayPendingTopUp(t *testing.T, db *gorm.DB, tradeNo string, payAmountCents int64) *model.TopUp {
	t.Helper()
	user := seedAlipayUser(t, db)
	topUp := &model.TopUp{
		UserId:         user.Id,
		Amount:         100,
		Money:          1.0,
		TradeNo:        tradeNo,
		PaymentMethod:  PaymentMethodAlipay,
		CreateTime:     1,
		Status:         common.TopUpStatusPending,
		PayAmountCents: payAmountCents,
	}
	require.NoError(t, db.Create(topUp).Error)
	return topUp
}

func postAlipayNotify(t *testing.T, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/alipay/notify",
		strings.NewReader(form.Encode()))
	ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	AlipayNotify(ctx)
	return recorder
}

func validAlipayNotificationForm(tradeNo string, totalAmount string) url.Values {
	v := url.Values{}
	v.Set("app_id", "test_app_id")
	v.Set("seller_id", "test_seller_id")
	v.Set("out_trade_no", tradeNo)
	v.Set("trade_no", "alipay_tx_"+tradeNo)
	v.Set("total_amount", totalAmount)
	v.Set("trade_status", "TRADE_SUCCESS")
	v.Set("notify_id", "ntf_"+tradeNo)
	v.Set("notify_time", "2026-05-14 12:00:00")
	v.Set("gmt_payment", "2026-05-14 12:00:01")
	v.Set("charset", "utf-8")
	return v
}

func buildDecodedNotification(vals url.Values) *alipay.Notification {
	return &alipay.Notification{
		AppId:       vals.Get("app_id"),
		SellerId:    vals.Get("seller_id"),
		OutTradeNo:  vals.Get("out_trade_no"),
		TradeNo:     vals.Get("trade_no"),
		TotalAmount: vals.Get("total_amount"),
		TradeStatus: alipay.TradeStatus(vals.Get("trade_status")),
		NotifyId:    vals.Get("notify_id"),
		GmtPayment:  vals.Get("gmt_payment"),
	}
}

func TestAlipayNotifyHappyPath(t *testing.T) {
	db := setupAlipayControllerTestDB(t)
	withAlipayEnabled(t, true)
	topUp := seedAlipayPendingTopUp(t, db, "alipay_notify_001", 10000) // 100.00 CNY

	form := validAlipayNotificationForm("alipay_notify_001", "100.00")

	mock := &service.MockAlipayService{
		VerifySignFunc: func(_ context.Context, _ url.Values) error { return nil },
		DecodeNotificationFunc: func(_ context.Context, vals url.Values) (*alipay.Notification, error) {
			return buildDecodedNotification(vals), nil
		},
	}
	withAlipayService(t, mock)

	recorder := postAlipayNotify(t, form)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "success", recorder.Body.String())

	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "alipay_notify_001").First(&loaded).Error)
	require.Equal(t, common.TopUpStatusSuccess, loaded.Status)
	require.Equal(t, "alipay_tx_alipay_notify_001", loaded.ProviderTxId)
	require.Contains(t, loaded.CallbackRaw, "alipay_notify_001")

	var user model.User
	require.NoError(t, db.First(&user, topUp.UserId).Error)
	expectedQuota := 100 + int(float64(topUp.Amount)*common.QuotaPerUnit)
	require.Equal(t, expectedQuota, user.Quota)
}

func TestAlipayNotifyDuplicateCallback(t *testing.T) {
	db := setupAlipayControllerTestDB(t)
	withAlipayEnabled(t, true)
	topUp := seedAlipayPendingTopUp(t, db, "alipay_notify_dup", 10000)

	form := validAlipayNotificationForm("alipay_notify_dup", "100.00")

	mock := &service.MockAlipayService{
		VerifySignFunc: func(_ context.Context, _ url.Values) error { return nil },
		DecodeNotificationFunc: func(_ context.Context, vals url.Values) (*alipay.Notification, error) {
			return buildDecodedNotification(vals), nil
		},
	}
	withAlipayService(t, mock)

	rec1 := postAlipayNotify(t, form)
	require.Equal(t, "success", rec1.Body.String())

	rec2 := postAlipayNotify(t, form)
	require.Equal(t, http.StatusOK, rec2.Code)
	require.Equal(t, "success", rec2.Body.String())

	var user model.User
	require.NoError(t, db.First(&user, topUp.UserId).Error)
	expectedQuota := 100 + int(float64(topUp.Amount)*common.QuotaPerUnit)
	require.Equal(t, expectedQuota, user.Quota, "quota must only be granted once")

	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "alipay_notify_dup").First(&loaded).Error)
	require.Contains(t, loaded.CallbackRaw, "alipay_notify_dup")
}

func TestAlipayNotifyAmountMismatch(t *testing.T) {
	db := setupAlipayControllerTestDB(t)
	withAlipayEnabled(t, true)
	topUp := seedAlipayPendingTopUp(t, db, "alipay_notify_mismatch", 10000) // expects 100.00

	form := validAlipayNotificationForm("alipay_notify_mismatch", "50.00")

	mock := &service.MockAlipayService{
		VerifySignFunc: func(_ context.Context, _ url.Values) error { return nil },
		DecodeNotificationFunc: func(_ context.Context, vals url.Values) (*alipay.Notification, error) {
			return buildDecodedNotification(vals), nil
		},
	}
	withAlipayService(t, mock)

	recorder := postAlipayNotify(t, form)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "failure", recorder.Body.String())

	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "alipay_notify_mismatch").First(&loaded).Error)
	require.Equal(t, common.TopUpStatusAnomaly, loaded.Status)
	require.Contains(t, loaded.CallbackRaw, "mismatch")

	var user model.User
	require.NoError(t, db.First(&user, topUp.UserId).Error)
	require.Equal(t, 100, user.Quota, "quota must not be granted on amount mismatch")
}

func TestAlipayNotifyBadSignature(t *testing.T) {
	db := setupAlipayControllerTestDB(t)
	withAlipayEnabled(t, true)
	topUp := seedAlipayPendingTopUp(t, db, "alipay_notify_badsig", 10000)

	form := validAlipayNotificationForm("alipay_notify_badsig", "100.00")

	mock := &service.MockAlipayService{
		VerifySignFunc: func(_ context.Context, _ url.Values) error { return errAlipayMockFailure },
		DecodeNotificationFunc: func(_ context.Context, _ url.Values) (*alipay.Notification, error) {
			return nil, errAlipayMockFailure
		},
	}
	withAlipayService(t, mock)

	recorder := postAlipayNotify(t, form)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "failure", recorder.Body.String())

	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "alipay_notify_badsig").First(&loaded).Error)
	require.Equal(t, common.TopUpStatusPending, loaded.Status, "status must not change on bad signature")

	var user model.User
	require.NoError(t, db.First(&user, topUp.UserId).Error)
	require.Equal(t, 100, user.Quota)
}

func TestAlipayNotifyTopUpNotFound(t *testing.T) {
	setupAlipayControllerTestDB(t)
	withAlipayEnabled(t, true)

	form := validAlipayNotificationForm("alipay_notify_missing", "100.00")

	mock := &service.MockAlipayService{
		VerifySignFunc: func(_ context.Context, _ url.Values) error { return nil },
		DecodeNotificationFunc: func(_ context.Context, vals url.Values) (*alipay.Notification, error) {
			return buildDecodedNotification(vals), nil
		},
	}
	withAlipayService(t, mock)

	recorder := postAlipayNotify(t, form)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "failure", recorder.Body.String())
}
