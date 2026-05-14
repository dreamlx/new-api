package service

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

// WechatPayService isolates the WeChat Pay SDK for testability.
// Controllers depend on this interface so the SDK can be mocked or replaced
// without touching the controller layer.
type WechatPayService interface {
	// NativePrepay creates a Native payment order and returns the code_url for QR generation.
	NativePrepay(ctx context.Context, outTradeNo string, description string, amountCents int64, notifyURL string) (codeURL string, err error)

	// QueryOrderByOutTradeNo queries an order by the merchant order number.
	// Returns a Transaction with trade state or an error.
	QueryOrderByOutTradeNo(ctx context.Context, outTradeNo string) (*payments.Transaction, error)

	// CloseOrder closes an unpaid order so it cannot be paid later.
	CloseOrder(ctx context.Context, outTradeNo string) error

	// DecryptNotification decrypts and verifies a WeChat Pay payment notification.
	// The caller passes the raw *http.Request; the SDK handler verifies the RSA
	// signature and decrypts the AES-256-GCM ciphertext.
	// Returns a NotificationResult with the essential payment fields.
	DecryptNotification(ctx context.Context, request *http.Request) (*NotificationResult, error)

	// Refund submits a refund request for a previously paid order.
	Refund(ctx context.Context, outTradeNo string, outRefundNo string, totalCents int64, refundCents int64, reason string) error
}

// NotificationResult represents a decrypted WeChat Pay payment notification,
// decoupled from the SDK's internal Transaction type.
type NotificationResult struct {
	OutTradeNo    string // merchant order number
	TransactionId string // WeChat Pay transaction ID
	TradeState    string // SUCCESS / NOTPAY / CLOSED etc.
	AmountTotal   int64  // order total in cents (Fen)
	PaidAt        int64  // payment success time as unix timestamp, 0 if not paid
	Raw           string // original decrypted JSON for audit/logging
}

// RealWechatPayService is the production implementation that wraps the
// wechatpay-go SDK. It is created by NewRealWechatPayService and should be
// used as a singleton per merchant configuration.
type RealWechatPayService struct {
	client    *core.Client
	mchId     string
	appId     string
	apiV3Key  string
	verifier  auth.Verifier
}

// NewRealWechatPayService creates a RealWechatPayService.
//   - client: an initialised wechatpay-go core.Client
//   - mchId:  the WeChat Pay merchant ID
//   - appId:  the WeChat App ID (public account or mini-program)
//   - apiV3Key: the APIv3 key used for AES-GCM notification decryption
//   - verifier: an auth.Verifier for notification RSA signature verification
func NewRealWechatPayService(client *core.Client, mchId string, appId string, apiV3Key string, verifier auth.Verifier) *RealWechatPayService {
	return &RealWechatPayService{
		client:   client,
		mchId:    mchId,
		appId:    appId,
		apiV3Key: apiV3Key,
		verifier: verifier,
	}
}

// NativePrepay creates a Native (QR-code) payment order via the WeChat Pay API.
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

// QueryOrderByOutTradeNo queries an order by the merchant order number.
func (s *RealWechatPayService) QueryOrderByOutTradeNo(ctx context.Context, outTradeNo string) (*payments.Transaction, error) {
	svc := nativeSvc.NativeApiService{Client: s.client}

	req := nativeSvc.QueryOrderByOutTradeNoRequest{
		OutTradeNo: core.String(outTradeNo),
		Mchid:      core.String(s.mchId),
	}

	resp, _, err := svc.QueryOrderByOutTradeNo(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("wechat query order failed: %w", err)
	}
	return resp, nil
}

// CloseOrder closes an unpaid order so it cannot be paid later.
func (s *RealWechatPayService) CloseOrder(ctx context.Context, outTradeNo string) error {
	svc := nativeSvc.NativeApiService{Client: s.client}

	req := nativeSvc.CloseOrderRequest{
		OutTradeNo: core.String(outTradeNo),
		Mchid:      core.String(s.mchId),
	}

	_, err := svc.CloseOrder(ctx, req)
	if err != nil {
		return fmt.Errorf("wechat close order failed: %w", err)
	}
	return nil
}

// DecryptNotification verifies the RSA signature and decrypts the AES-256-GCM
// ciphertext of a WeChat Pay payment notification. It uses the SDK's built-in
// notify.Handler for both operations.
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

	// Preserve the raw decrypted JSON for audit
	raw, err := common.Marshal(txn)
	if err == nil {
		result.Raw = string(raw)
	}

	return result, nil
}

// Refund submits a refund request via the WeChat Pay domestic refund API.
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
