/**
此文件为旧版支付设置文件，如需增加新的参数、变量等，请在 payment_setting.go 中添加
This file is the old version of the payment settings file. If you need to add new parameters, variables, etc., please add them in payment_setting.go
*/

package operation_setting

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
)

var PayAddress = ""
var CustomCallbackAddress = ""
var EpayId = ""
var EpayKey = ""
var Price = 7.3
var MinTopUp = 1
var USDExchangeRate = 7.3

var PayMethods = []map[string]string{
	{
		"name":  "支付宝",
		"color": "rgba(var(--semi-blue-5), 1)",
		"type":  "alipay",
	},
	{
		"name":  "微信",
		"color": "rgba(var(--semi-green-5), 1)",
		"type":  "wxpay",
	},
	{
		"name":      "自定义1",
		"color":     "black",
		"type":      "custom1",
		"min_topup": "50",
	},
}

func UpdatePayMethodsByJsonString(jsonString string) error {
	PayMethods = make([]map[string]string, 0)
	return common.Unmarshal([]byte(jsonString), &PayMethods)
}

func PayMethods2JsonString() string {
	jsonBytes, err := common.Marshal(PayMethods)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

// isPayMethodConfigured 检查指定的动态支付方式是否已配置
func isPayMethodConfigured(method string) bool {
	switch method {
	case "stripe":
		return setting.StripeApiSecret != "" && setting.StripeWebhookSecret != "" && setting.StripePriceId != ""
	case "paypal":
		return setting.PayPalClientId != "" && setting.PayPalClientSecret != ""
	case "waffo":
		return setting.WaffoEnabled &&
			((!setting.WaffoSandbox &&
				setting.WaffoApiKey != "" &&
				setting.WaffoPrivateKey != "" &&
				setting.WaffoPublicCert != "") ||
				(setting.WaffoSandbox &&
					setting.WaffoSandboxApiKey != "" &&
					setting.WaffoSandboxPrivateKey != "" &&
					setting.WaffoSandboxPublicCert != ""))
	default:
		return false
	}
}

func ContainsPayMethod(method string) bool {
	// Check static pay methods
	for _, payMethod := range PayMethods {
		if payMethod["type"] == method {
			return true
		}
	}
	// Check dynamic pay methods
	return isPayMethodConfigured(method)
}
