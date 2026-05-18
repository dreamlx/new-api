package setting

import (
	"context"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"
)

var (
	WxpayEnabled     = false
	WxpayMchId       = ""
	WxpayMchSerialNo = ""
	WxpayApiV3Key    = ""
	WxpayPrivateKey  = "" // PEM content
	WxpayPublicKeyId = ""
	WxpayPublicKey   = "" // PEM content
	WxpayAppId       = ""
	WxpayNotifyURL   = ""
	WxpayMinTopUp    = 1
)

var (
	wxpayClient     *core.Client
	wxpayVerifier   auth.Verifier
	wxpayClientOnce sync.Once
	wxpayClientMu   sync.Mutex
)

func missingWechatPayConfigFields() []string {
	missing := make([]string, 0, 5)
	if WxpayMchId == "" {
		missing = append(missing, "WxpayMchId")
	}
	if WxpayApiV3Key == "" {
		missing = append(missing, "WxpayApiV3Key")
	}
	if WxpayPrivateKey == "" {
		missing = append(missing, "WxpayPrivateKey")
	}
	if WxpayPublicKeyId == "" {
		missing = append(missing, "WxpayPublicKeyId")
	}
	if WxpayPublicKey == "" {
		missing = append(missing, "WxpayPublicKey")
	}
	return missing
}

// GetWechatPayClient returns a cached WeChat Pay client (initialized once).
// Returns nil if WeChat Pay is not configured.
//
// The whole body holds wxpayClientMu so that ResetWechatPayClient's pointer
// reassignment cannot race with a concurrent caller's read. sync.Once still
// guarantees one-time initialisation per epoch.
func GetWechatPayClient() *core.Client {
	wxpayClientMu.Lock()
	defer wxpayClientMu.Unlock()
	wxpayClientOnce.Do(func() {
		if missing := missingWechatPayConfigFields(); len(missing) > 0 {
			common.SysError("WeChat Pay client not configured, missing: " + strings.Join(missing, ", "))
			return
		}
		privateKey, err := utils.LoadPrivateKey(WxpayPrivateKey)
		if err != nil {
			common.SysError("failed to load WeChat Pay private key: " + err.Error())
			return
		}
		publicKey, err := utils.LoadPublicKey(WxpayPublicKey)
		if err != nil {
			common.SysError("failed to load WeChat Pay public key: " + err.Error())
			return
		}

		ctx := context.Background()
		client, err := core.NewClient(
			ctx,
			option.WithWechatPayPublicKeyAuthCipher(
				WxpayMchId,
				WxpayMchSerialNo,
				privateKey,
				WxpayPublicKeyId,
				publicKey,
			),
		)
		if err != nil {
			common.SysError("failed to create WeChat Pay client: " + err.Error())
			return
		}
		wxpayClient = client
		wxpayVerifier = verifiers.NewSHA256WithRSAPubkeyVerifier(WxpayPublicKeyId, *publicKey)
	})
	return wxpayClient
}

// ResetWechatPayClient clears the cached client (call after config update).
func ResetWechatPayClient() {
	wxpayClientMu.Lock()
	defer wxpayClientMu.Unlock()
	wxpayClient = nil
	wxpayVerifier = nil
	wxpayClientOnce = sync.Once{}
}

// GetWechatPayVerifier returns a verifier backed by the WeChat Pay public key.
// Required by notify.NewRSANotifyHandler to verify WeChat-Pay-Signature.
// Returns nil when the client has not been initialised.
func GetWechatPayVerifier() auth.Verifier {
	if GetWechatPayClient() == nil {
		return nil
	}
	wxpayClientMu.Lock()
	defer wxpayClientMu.Unlock()
	return wxpayVerifier
}
