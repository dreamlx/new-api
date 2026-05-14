package service

import (
	"context"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
)

type MockWechatPayService struct {
	NativePrepayFunc           func(ctx context.Context, outTradeNo string, description string, amountCents int64, notifyURL string) (string, error)
	QueryOrderByOutTradeNoFunc func(ctx context.Context, outTradeNo string) (*payments.Transaction, error)
	CloseOrderFunc             func(ctx context.Context, outTradeNo string) error
	DecryptNotificationFunc    func(ctx context.Context, request *http.Request) (*NotificationResult, error)
	RefundFunc                 func(ctx context.Context, outTradeNo string, outRefundNo string, totalCents int64, refundCents int64, reason string) error
}

func (m *MockWechatPayService) NativePrepay(ctx context.Context, outTradeNo string, description string, amountCents int64, notifyURL string) (string, error) {
	if m.NativePrepayFunc != nil {
		return m.NativePrepayFunc(ctx, outTradeNo, description, amountCents, notifyURL)
	}
	return "weixin://wxpay/bizpayurl?mock=1", nil
}

func (m *MockWechatPayService) QueryOrderByOutTradeNo(ctx context.Context, outTradeNo string) (*payments.Transaction, error) {
	if m.QueryOrderByOutTradeNoFunc != nil {
		return m.QueryOrderByOutTradeNoFunc(ctx, outTradeNo)
	}
	return &payments.Transaction{}, nil
}

func (m *MockWechatPayService) CloseOrder(ctx context.Context, outTradeNo string) error {
	if m.CloseOrderFunc != nil {
		return m.CloseOrderFunc(ctx, outTradeNo)
	}
	return nil
}

func (m *MockWechatPayService) DecryptNotification(ctx context.Context, request *http.Request) (*NotificationResult, error) {
	if m.DecryptNotificationFunc != nil {
		return m.DecryptNotificationFunc(ctx, request)
	}
	return &NotificationResult{}, nil
}

func (m *MockWechatPayService) Refund(ctx context.Context, outTradeNo string, outRefundNo string, totalCents int64, refundCents int64, reason string) error {
	if m.RefundFunc != nil {
		return m.RefundFunc(ctx, outTradeNo, outRefundNo, totalCents, refundCents, reason)
	}
	return nil
}

// Compile-time interface checks
var (
	_ WechatPayService = (*MockWechatPayService)(nil)
	_ WechatPayService = (*RealWechatPayService)(nil)
)

// NewMockNotificationResult creates a NotificationResult with typical fields filled.
func NewMockNotificationResult(outTradeNo, transactionId, tradeState string, amountTotal int64) *NotificationResult {
	return &NotificationResult{
		OutTradeNo:    outTradeNo,
		TransactionId: transactionId,
		TradeState:    tradeState,
		AmountTotal:   amountTotal,
		PaidAt:        common.GetTimestamp(),
	}
}
