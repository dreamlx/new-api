package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTopUpTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	InitColumnsForTest()

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&TopUp{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestTopUpNewFields(t *testing.T) {
	db := setupTopUpTestDB(t)

	topUp := &TopUp{
		UserId:            1,
		Amount:            100,
		Money:             2.0,
		TradeNo:           "test_trade_no",
		PayAmountCents:    1450,
		Currency:          "CNY",
		QuotaGranted:      100,
		ExpireTime:        1234567890,
		ProviderTxId:      "alipay_tx_123",
		PaidAt:            1234567900,
		CallbackRaw:       `{"event":"test"}`,
		RefundStatus:      "refund_pending",
		RefundRequestTime: 1234567910,
		RefundTime:        1234567920,
		RefundReason:      "user request",
		RefundTradeNo:     "refund_123",
		RefundAdminId:     1,
		RefundedQuota:     100,
	}

	require.NoError(t, db.Create(topUp).Error)

	var loaded TopUp
	require.NoError(t, db.First(&loaded, topUp.Id).Error)
	require.Equal(t, int64(1450), loaded.PayAmountCents)
	require.Equal(t, "CNY", loaded.Currency)
	require.Equal(t, int64(100), loaded.QuotaGranted)
	require.Equal(t, int64(1234567890), loaded.ExpireTime)
	require.Equal(t, "alipay_tx_123", loaded.ProviderTxId)
	require.Equal(t, "refund_pending", loaded.RefundStatus)
	require.Equal(t, int64(100), loaded.RefundedQuota)
}

func TestTopUpRefundStatusDefaultsEmpty(t *testing.T) {
	db := setupTopUpTestDB(t)

	topUp := &TopUp{
		UserId:  2,
		Amount:  50,
		Money:   1.0,
		TradeNo: "default_test",
	}
	require.NoError(t, db.Create(topUp).Error)

	var loaded TopUp
	require.NoError(t, db.Where("trade_no = ?", "default_test").First(&loaded).Error)
	require.Equal(t, "", loaded.RefundStatus)
}

func TestTopUpCallbackRawHiddenFromJSON(t *testing.T) {
	topUp := &TopUp{
		TradeNo:     "json_test",
		CallbackRaw: "secret payload",
	}
	jsonBytes, err := common.Marshal(topUp)
	require.NoError(t, err)
	require.NotContains(t, string(jsonBytes), "secret payload")
	require.NotContains(t, string(jsonBytes), "callback_raw")
}
