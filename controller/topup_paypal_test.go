package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPayPalControllerTestDB(t *testing.T) *gorm.DB {
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

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}, &model.SubscriptionOrder{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func seedPayPalTopUp(t *testing.T, db *gorm.DB, tradeNo string) {
	t.Helper()

	user := &model.User{
		Id:       1,
		Username: "paypal_user",
		Password: "password",
		Status:   common.UserStatusEnabled,
		Quota:    100,
	}
	require.NoError(t, db.Create(user).Error)

	topUp := &model.TopUp{
		UserId:        user.Id,
		Amount:        100,
		Money:         2,
		TradeNo:       tradeNo,
		PaymentMethod: PaymentMethodPayPal,
		CreateTime:    1,
		Status:        common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(topUp).Error)
}

func postPayPalWebhook(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/paypal/webhook", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	PayPalWebhook(ctx)

	return recorder
}

func TestPayPalApprovedWebhookCapturesOrderAndCompletesTopUp(t *testing.T) {
	db := setupPayPalControllerTestDB(t)
	seedPayPalTopUp(t, db, "paypal_ref_approved")

	captureCalls := 0
	originalCapture := payPalCaptureOrder
	payPalCaptureOrder = func(orderID string) (*service.PayPalCaptureResponse, error) {
		captureCalls++
		require.Equal(t, "PAYPAL_ORDER_123", orderID)
		return &service.PayPalCaptureResponse{
			Id:     "PAYPAL_ORDER_123",
			Status: "COMPLETED",
			PurchaseUnits: []service.PayPalCapturedUnit{
				{
					ReferenceId: "paypal_ref_approved",
					Payments: &service.PayPalCapturedPayments{
						Captures: []service.PayPalCapture{
							{
								Id:     "CAPTURE_123",
								Status: "COMPLETED",
								Amount: service.PayPalAmount{
									CurrencyCode: "USD",
									Value:        "2.00",
								},
							},
						},
					},
				},
			},
		}, nil
	}
	t.Cleanup(func() { payPalCaptureOrder = originalCapture })

	body := `{
		"event_type": "CHECKOUT.ORDER.APPROVED",
		"resource": {
			"id": "PAYPAL_ORDER_123",
			"purchase_units": [{"reference_id": "paypal_ref_approved"}]
		}
	}`

	recorder := postPayPalWebhook(t, body)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, captureCalls)

	var topUp model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "paypal_ref_approved").First(&topUp).Error)
	require.Equal(t, common.TopUpStatusSuccess, topUp.Status)

	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	require.Equal(t, 100+int(2*common.QuotaPerUnit), user.Quota)
}

func TestPayPalCaptureCompletedWebhookUsesInvoiceIdAndIsIdempotent(t *testing.T) {
	db := setupPayPalControllerTestDB(t)
	seedPayPalTopUp(t, db, "paypal_ref_capture")

	body := `{
		"event_type": "PAYMENT.CAPTURE.COMPLETED",
		"resource": {
			"id": "CAPTURE_456",
			"invoice_id": "paypal_ref_capture",
			"amount": {
				"currency_code": "USD",
				"value": "2.00"
			}
		}
	}`

	recorder := postPayPalWebhook(t, body)
	require.Equal(t, http.StatusOK, recorder.Code)

	recorder = postPayPalWebhook(t, body)
	require.Equal(t, http.StatusOK, recorder.Code)

	var topUp model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "paypal_ref_capture").First(&topUp).Error)
	require.Equal(t, common.TopUpStatusSuccess, topUp.Status)

	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	require.Equal(t, 100+int(2*common.QuotaPerUnit), user.Quota)
}

func TestPayPalCheckoutOrderCompletedWebhookKeepsOrderPayloadSupport(t *testing.T) {
	db := setupPayPalControllerTestDB(t)
	seedPayPalTopUp(t, db, "paypal_ref_order_completed")

	body := `{
		"event_type": "CHECKOUT.ORDER.COMPLETED",
		"resource": {
			"id": "PAYPAL_ORDER_789",
			"purchase_units": [{
				"reference_id": "paypal_ref_order_completed",
				"payments": {
					"captures": [{
						"id": "CAPTURE_789",
						"status": "COMPLETED",
						"amount": {
							"currency_code": "USD",
							"value": "2.00"
						}
					}]
				}
			}]
		}
	}`

	recorder := postPayPalWebhook(t, body)
	require.Equal(t, http.StatusOK, recorder.Code)

	var topUp model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "paypal_ref_order_completed").First(&topUp).Error)
	require.Equal(t, common.TopUpStatusSuccess, topUp.Status)

	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	require.Equal(t, 100+int(2*common.QuotaPerUnit), user.Quota)
}
