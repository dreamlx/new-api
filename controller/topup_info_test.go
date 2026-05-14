package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestGetTopUpInfoIncludesAlipayFields verifies the response surface adds
// the symmetric Alipay fields the frontend needs to render the Alipay button.
func TestGetTopUpInfoIncludesAlipayFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalEnabled := setting.AlipayEnabled
	originalMin := setting.AlipayMinTopUp
	setting.AlipayEnabled = true
	setting.AlipayMinTopUp = 5
	t.Cleanup(func() {
		setting.AlipayEnabled = originalEnabled
		setting.AlipayMinTopUp = originalMin
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

	require.Equal(t, true, resp.Data["enable_alipay_topup"], "enable_alipay_topup must be true when AlipayEnabled=true")
	// JSON numbers unmarshal as float64 into interface{}.
	require.Equal(t, float64(5), resp.Data["alipay_min_topup"])
}

func TestGetTopUpInfoAlipayDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalEnabled := setting.AlipayEnabled
	setting.AlipayEnabled = false
	t.Cleanup(func() { setting.AlipayEnabled = originalEnabled })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/topup/info", nil)

	GetTopUpInfo(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, common.UnmarshalJsonStr(recorder.Body.String(), &resp))
	require.Equal(t, false, resp.Data["enable_alipay_topup"])
}
