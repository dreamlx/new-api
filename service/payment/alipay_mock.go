package payment

import (
	"context"
	"fmt"
	"net/url"

	"github.com/smartwalle/alipay/v3"
)

type MockAlipayService struct {
	TradePagePayFunc       func(outTradeNo string, subject string, totalAmount string, notifyURL string, returnURL string) (string, error)
	TradeQueryFunc         func(ctx context.Context, outTradeNo string) (*alipay.TradeQueryRsp, error)
	TradeCloseFunc         func(ctx context.Context, outTradeNo string) (*alipay.TradeCloseRsp, error)
	VerifySignFunc         func(ctx context.Context, values url.Values) error
	DecodeNotificationFunc func(ctx context.Context, values url.Values) (*alipay.Notification, error)
}

func (m *MockAlipayService) TradePagePay(outTradeNo string, subject string, totalAmount string, notifyURL string, returnURL string) (string, error) {
	if m.TradePagePayFunc != nil {
		return m.TradePagePayFunc(outTradeNo, subject, totalAmount, notifyURL, returnURL)
	}
	return fmt.Sprintf("https://mock.alipay.com?out_trade_no=%s&total_amount=%s", outTradeNo, totalAmount), nil
}

func (m *MockAlipayService) TradeQuery(ctx context.Context, outTradeNo string) (*alipay.TradeQueryRsp, error) {
	if m.TradeQueryFunc != nil {
		return m.TradeQueryFunc(ctx, outTradeNo)
	}
	return &alipay.TradeQueryRsp{}, nil
}

func (m *MockAlipayService) TradeClose(ctx context.Context, outTradeNo string) (*alipay.TradeCloseRsp, error) {
	if m.TradeCloseFunc != nil {
		return m.TradeCloseFunc(ctx, outTradeNo)
	}
	return &alipay.TradeCloseRsp{}, nil
}

func (m *MockAlipayService) VerifySign(ctx context.Context, values url.Values) error {
	if m.VerifySignFunc != nil {
		return m.VerifySignFunc(ctx, values)
	}
	return nil
}

func (m *MockAlipayService) DecodeNotification(ctx context.Context, values url.Values) (*alipay.Notification, error) {
	if m.DecodeNotificationFunc != nil {
		return m.DecodeNotificationFunc(ctx, values)
	}
	return &alipay.Notification{}, nil
}
