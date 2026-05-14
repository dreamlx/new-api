package setting

import (
	"context"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/downloader"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"
)

var (
	WxpayEnabled     = false
	WxpayMchId       = ""
	WxpayMchSerialNo = ""
	WxpayApiV3Key    = ""
	WxpayPrivateKey  = "" // PEM content
	WxpayAppId       = ""
	WxpayNotifyURL   = ""
	WxpayMinTopUp    = 1
)

var (
	wxpayClient     *core.Client
	wxpayClientOnce sync.Once
	wxpayClientMu   sync.Mutex
)

// GetWechatPayClient returns a cached WeChat Pay client (initialized once).
// Returns nil if WeChat Pay is not configured.
func GetWechatPayClient() *core.Client {
	wxpayClientOnce.Do(func() {
		if WxpayMchId == "" || WxpayApiV3Key == "" || WxpayPrivateKey == "" {
			return
		}
		// Load merchant private key from PEM string stored in DB
		privateKey, err := utils.LoadPrivateKey(WxpayPrivateKey)
		if err != nil {
			common.SysError("failed to load WeChat Pay private key: " + err.Error())
			return
		}

		ctx := context.Background()
		client, err := core.NewClient(
			ctx,
			option.WithWechatPayAutoAuthCipher(WxpayMchId, WxpayMchSerialNo, privateKey, WxpayApiV3Key),
		)
		if err != nil {
			common.SysError("failed to create WeChat Pay client: " + err.Error())
			return
		}
		wxpayClient = client
	})
	return wxpayClient
}

// ResetWechatPayClient clears the cached client (call after config update).
func ResetWechatPayClient() {
	wxpayClientMu.Lock()
	defer wxpayClientMu.Unlock()
	wxpayClient = nil
	wxpayClientOnce = sync.Once{}
}

// GetWechatPayVerifier returns a Verifier backed by the platform-certificate
// downloader that WithWechatPayAutoAuthCipher registers on first client init.
// Required by notify.NewRSANotifyHandler to verify WeChat-Pay-Signature.
// Returns nil when the client has not been initialised (no downloader registered).
func GetWechatPayVerifier() auth.Verifier {
	if GetWechatPayClient() == nil {
		return nil
	}
	visitor := downloader.MgrInstance().GetCertificateVisitor(WxpayMchId)
	if visitor == nil {
		return nil
	}
	return verifiers.NewSHA256WithRSAVerifier(visitor)
}
