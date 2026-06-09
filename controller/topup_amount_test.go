package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func requestAmountForTest(t *testing.T, userID int, body string) (int, string) {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", userID)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/amount", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestAmount(ctx)
	return recorder.Code, recorder.Body.String()
}

func TestRequestAmountUsesAlipayMinimumForDirectAlipay(t *testing.T) {
	db := setupAlipayControllerTestDB(t)
	user := seedAlipayUser(t, db)

	originalEpayMin := operation_setting.MinTopUp
	originalAlipayMin := setting.AlipayMinTopUp
	operation_setting.MinTopUp = 100
	setting.AlipayMinTopUp = 5
	t.Cleanup(func() {
		operation_setting.MinTopUp = originalEpayMin
		setting.AlipayMinTopUp = originalAlipayMin
	})

	code, body := requestAmountForTest(t, user.Id, `{"amount":10,"payment_method":"direct-alipay"}`)

	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, `"message":"success"`)
}

func TestRequestAmountUsesWxpayMinimumForDirectWxpay(t *testing.T) {
	db := setupWxpayControllerTestDB(t)
	user := seedWxpayUser(t, db)

	originalEpayMin := operation_setting.MinTopUp
	originalWxpayMin := setting.WxpayMinTopUp
	operation_setting.MinTopUp = 100
	setting.WxpayMinTopUp = 5
	t.Cleanup(func() {
		operation_setting.MinTopUp = originalEpayMin
		setting.WxpayMinTopUp = originalWxpayMin
	})

	code, body := requestAmountForTest(t, user.Id, `{"amount":10,"payment_method":"direct-wxpay"}`)

	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, `"message":"success"`)
}

func TestRequestAmountStillUsesEpayMinimumByDefault(t *testing.T) {
	db := setupAlipayControllerTestDB(t)
	user := seedAlipayUser(t, db)

	originalEpayMin := operation_setting.MinTopUp
	originalAlipayMin := setting.AlipayMinTopUp
	operation_setting.MinTopUp = 100
	setting.AlipayMinTopUp = 5
	t.Cleanup(func() {
		operation_setting.MinTopUp = originalEpayMin
		setting.AlipayMinTopUp = originalAlipayMin
	})

	code, body := requestAmountForTest(t, user.Id, `{"amount":10}`)

	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, `"message":"error"`)
	require.Contains(t, body, "充值数量不能小于 100")
}

func TestGetTopUpInfoDoesNotExposePayPalWithoutWebhookSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	confirmPaymentComplianceForTest(t)

	originalClientID := setting.PayPalClientId
	originalClientSecret := setting.PayPalClientSecret
	originalWebhookSecret := setting.PayPalWebhookSecret
	originalPayMethods := operation_setting.PayMethods
	setting.PayPalClientId = "client_id"
	setting.PayPalClientSecret = "client_secret"
	setting.PayPalWebhookSecret = ""
	operation_setting.PayMethods = nil
	t.Cleanup(func() {
		setting.PayPalClientId = originalClientID
		setting.PayPalClientSecret = originalClientSecret
		setting.PayPalWebhookSecret = originalWebhookSecret
		operation_setting.PayMethods = originalPayMethods
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/topup/info", nil)

	GetTopUpInfo(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, common.UnmarshalJsonStr(recorder.Body.String(), &resp))
	require.Equal(t, false, resp.Data["enable_paypal_topup"])
	require.Empty(t, resp.Data["pay_methods"])
}

func TestAdminPlatform_CreateWithDisabledStatus(t *testing.T) {
	router := setupAdminPlatformRouter(t)

	w := doRequest(router, "POST", "/api/admin/v2/platforms/", map[string]interface{}{
		"platform_id": "disabled_on_create",
		"status":      common.UserStatusDisabled,
	})
	require.Equal(t, 200, w.Code, w.Body.String())
	resp := parseResponse(t, w)
	data := resp["data"].(map[string]interface{})
	require.Equal(t, float64(common.UserStatusDisabled), data["status"])

	var stored model.Platform
	require.NoError(t, model.DB.Where("platform_id = ?", "disabled_on_create").First(&stored).Error)
	require.Equal(t, common.UserStatusDisabled, stored.Status)
}
