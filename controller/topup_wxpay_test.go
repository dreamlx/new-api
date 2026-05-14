package controller

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var errWxpayMockFailure = errors.New("wxpay mock failure")

func setupWxpayControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	model.InitColumnsForTest()

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedWxpayUser(t *testing.T, db *gorm.DB) *model.User {
	t.Helper()
	user := &model.User{
		Id:       1,
		Username: "wxpay_user",
		Password: "password",
		Status:   common.UserStatusEnabled,
		Quota:    100,
		Group:    "default",
	}
	require.NoError(t, db.Create(user).Error)
	return user
}

func withWxpayEnabled(t *testing.T, enabled bool) {
	t.Helper()
	original := setting.WxpayEnabled
	originalAppId := setting.WxpayAppId
	originalMchId := setting.WxpayMchId
	setting.WxpayEnabled = enabled
	setting.WxpayAppId = "test_wx_app_id"
	setting.WxpayMchId = "test_wx_mch_id"
	t.Cleanup(func() {
		setting.WxpayEnabled = original
		setting.WxpayAppId = originalAppId
		setting.WxpayMchId = originalMchId
	})
}

func withWxpayService(t *testing.T, mock service.WechatPayService) {
	t.Helper()
	original := wechatPayServiceProvider
	wechatPayServiceProvider = func() (service.WechatPayService, error) {
		return mock, nil
	}
	t.Cleanup(func() { wechatPayServiceProvider = original })
}

func TestRequestWxpayHappyPath(t *testing.T) {
	db := setupWxpayControllerTestDB(t)
	user := seedWxpayUser(t, db)
	withWxpayEnabled(t, true)

	var captured struct {
		outTradeNo  string
		description string
		amountCents int64
		notifyURL   string
	}
	mock := &service.MockWechatPayService{
		NativePrepayFunc: func(_ context.Context, outTradeNo, description string, amountCents int64, notifyURL string) (string, error) {
			captured.outTradeNo = outTradeNo
			captured.description = description
			captured.amountCents = amountCents
			captured.notifyURL = notifyURL
			return "weixin://wxpay/bizpayurl?pr=" + outTradeNo, nil
		},
	}
	withWxpayService(t, mock)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", user.Id)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/wxpay/pay",
		bytes.NewBufferString(`{"amount":100}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestWxpay(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	require.Contains(t, body, "success")
	require.Contains(t, body, "code_url")
	require.Contains(t, body, "weixin://")
	require.NotEmpty(t, captured.outTradeNo)
	require.Contains(t, captured.description, captured.outTradeNo)
	require.Greater(t, captured.amountCents, int64(0))
	require.NotEmpty(t, captured.notifyURL)

	var topUp model.TopUp
	require.NoError(t, db.Where("trade_no = ?", captured.outTradeNo).First(&topUp).Error)
	require.Equal(t, common.TopUpStatusPending, topUp.Status)
	require.Equal(t, "wxpay", topUp.PaymentMethod)
	require.Equal(t, "CNY", topUp.Currency)
	require.Equal(t, captured.amountCents, topUp.PayAmountCents)
	require.Greater(t, topUp.ExpireTime, topUp.CreateTime)
}

func TestRequestWxpayDisabled(t *testing.T) {
	setupWxpayControllerTestDB(t)
	withWxpayEnabled(t, false)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/wxpay/pay",
		bytes.NewBufferString(`{"amount":100}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestWxpay(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	require.Contains(t, body, "error")
	require.Contains(t, body, "微信支付未启用")
}

func TestRequestWxpayAmountBelowMin(t *testing.T) {
	db := setupWxpayControllerTestDB(t)
	user := seedWxpayUser(t, db)
	withWxpayEnabled(t, true)

	originalMin := setting.WxpayMinTopUp
	setting.WxpayMinTopUp = 50
	t.Cleanup(func() { setting.WxpayMinTopUp = originalMin })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", user.Id)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/wxpay/pay",
		bytes.NewBufferString(`{"amount":1}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestWxpay(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "error")

	var count int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestRequestWxpayServiceError(t *testing.T) {
	db := setupWxpayControllerTestDB(t)
	user := seedWxpayUser(t, db)
	withWxpayEnabled(t, true)

	mock := &service.MockWechatPayService{
		NativePrepayFunc: func(_ context.Context, _, _ string, _ int64, _ string) (string, error) {
			return "", errWxpayMockFailure
		},
	}
	withWxpayService(t, mock)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", user.Id)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/wxpay/pay",
		bytes.NewBufferString(`{"amount":100}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestWxpay(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	require.Contains(t, body, "error")
	require.Contains(t, body, "拉起支付失败")

	var count int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&count).Error)
	require.Equal(t, int64(0), count, "no TopUp should be inserted when prepay fails")
}
