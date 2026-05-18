package setting

import (
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/smartwalle/alipay/v3"
)

var (
	AlipayEnabled    = false
	AlipayAppId      = ""
	AlipayPrivateKey = ""
	AlipayPublicKey  = "" // Alipay's public key (for signature verification)
	AlipaySellerId   = ""
	AlipayIsSandbox  = false
	AlipayMinTopUp   = 1
)

var (
	alipayClient     *alipay.Client
	alipayClientOnce sync.Once
	alipayClientMu   sync.Mutex
)

// GetAlipayClient returns a cached Alipay client (initialized once).
// Returns nil if Alipay is not configured.
//
// The whole body holds alipayClientMu so that ResetAlipayClient's pointer
// reassignment cannot race with a concurrent caller's read. sync.Once still
// guarantees one-time initialisation per epoch.
func GetAlipayClient() *alipay.Client {
	alipayClientMu.Lock()
	defer alipayClientMu.Unlock()
	alipayClientOnce.Do(func() {
		if AlipayAppId == "" || AlipayPrivateKey == "" {
			return
		}
		client, err := alipay.New(AlipayAppId, AlipayPrivateKey, !AlipayIsSandbox)
		if err != nil {
			// Log error but don't panic — config may be updated later
			common.SysError("failed to create Alipay client: " + err.Error())
			return
		}
		if AlipayPublicKey != "" {
			if err := client.LoadAliPayPublicKey(AlipayPublicKey); err != nil {
				common.SysError("failed to load Alipay public key: " + err.Error())
			}
		}
		alipayClient = client
	})
	return alipayClient
}

// ResetAlipayClient clears the cached client (call after config update).
func ResetAlipayClient() {
	alipayClientMu.Lock()
	defer alipayClientMu.Unlock()
	alipayClient = nil
	alipayClientOnce = sync.Once{}
}
