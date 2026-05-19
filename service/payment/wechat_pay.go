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
	"github.com/wechatpay-apiv3/wechatpay-go/services/refunddomestic"
)

type WechatPayService interface {
	NativePrepay(ctx context.Context, outTradeNo string, description string, amountCents int64, notifyURL string) (codeURL string, err error)
	QueryOrderByOutTradeNo(ctx context.Context, outTradeNo string) (*payments.Transaction, error)
	CloseOrder(ctx context.Context, outTradeNo string) error
	DecryptNotification(ctx context.Context, request *http.Request) (*NotificationResult, error)
	DecryptRefundNotification(ctx context.Context, request *http.Request) (*RefundNotificationResult, error)
	Refund(ctx context.Context, outTradeNo string, outRefundNo string, totalCents int64, refundCents int64, reason string) error
}

type NotificationResult struct {
	OutTradeNo    string
	TransactionId string
	TradeState    string
	AmountTotal   int64
	PaidAt        int64
	Raw           string
}

type RefundNotificationResult struct {
	OutTradeNo   string
	OutRefundNo  string
	RefundId     string
	RefundStatus string
	RefundAmount int64
	SuccessTime  int64
	Raw          string
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

func (s *RealWechatPayService) NativePrepay(ctx context.Context, outTradeNo string, description string, amountCents int64, notifyURL string) (string, error) {
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

func (s *RealWechatPayService) DecryptRefundNotification(ctx context.Context, request *http.Request) (*RefundNotificationResult, error) {
	handler, err := notify.NewRSANotifyHandler(s.apiV3Key, s.verifier)
	if err != nil {
		return nil, fmt.Errorf("wechat refund notify handler init failed: %w", err)
	}
	var rfd refunddomestic.Refund
	_, err = handler.ParseNotifyRequest(ctx, request, &rfd)
	if err != nil {
		return nil, fmt.Errorf("wechat decrypt refund notification failed: %w", err)
	}
	result := &RefundNotificationResult{}
	if rfd.OutTradeNo != nil {
		result.OutTradeNo = *rfd.OutTradeNo
	}
	if rfd.OutRefundNo != nil {
		result.OutRefundNo = *rfd.OutRefundNo
	}
	if rfd.RefundId != nil {
		result.RefundId = *rfd.RefundId
	}
	if rfd.Status != nil {
		result.RefundStatus = string(*rfd.Status)
	}
	if rfd.Amount != nil && rfd.Amount.Refund != nil {
		result.RefundAmount = *rfd.Amount.Refund
	}
	if rfd.SuccessTime != nil {
		result.SuccessTime = rfd.SuccessTime.Unix()
	}
	if raw, err := common.Marshal(rfd); err == nil {
		result.Raw = string(raw)
	}
	return result, nil
}

func (s *RealWechatPayService) Refund(ctx context.Context, outTradeNo string, outRefundNo string, totalCents int64, refundCents int64, reason string) error {
	svc := refunddomestic.RefundsApiService{Client: s.client}
	req := refunddomestic.CreateRequest{
		OutTradeNo:  core.String(outTradeNo),
		OutRefundNo: core.String(outRefundNo),
		Reason:      core.String(reason),
		Amount: &refunddomestic.AmountReq{
			Refund:   core.Int64(refundCents),
			Total:    core.Int64(totalCents),
			Currency: core.String("CNY"),
		},
	}
	_, _, err := svc.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("wechat refund failed: %w", err)
	}
	return nil
}
