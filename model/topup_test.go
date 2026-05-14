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
	common.RedisEnabled = false
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

	// Raw INSERT omitting refund_status — DB-level default must apply
	require.NoError(t, db.Exec(
		"INSERT INTO top_ups (user_id, amount, money, trade_no) VALUES (?, ?, ?, ?)",
		2, 50, 1.0, "default_test",
	).Error)

	var loaded TopUp
	require.NoError(t, db.Where("trade_no = ?", "default_test").First(&loaded).Error)
	require.Equal(t, "", loaded.RefundStatus)

	// GORM silently scans SQL NULL into Go's "" — so the Go-level read above
	// would pass even if the DEFAULT '' clause were missing. Verify at the
	// SQL level that the stored value is actually '' (not NULL), which proves
	// the DEFAULT '' clause from the GORM tag is in effect.
	var isNull int
	require.NoError(t, db.Raw(
		"SELECT refund_status IS NULL FROM top_ups WHERE trade_no = ?", "default_test",
	).Scan(&isNull).Error)
	require.Equal(t, 0, isNull, "refund_status must default to '' (not NULL); DEFAULT '' clause missing?")

	// Explicitly inserting NULL must fail — proves the NOT NULL clause is in effect.
	err := db.Exec(
		"INSERT INTO top_ups (user_id, amount, money, trade_no, refund_status) VALUES (?, ?, ?, ?, NULL)",
		3, 50, 1.0, "null_test",
	).Error
	require.Error(t, err, "inserting NULL into refund_status must fail; NOT NULL clause missing?")
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

func TestTopUpCallbackRawHiddenInSlice(t *testing.T) {
	topups := []*TopUp{
		{TradeNo: "slice_test_1", CallbackRaw: "secret_one"},
		{TradeNo: "slice_test_2", CallbackRaw: "secret_two"},
	}
	jsonBytes, err := common.Marshal(topups)
	require.NoError(t, err)
	require.NotContains(t, string(jsonBytes), "secret_one")
	require.NotContains(t, string(jsonBytes), "secret_two")
	require.NotContains(t, string(jsonBytes), "callback_raw")
}
