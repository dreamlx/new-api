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
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// withServerAddress sets system_setting.ServerAddress for the duration of the
// test (so we get a deterministic redirect target).
func withServerAddress(t *testing.T, addr string) {
	t.Helper()
	original := system_setting.ServerAddress
	system_setting.ServerAddress = addr
	t.Cleanup(func() { system_setting.ServerAddress = original })
}

func getAlipayReturn(t *testing.T, query url.Values) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	target := "/api/user/alipay/return"
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	AlipayReturn(ctx)
	return recorder
}

// seedAlipayCompletedTopUp inserts a TopUp already marked success — what the
// notify handler would have done before the browser bounced back.
func seedAlipayCompletedTopUp(t *testing.T, db *gorm.DB, tradeNo string) *model.TopUp {
	t.Helper()
	user := seedAlipayUser(t, db)
	topUp := &model.TopUp{
		UserId:         user.Id,
		Amount:         100,
		Money:          1.0,
		TradeNo:        tradeNo,
		PaymentMethod:  PaymentMethodAlipay,
		CreateTime:     1,
		Status:         common.TopUpStatusSuccess,
		PayAmountCents: 10000,
		Currency:       "CNY",
	}
	require.NoError(t, db.Create(topUp).Error)
	return topUp
}

func TestAlipayReturnSuccess(t *testing.T) {
	db := setupAlipayControllerTestDB(t)
	withAlipayEnabled(t, true)
	withServerAddress(t, "http://localhost:3000")
	seedAlipayCompletedTopUp(t, db, "alipay_return_ok")

	mock := &service.MockAlipayService{
		VerifySignFunc: func(_ context.Context, _ url.Values) error { return nil },
	}
	withAlipayService(t, mock)

	q := url.Values{}
	q.Set("out_trade_no", "alipay_return_ok")

	recorder := getAlipayReturn(t, q)

	require.Equal(t, http.StatusFound, recorder.Code)
	loc := recorder.Header().Get("Location")
	require.True(t, strings.HasPrefix(loc, "http://localhost:3000/console/topup?"),
		"unexpected redirect prefix: %s", loc)
	require.Contains(t, loc, "pay=success")
	require.Contains(t, loc, "trade_no=alipay_return_ok")
}

func TestAlipayReturnPending(t *testing.T) {
	db := setupAlipayControllerTestDB(t)
	withAlipayEnabled(t, true)
	withServerAddress(t, "http://localhost:3000")
	// Seed a pending TopUp — notify has not fired yet.
	seedAlipayPendingTopUp(t, db, "alipay_return_pending", 10000)

	mock := &service.MockAlipayService{
		VerifySignFunc: func(_ context.Context, _ url.Values) error { return nil },
	}
	withAlipayService(t, mock)

	q := url.Values{}
	q.Set("out_trade_no", "alipay_return_pending")

	recorder := getAlipayReturn(t, q)

	require.Equal(t, http.StatusFound, recorder.Code)
	loc := recorder.Header().Get("Location")
	require.Contains(t, loc, "pay=pending")
	require.Contains(t, loc, "trade_no=alipay_return_pending")

	// Quota MUST NOT have been granted by the return handler.
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	require.Equal(t, 100, user.Quota, "AlipayReturn must not grant quota")

	// Status must still be pending.
	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "alipay_return_pending").First(&loaded).Error)
	require.Equal(t, common.TopUpStatusPending, loaded.Status)
}

func TestAlipayReturnBadSignature(t *testing.T) {
	setupAlipayControllerTestDB(t)
	withAlipayEnabled(t, true)
	withServerAddress(t, "http://localhost:3000")

	mock := &service.MockAlipayService{
		VerifySignFunc: func(_ context.Context, _ url.Values) error {
			return errAlipayMockFailure
		},
	}
	withAlipayService(t, mock)

	q := url.Values{}
	q.Set("out_trade_no", "alipay_return_badsig")
	q.Set("trade_no", "alipay_tx_badsig")
	q.Set("total_amount", "100.00")

	recorder := getAlipayReturn(t, q)

	require.Equal(t, http.StatusFound, recorder.Code)
	loc := recorder.Header().Get("Location")
	require.Contains(t, loc, "pay=fail")
}

func TestAlipayReturnMissingOutTradeNo(t *testing.T) {
	setupAlipayControllerTestDB(t)
	withAlipayEnabled(t, true)
	withServerAddress(t, "http://localhost:3000")

	mock := &service.MockAlipayService{
		VerifySignFunc: func(_ context.Context, _ url.Values) error { return nil },
	}
	withAlipayService(t, mock)

	// Decision: missing out_trade_no -> 302 redirect to pay=fail (consistent
	// with the bad-signature path). We never want to expose a raw 400 to a
	// user bouncing back from Alipay; the console handles the fail state.
	recorder := getAlipayReturn(t, url.Values{})

	require.Equal(t, http.StatusFound, recorder.Code)
	loc := recorder.Header().Get("Location")
	require.Contains(t, loc, "pay=fail")
}

func TestAlipayReturnTopUpNotFound(t *testing.T) {
	setupAlipayControllerTestDB(t)
	withAlipayEnabled(t, true)
	withServerAddress(t, "http://localhost:3000")

	mock := &service.MockAlipayService{
		VerifySignFunc: func(_ context.Context, _ url.Values) error { return nil },
	}
	withAlipayService(t, mock)

	q := url.Values{}
	q.Set("out_trade_no", "alipay_return_orphan")

	recorder := getAlipayReturn(t, q)

	// Unknown order — treat as pending; notify may yet arrive (or operator
	// can investigate). Definitely do not claim success.
	require.Equal(t, http.StatusFound, recorder.Code)
	loc := recorder.Header().Get("Location")
	require.Contains(t, loc, "pay=pending")
}
