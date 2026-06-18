package controller

import (
	"context"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/smartwalle/alipay/v3"
	"github.com/stretchr/testify/require"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
)

// stringPtr helper for wechatpay payments.Transaction pointer fields.
func stringPtr(s string) *string { return &s }
func int64Ptr(v int64) *int64    { return &v }

// withTopUpStatusUserAndOrder seeds a user (id=1) plus one pending topup
// for that user, with the given create_time and payment_method.
func withTopUpStatusUserAndOrder(t *testing.T, paymentMethod string, createTime int64) (*model.TopUp, *model.User) {
	t.Helper()
	db := setupTopUpStatusTestDB(t)
	user := seedTopUpStatusUser(t, db, 1, "active_query_user")
	topUp := seedTopUpRow(t, db, user.Id, "USR1NO_active_q", paymentMethod, common.TopUpStatusPending, createTime)
	return topUp, user
}

func TestGetTopUpStatusActiveQueryAlipaySuccess(t *testing.T) {
	staleCreateTime := time.Now().Unix() - 60
	topUp, user := withTopUpStatusUserAndOrder(t, PaymentMethodAlipay, staleCreateTime)

	var queryCalls int32
	mock := &service.MockAlipayService{
		TradeQueryFunc: func(_ context.Context, outTradeNo string) (*alipay.TradeQueryRsp, error) {
			atomic.AddInt32(&queryCalls, 1)
			require.Equal(t, topUp.TradeNo, outTradeNo)
			return &alipay.TradeQueryRsp{
				TradeNo:     "alipay_tx_active",
				OutTradeNo:  outTradeNo,
				TradeStatus: alipay.TradeStatusSuccess,
				TotalAmount: "14.50",
				SendPayDate: "2026-05-14 12:00:01",
			}, nil
		},
	}
	withAlipayService(t, mock)

	recorder := callGetTopUpStatus(t, user.Id, topUp.TradeNo)

	require.Equal(t, 200, recorder.Code)
	body := recorder.Body.String()
	require.Contains(t, body, `"status":"success"`)
	require.Equal(t, int32(1), atomic.LoadInt32(&queryCalls))

	var loaded model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", topUp.TradeNo).First(&loaded).Error)
	require.Equal(t, common.TopUpStatusSuccess, loaded.Status)
	require.Equal(t, "alipay_tx_active", loaded.ProviderTxId)

	var u model.User
	require.NoError(t, model.DB.First(&u, user.Id).Error)
	expectedQuota := 100 + int(float64(topUp.Amount)*common.QuotaPerUnit)
	require.Equal(t, expectedQuota, u.Quota)

	var logCount int64
	require.NoError(t, model.DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", user.Id, model.LogTypeTopup).Count(&logCount).Error)
	require.Equal(t, int64(1), logCount)
}

func TestGetTopUpStatusActiveQueryWxpaySuccess(t *testing.T) {
	staleCreateTime := time.Now().Unix() - 60
	topUp, user := withTopUpStatusUserAndOrder(t, PaymentMethodWxpay, staleCreateTime)

	var queryCalls int32
	mock := &service.MockWechatPayService{
		QueryOrderByOutTradeNoFunc: func(_ context.Context, outTradeNo string) (*payments.Transaction, error) {
			atomic.AddInt32(&queryCalls, 1)
			require.Equal(t, topUp.TradeNo, outTradeNo)
			return &payments.Transaction{
				OutTradeNo:    stringPtr(outTradeNo),
				TransactionId: stringPtr("wx_tx_active"),
				TradeState:    stringPtr("SUCCESS"),
				SuccessTime:   stringPtr("2026-05-14T12:00:01+08:00"),
				Amount: &payments.TransactionAmount{
					Total: int64Ptr(1450),
				},
			}, nil
		},
	}
	withWxpayService(t, mock)

	recorder := callGetTopUpStatus(t, user.Id, topUp.TradeNo)

	require.Equal(t, 200, recorder.Code)
	body := recorder.Body.String()
	require.Contains(t, body, `"status":"success"`)
	require.Equal(t, int32(1), atomic.LoadInt32(&queryCalls))

	var loaded model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", topUp.TradeNo).First(&loaded).Error)
	require.Equal(t, common.TopUpStatusSuccess, loaded.Status)
	require.Equal(t, "wx_tx_active", loaded.ProviderTxId)

	var u model.User
	require.NoError(t, model.DB.First(&u, user.Id).Error)
	expectedQuota := 100 + int(float64(topUp.Amount)*common.QuotaPerUnit)
	require.Equal(t, expectedQuota, u.Quota)
}

func TestGetTopUpStatusActiveQueryAlipayStillPending(t *testing.T) {
	staleCreateTime := time.Now().Unix() - 60
	topUp, user := withTopUpStatusUserAndOrder(t, PaymentMethodAlipay, staleCreateTime)

	mock := &service.MockAlipayService{
		TradeQueryFunc: func(_ context.Context, outTradeNo string) (*alipay.TradeQueryRsp, error) {
			return &alipay.TradeQueryRsp{
				OutTradeNo:  outTradeNo,
				TradeStatus: alipay.TradeStatusWaitBuyerPay,
				TotalAmount: "14.50",
			}, nil
		},
	}
	withAlipayService(t, mock)

	recorder := callGetTopUpStatus(t, user.Id, topUp.TradeNo)
	require.Equal(t, 200, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"status":"pending"`)

	var u model.User
	require.NoError(t, model.DB.First(&u, user.Id).Error)
	require.Equal(t, 100, u.Quota, "no quota granted while pending")
}

func TestGetTopUpStatusActiveQueryFreshOrderSkipsSDK(t *testing.T) {
	freshCreateTime := time.Now().Unix() - 1 // < 5s threshold
	topUp, user := withTopUpStatusUserAndOrder(t, PaymentMethodAlipay, freshCreateTime)

	var queryCalls int32
	mock := &service.MockAlipayService{
		TradeQueryFunc: func(_ context.Context, _ string) (*alipay.TradeQueryRsp, error) {
			atomic.AddInt32(&queryCalls, 1)
			return &alipay.TradeQueryRsp{TradeStatus: alipay.TradeStatusSuccess, TotalAmount: "14.50"}, nil
		},
	}
	withAlipayService(t, mock)

	recorder := callGetTopUpStatus(t, user.Id, topUp.TradeNo)
	require.Equal(t, 200, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"status":"pending"`)
	require.Equal(t, int32(0), atomic.LoadInt32(&queryCalls), "fresh order must not trigger active query")
}

func TestGetTopUpStatusActiveQueryAmountMismatchSetsAnomaly(t *testing.T) {
	staleCreateTime := time.Now().Unix() - 60
	topUp, user := withTopUpStatusUserAndOrder(t, PaymentMethodAlipay, staleCreateTime)

	mock := &service.MockAlipayService{
		TradeQueryFunc: func(_ context.Context, outTradeNo string) (*alipay.TradeQueryRsp, error) {
			return &alipay.TradeQueryRsp{
				OutTradeNo:  outTradeNo,
				TradeNo:     "alipay_tx_mismatch",
				TradeStatus: alipay.TradeStatusSuccess,
				TotalAmount: "1.00", // expected 14.50
			}, nil
		},
	}
	withAlipayService(t, mock)

	recorder := callGetTopUpStatus(t, user.Id, topUp.TradeNo)
	require.Equal(t, 200, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"status":"pending"`,
		"anomaly is not surfaced to the user directly")

	var loaded model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", topUp.TradeNo).First(&loaded).Error)
	require.Equal(t, common.TopUpStatusAnomaly, loaded.Status)
	require.Contains(t, loaded.CallbackRaw, "mismatch")

	var u model.User
	require.NoError(t, model.DB.First(&u, user.Id).Error)
	require.Equal(t, 100, u.Quota, "no quota granted on amount mismatch")
}

func TestGetTopUpStatusActiveQueryAlreadyCompletedByConcurrentNotify(t *testing.T) {
	staleCreateTime := time.Now().Unix() - 60
	topUp, user := withTopUpStatusUserAndOrder(t, PaymentMethodAlipay, staleCreateTime)

	// Simulate a concurrent notify having already completed the order.
	rows, err := model.CompleteTopUpByCondition(model.DB, topUp.TradeNo, "alipay_tx_already", time.Now().Unix(), 0)
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)
	// Mimic the notify path granting quota too:
	require.NoError(t, model.IncreaseUserQuota(user.Id, int(float64(topUp.Amount)*common.QuotaPerUnit), true))

	mock := &service.MockAlipayService{
		TradeQueryFunc: func(_ context.Context, outTradeNo string) (*alipay.TradeQueryRsp, error) {
			return &alipay.TradeQueryRsp{
				OutTradeNo:  outTradeNo,
				TradeNo:     "alipay_tx_already",
				TradeStatus: alipay.TradeStatusSuccess,
				TotalAmount: "14.50",
			}, nil
		},
	}
	withAlipayService(t, mock)

	recorder := callGetTopUpStatus(t, user.Id, topUp.TradeNo)
	require.Equal(t, 200, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"status":"success"`)

	var u model.User
	require.NoError(t, model.DB.First(&u, user.Id).Error)
	expectedQuota := 100 + int(float64(topUp.Amount)*common.QuotaPerUnit)
	require.Equal(t, expectedQuota, u.Quota, "quota must not be double-granted")
}

// Confirm that the active query path is silently a no-op for unsupported
// payment methods (e.g. stripe).
func TestGetTopUpStatusActiveQueryUnsupportedMethodNoOp(t *testing.T) {
	staleCreateTime := time.Now().Unix() - 60
	topUp, user := withTopUpStatusUserAndOrder(t, "stripe", staleCreateTime)

	// Inject mocks that would FAIL the test if called (no service should be invoked).
	withAlipayService(t, &service.MockAlipayService{
		TradeQueryFunc: func(context.Context, string) (*alipay.TradeQueryRsp, error) {
			t.Fatal("alipay service must not be queried for stripe orders")
			return nil, nil
		},
	})
	withWxpayService(t, &service.MockWechatPayService{
		QueryOrderByOutTradeNoFunc: func(context.Context, string) (*payments.Transaction, error) {
			t.Fatal("wechat service must not be queried for stripe orders")
			return nil, nil
		},
	})

	recorder := callGetTopUpStatus(t, user.Id, topUp.TradeNo)
	require.Equal(t, 200, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"status":"pending"`)
}

// Silence unused warnings if any helper is not currently used by every test.
var _ = url.Values{}
