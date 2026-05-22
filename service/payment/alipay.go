package payment

import (
	"context"
	"fmt"
	"net/url"

	"github.com/smartwalle/alipay/v3"
)

// AlipayService isolates the Alipay SDK behind an interface for testability
// and future SDK replacement. Controllers should depend on this interface,
// not on the SDK types directly.
type AlipayService interface {
	TradePagePay(outTradeNo string, subject string, totalAmount string, notifyURL string, returnURL string) (payURL string, err error)
	TradeQuery(ctx context.Context, outTradeNo string) (*alipay.TradeQueryRsp, error)
	TradeClose(ctx context.Context, outTradeNo string) (*alipay.TradeCloseRsp, error)
	VerifySign(ctx context.Context, values url.Values) error
	DecodeNotification(ctx context.Context, values url.Values) (*alipay.Notification, error)
}

// RealAlipayService wraps a smartwalle/alipay Client and implements AlipayService.
type RealAlipayService struct {
	client *alipay.Client
}

func NewRealAlipayService(client *alipay.Client) *RealAlipayService {
	return &RealAlipayService{client: client}
}

func (s *RealAlipayService) TradePagePay(outTradeNo string, subject string, totalAmount string, notifyURL string, returnURL string) (string, error) {
	p := alipay.TradePagePay{
		Trade: alipay.Trade{
			OutTradeNo:  outTradeNo,
			Subject:     subject,
			TotalAmount: totalAmount,
			ProductCode: "FAST_INSTANT_TRADE_PAY",
			NotifyURL:   notifyURL,
			ReturnURL:   returnURL,
		},
	}

	result, err := s.client.TradePagePay(p)
	if err != nil {
		return "", fmt.Errorf("alipay TradePagePay failed: %w", err)
	}
	return result.String(), nil
}

func (s *RealAlipayService) TradeQuery(ctx context.Context, outTradeNo string) (*alipay.TradeQueryRsp, error) {
	p := alipay.TradeQuery{OutTradeNo: outTradeNo}
	rsp, err := s.client.TradeQuery(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("alipay TradeQuery failed: %w", err)
	}
	if rsp.IsSuccess() == false {
		return rsp, fmt.Errorf("alipay TradeQuery returned error: code=%s msg=%s sub_code=%s sub_msg=%s",
			rsp.Code, rsp.Msg, rsp.SubCode, rsp.SubMsg)
	}
	return rsp, nil
}

func (s *RealAlipayService) TradeClose(ctx context.Context, outTradeNo string) (*alipay.TradeCloseRsp, error) {
	p := alipay.TradeClose{OutTradeNo: outTradeNo}
	rsp, err := s.client.TradeClose(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("alipay TradeClose failed: %w", err)
	}
	if rsp.IsSuccess() == false {
		return rsp, fmt.Errorf("alipay TradeClose returned error: code=%s msg=%s sub_code=%s sub_msg=%s",
			rsp.Code, rsp.Msg, rsp.SubCode, rsp.SubMsg)
	}
	return rsp, nil
}

func (s *RealAlipayService) VerifySign(ctx context.Context, values url.Values) error {
	err := s.client.VerifySign(ctx, values)
	if err != nil {
		return fmt.Errorf("alipay VerifySign failed: %w", err)
	}
	return nil
}

func (s *RealAlipayService) DecodeNotification(ctx context.Context, values url.Values) (*alipay.Notification, error) {
	notification, err := s.client.DecodeNotification(ctx, values)
	if err != nil {
		return nil, fmt.Errorf("alipay DecodeNotification failed: %w", err)
	}
	return notification, nil
}
