package payment

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWechatPayServiceInterface(t *testing.T) {
	var _ WechatPayService = (*RealWechatPayService)(nil)
	var _ WechatPayService = (*MockWechatPayService)(nil)
	require.NotNil(t, NewRealWechatPayService)
}

func TestNotificationResultFields(t *testing.T) {
	r := NotificationResult{
		OutTradeNo:    "order123",
		TransactionId: "tx456",
		TradeState:    "SUCCESS",
		AmountTotal:   100,
		PaidAt:        1700000000,
		Raw:           `{"test":true}`,
	}
	require.Equal(t, "order123", r.OutTradeNo)
	require.Equal(t, "tx456", r.TransactionId)
	require.Equal(t, "SUCCESS", r.TradeState)
	require.Equal(t, int64(100), r.AmountTotal)
	require.Equal(t, int64(1700000000), r.PaidAt)
	require.Equal(t, `{"test":true}`, r.Raw)
}
