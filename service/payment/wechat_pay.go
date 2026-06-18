package payment

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	nativeSvc "github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
)

type WechatPayService interface {
	NativePrepay(ctx context.Context, outTradeNo string, description string, amountCents int64, notifyURL string, timeExpire time.Time) (codeURL string, err error)
	QueryOrderByOutTradeNo(ctx context.Context, outTradeNo string) (*payments.Transaction, error)
	CloseOrder(ctx context.Context, outTradeNo string) error
	DecryptNotification(ctx context.Context, request *http.Request) (*NotificationResult, error)
}

type NotificationResult struct {
	OutTradeNo    string
	TransactionId string
	TradeState    string
	AmountTotal   int64
	PaidAt        int64
	Raw           string
}

type RealWechatPayService struct {
	client   *core.Client
	mchId    string
	appId    string
	apiV3Key string
	verifier auth.Verifier
}

func NewRealWechatPayService(client *core.Client, mchId string, appId string, apiV3Key string, verifier auth.Verifier) *RealWechatPayService {
	return &RealWechatPayService{client: client, mchId: mchId, appId: appId, apiV3Key: apiV3Key, verifier: verifier}
}

func (s *RealWechatPayService) NativePrepay(ctx context.Context, outTradeNo string, description string, amountCents int64, notifyURL string, timeExpire time.Time) (string, error) {
	svc := nativeSvc.NativeApiService{Client: s.client}
	total := amountCents
	req := nativeSvc.PrepayRequest{
		Appid:       core.String(s.appId),
		Mchid:       core.String(s.mchId),
		Description: core.String(description),
		OutTradeNo:  core.String(outTradeNo),
		NotifyUrl:   core.String(notifyURL),
		Amount: &nativeSvc.Amount{
			Total:    &total,
			Currency: core.String("CNY"),
		},
	}
	// Align WeChat-side TTL with our local ExpireTime when caller provides it,
	// so a payment arriving after our local order has been swept closed is
	// also rejected upstream — instead of letting WeChat keep accepting it for
	// its default 2-hour window.
	if !timeExpire.IsZero() {
		t := timeExpire
		req.TimeExpire = &t
	}
	resp, _, err := svc.Prepay(ctx, req)
	if err != nil {
		return "", fmt.Errorf("wechat native prepay failed: %w", err)
	}
	if resp == nil || resp.CodeUrl == nil {
		return "", fmt.Errorf("wechat native prepay: empty code_url in response")
	}
	return *resp.CodeUrl, nil
}

func (s *RealWechatPayService) QueryOrderByOutTradeNo(ctx context.Context, outTradeNo string) (*payments.Transaction, error) {
	svc := nativeSvc.NativeApiService{Client: s.client}
	req := nativeSvc.QueryOrderByOutTradeNoRequest{OutTradeNo: core.String(outTradeNo), Mchid: core.String(s.mchId)}
	resp, _, err := svc.QueryOrderByOutTradeNo(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("wechat query order failed: %w", err)
	}
	return resp, nil
}

func (s *RealWechatPayService) CloseOrder(ctx context.Context, outTradeNo string) error {
	svc := nativeSvc.NativeApiService{Client: s.client}
	req := nativeSvc.CloseOrderRequest{OutTradeNo: core.String(outTradeNo), Mchid: core.String(s.mchId)}
	_, err := svc.CloseOrder(ctx, req)
	if err != nil {
		return fmt.Errorf("wechat close order failed: %w", err)
	}
	return nil
}

func (s *RealWechatPayService) DecryptNotification(ctx context.Context, request *http.Request) (*NotificationResult, error) {
	handler, err := notify.NewRSANotifyHandler(s.apiV3Key, s.verifier)
	if err != nil {
		return nil, fmt.Errorf("wechat notify handler init failed: %w", err)
	}
	var txn payments.Transaction
	_, err = handler.ParseNotifyRequest(ctx, request, &txn)
	if err != nil {
		return nil, fmt.Errorf("wechat decrypt notification failed: %w", err)
	}
	result := &NotificationResult{}
	if txn.OutTradeNo != nil {
		result.OutTradeNo = *txn.OutTradeNo
	}
	if txn.TransactionId != nil {
		result.TransactionId = *txn.TransactionId
	}
	if txn.TradeState != nil {
		result.TradeState = *txn.TradeState
	}
	if txn.Amount != nil && txn.Amount.Total != nil {
		result.AmountTotal = *txn.Amount.Total
	}
	if txn.SuccessTime != nil {
		if t, parseErr := time.Parse(time.RFC3339, *txn.SuccessTime); parseErr == nil {
			result.PaidAt = t.Unix()
		}
	}
	raw, err := common.Marshal(txn)
	if err == nil {
		result.Raw = string(raw)
	}
	return result, nil
}
