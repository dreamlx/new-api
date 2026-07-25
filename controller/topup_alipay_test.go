package controller

import (
	"bytes"
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

var errAlipayMockFailure = errors.New("alipay mock failure")

func setupAlipayControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
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

func seedAlipayUser(t *testing.T, db *gorm.DB) *model.User {
	t.Helper()
	user := &model.User{
		Id:       1,
		Username: "alipay_user",
		Password: "password",
		Status:   common.UserStatusEnabled,
		Quota:    100,
		Group:    "default",
	}
	require.NoError(t, db.Create(user).Error)
	return user
}

// withAlipayEnabled flips the enable flag and the identity-validation fields
// used by the notify handler, restoring them on test cleanup.
func withAlipayEnabled(t *testing.T, enabled bool) {
	t.Helper()
	original := setting.AlipayEnabled
	originalAppId := setting.AlipayAppId
	originalSellerId := setting.AlipaySellerId
	setting.AlipayEnabled = enabled
	setting.AlipayAppId = "test_app_id"
	setting.AlipaySellerId = "test_seller_id"
	t.Cleanup(func() {
		setting.AlipayEnabled = original
		setting.AlipayAppId = originalAppId
		setting.AlipaySellerId = originalSellerId
	})
}

// withAlipayService injects a mock AlipayService for the duration of the test.
func withAlipayService(t *testing.T, mock service.AlipayService) {
	t.Helper()
	original := alipayServiceProvider
	alipayServiceProvider = func() (service.AlipayService, error) {
		return mock, nil
	}
	t.Cleanup(func() { alipayServiceProvider = original })
}

func TestRequestAlipayHappyPath(t *testing.T) {
	db := setupAlipayControllerTestDB(t)
	user := seedAlipayUser(t, db)
	withAlipayEnabled(t, true)

	var captured struct {
		outTradeNo  string
		totalAmount string
		notifyURL   string
		returnURL   string
	}
	mock := &service.MockAlipayService{
		TradePagePayFunc: func(outTradeNo, subject, totalAmount, notifyURL, returnURL string) (string, error) {
			captured.outTradeNo = outTradeNo
			captured.totalAmount = totalAmount
			captured.notifyURL = notifyURL
			captured.returnURL = returnURL
			return "https://openapi.alipaydev.com/gateway.do?out_trade_no=" + outTradeNo, nil
		},
	}
	withAlipayService(t, mock)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", user.Id)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/alipay/pay",
		bytes.NewBufferString(`{"amount":100}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestAlipay(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	require.Contains(t, body, "success")
	require.Contains(t, body, "pay_link")
	require.NotEmpty(t, captured.outTradeNo)

	var topUp model.TopUp
	require.NoError(t, db.Where("trade_no = ?", captured.outTradeNo).First(&topUp).Error)
	require.Equal(t, common.TopUpStatusPending, topUp.Status)
	require.Equal(t, "alipay", topUp.PaymentMethod)
	require.Greater(t, topUp.PayAmountCents, int64(0))
	require.Greater(t, topUp.ExpireTime, topUp.CreateTime)
	require.NotEmpty(t, captured.notifyURL)
	require.NotEmpty(t, captured.returnURL)
}

func TestRequestAlipayDisabled(t *testing.T) {
	setupAlipayControllerTestDB(t)
	withAlipayEnabled(t, false)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/alipay/pay",
		bytes.NewBufferString(`{"amount":100}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestAlipay(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "error")
}

func TestRequestAlipayAmountBelowMin(t *testing.T) {
	db := setupAlipayControllerTestDB(t)
	user := seedAlipayUser(t, db)
	withAlipayEnabled(t, true)

	originalMin := setting.AlipayMinTopUp
	setting.AlipayMinTopUp = 50
	t.Cleanup(func() { setting.AlipayMinTopUp = originalMin })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", user.Id)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/alipay/pay",
		bytes.NewBufferString(`{"amount":1}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestAlipay(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "error")

	var count int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestRequestAlipayServiceError(t *testing.T) {
	db := setupAlipayControllerTestDB(t)
	user := seedAlipayUser(t, db)
	withAlipayEnabled(t, true)

	mock := &service.MockAlipayService{
		TradePagePayFunc: func(outTradeNo, subject, totalAmount, notifyURL, returnURL string) (string, error) {
			return "", errAlipayMockFailure
		},
	}
	withAlipayService(t, mock)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", user.Id)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/alipay/pay",
		bytes.NewBufferString(`{"amount":100}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestAlipay(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "error")

	var count int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&count).Error)
	require.Equal(t, int64(0), count, "no TopUp should be inserted when SDK call fails")
}
