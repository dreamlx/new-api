package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	payment "github.com/QuantumNous/new-api/service/payment"
	"github.com/glebarez/sqlite"
	"github.com/smartwalle/alipay/v3"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupTopUpExpiryTest wires up an in-memory SQLite DB on model.DB, swaps
// the provider hooks for in-memory mocks, and restores everything on cleanup.
type expiryMocks struct {
	alipay *payment.MockAlipayService
	wechat *payment.MockWechatPayService
}

func setupTopUpExpiryTest(t *testing.T) (*gorm.DB, *expiryMocks) {
	t.Helper()
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	model.InitColumnsForTest()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}))

	origDB := model.DB
	model.DB = db

	mocks := &expiryMocks{
		alipay: &payment.MockAlipayService{},
		wechat: &payment.MockWechatPayService{},
	}
	restore := payment.SetExpiryProvidersForTest(
		func() (payment.AlipayService, error) { return mocks.alipay, nil },
		func() (payment.WechatPayService, error) { return mocks.wechat, nil },
	)

	t.Cleanup(func() {
		model.DB = origDB
		restore()
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	return db, mocks
}

func TestCloseExpiredPendingTopUps_HappyMixedRows(t *testing.T) {
	db, mocks := setupTopUpExpiryTest(t)
	now := time.Now().Unix()

	rows := []model.TopUp{
		{
			UserId: 1, Amount: 10, Money: 1, TradeNo: "expired_ali",
			PaymentMethod: "alipay", Status: common.TopUpStatusPending,
			ExpireTime: now - 60,
		},
		{
			UserId: 1, Amount: 10, Money: 1, TradeNo: "not_yet_expired",
			PaymentMethod: "alipay", Status: common.TopUpStatusPending,
			ExpireTime: now + 600,
		},
		{
			UserId: 1, Amount: 10, Money: 1, TradeNo: "expired_wx",
			PaymentMethod: "wxpay", Status: common.TopUpStatusPending,
			ExpireTime: now - 30,
		},
	}
	for i := range rows {
		require.NoError(t, db.Create(&rows[i]).Error)
	}

	var aliCalls []string
	var wxCalls []string
	mocks.alipay.TradeCloseFunc = func(ctx context.Context, outTradeNo string) (*alipay.TradeCloseRsp, error) {
		aliCalls = append(aliCalls, outTradeNo)
		return &alipay.TradeCloseRsp{}, nil
	}
	mocks.wechat.CloseOrderFunc = func(ctx context.Context, outTradeNo string) error {
		wxCalls = append(wxCalls, outTradeNo)
		return nil
	}

	ok, fail, err := CloseExpiredPendingTopUps(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, ok)
	require.Equal(t, 0, fail)
	require.ElementsMatch(t, []string{"expired_ali"}, aliCalls)
	require.ElementsMatch(t, []string{"expired_wx"}, wxCalls)

	// Verify post-state.
	statusOf := func(tradeNo string) string {
		var got model.TopUp
		require.NoError(t, db.Where("trade_no = ?", tradeNo).First(&got).Error)
		return got.Status
	}
	require.Equal(t, common.TopUpStatusExpired, statusOf("expired_ali"))
	require.Equal(t, common.TopUpStatusExpired, statusOf("expired_wx"))
	require.Equal(t, common.TopUpStatusPending, statusOf("not_yet_expired"))
}

func TestCloseExpiredPendingTopUps_SDKErrorStillClosesLocally(t *testing.T) {
	db, mocks := setupTopUpExpiryTest(t)
	now := time.Now().Unix()

	require.NoError(t, db.Create(&model.TopUp{
		UserId: 1, Amount: 10, Money: 1, TradeNo: "sdk_fails",
		PaymentMethod: "alipay", Status: common.TopUpStatusPending,
		ExpireTime: now - 60,
	}).Error)

	mocks.alipay.TradeCloseFunc = func(ctx context.Context, outTradeNo string) (*alipay.TradeCloseRsp, error) {
		return nil, errors.New("alipay 500")
	}

	ok, fail, err := CloseExpiredPendingTopUps(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, ok)
	require.Equal(t, 1, fail, "SDK failure increments closeFailed even when local close succeeds")

	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "sdk_fails").First(&loaded).Error)
	require.Equal(t, common.TopUpStatusExpired, loaded.Status,
		"row must be closed locally even when provider SDK errors — avoids the user paying a doomed order")
}

func TestCloseExpiredPendingTopUps_Idempotent(t *testing.T) {
	db, mocks := setupTopUpExpiryTest(t)
	now := time.Now().Unix()

	require.NoError(t, db.Create(&model.TopUp{
		UserId: 1, Amount: 10, Money: 1, TradeNo: "idem_ali",
		PaymentMethod: "alipay", Status: common.TopUpStatusPending,
		ExpireTime: now - 60,
	}).Error)

	callCount := 0
	mocks.alipay.TradeCloseFunc = func(ctx context.Context, outTradeNo string) (*alipay.TradeCloseRsp, error) {
		callCount++
		return &alipay.TradeCloseRsp{}, nil
	}

	ok1, fail1, err := CloseExpiredPendingTopUps(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, ok1)
	require.Equal(t, 0, fail1)

	ok2, fail2, err := CloseExpiredPendingTopUps(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, ok2, "second sweep finds no pending rows")
	require.Equal(t, 0, fail2)
	require.Equal(t, 1, callCount, "SDK Close not re-invoked on the already-expired row")
}

func TestCloseExpiredPendingTopUps_UnknownProviderSkipped(t *testing.T) {
	db, _ := setupTopUpExpiryTest(t)
	now := time.Now().Unix()

	require.NoError(t, db.Create(&model.TopUp{
		UserId: 1, Amount: 10, Money: 1, TradeNo: "epay_legacy",
		PaymentMethod: "epay", Status: common.TopUpStatusPending,
		ExpireTime: now - 60,
	}).Error)

	ok, fail, err := CloseExpiredPendingTopUps(context.Background())
	require.NoError(t, err)
	// Unknown provider: callProviderClose returns nil (no error), so the row
	// still gets the local UPDATE; ok==1. This is intentional — once we've
	// recorded an expire_time on the row, the order is dead either way.
	require.Equal(t, 1, ok)
	require.Equal(t, 0, fail)

	var loaded model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "epay_legacy").First(&loaded).Error)
	require.Equal(t, common.TopUpStatusExpired, loaded.Status)
}

// Smoke-test that exercising VerifySign through the mock works — present
// only to ensure expiryMocks alipay field is wired through the interface
// (compile-time guarantee, the function call is incidental).
func TestExpiryMocksImplementInterfaces(t *testing.T) {
	var _ payment.AlipayService = (*payment.MockAlipayService)(nil)
	var _ payment.WechatPayService = (*payment.MockWechatPayService)(nil)
	require.NoError(t, (&payment.MockAlipayService{}).VerifySign(context.Background(), url.Values{}))
}
