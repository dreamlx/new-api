package controller

import (
	"bytes"
	"fmt"
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

func setupPayPalControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
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
		UserId:          user.Id,
		Amount:          100,
		Money:           2,
		TradeNo:         tradeNo,
		PaymentMethod:   PaymentMethodPayPal,
		PaymentProvider: model.PaymentProviderPayPal,
		CreateTime:      1,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(topUp).Error)
}

func postPayPalWebhook(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	return postPayPalWebhookWithVerifier(t, body, func(_, _, _ string, _ []byte, _ string) bool {
		return true
	})
}

func postPayPalWebhookWithVerifier(
	t *testing.T,
	body string,
	verifier func(transmissionID, transmissionTime, certURL string, payload []byte, signature string) bool,
) *httptest.ResponseRecorder {
	t.Helper()

	originalVerifier := payPalVerifyWebhookSignature
	payPalVerifyWebhookSignature = verifier
	t.Cleanup(func() { payPalVerifyWebhookSignature = originalVerifier })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/paypal/webhook", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("Paypal-Transmission-Id", "transmission-id")
	ctx.Request.Header.Set("Paypal-Transmission-Time", "2026-06-08T00:00:00Z")
	ctx.Request.Header.Set("Paypal-Cert-Url", "https://api-m.sandbox.paypal.com/certs/test")
	ctx.Request.Header.Set("Paypal-Transmission-Sig", "signature")

	PayPalWebhook(ctx)

	return recorder
}

func requestPayPalTopUpForTest(t *testing.T, userID int, body string) (int, string) {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", userID)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/paypal/pay", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	RequestPayPalTopUp(ctx)
	return recorder.Code, recorder.Body.String()
}

func TestRequestPayPalTopUpRejectsBelowPayPalMinimum(t *testing.T) {
	db := setupPayPalControllerTestDB(t)
	user := &model.User{
		Id:       1,
		Username: "paypal_min_user",
		Password: "password",
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, db.Create(user).Error)

	originalClientID := setting.PayPalClientId
	originalClientSecret := setting.PayPalClientSecret
	originalWebhookSecret := setting.PayPalWebhookSecret
	originalMinTopUp := setting.PayPalMinTopUp
	originalCreateOrder := payPalCreateOrder
	setting.PayPalClientId = "client_id"
	setting.PayPalClientSecret = "client_secret"
	setting.PayPalWebhookSecret = "webhook_secret"
	setting.PayPalMinTopUp = 50
	payPalCreateOrder = func(_, _, _, _, _ string) (string, error) {
		t.Fatal("PayPal order should not be created below PayPalMinTopUp")
		return "", nil
	}
	t.Cleanup(func() {
		setting.PayPalClientId = originalClientID
		setting.PayPalClientSecret = originalClientSecret
		setting.PayPalWebhookSecret = originalWebhookSecret
		setting.PayPalMinTopUp = originalMinTopUp
		payPalCreateOrder = originalCreateOrder
	})

	code, body := requestPayPalTopUpForTest(t, user.Id, `{"amount":10}`)

	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, "充值数量不能小于 50")
}

func TestPayPalWebhookRejectsInvalidSignature(t *testing.T) {
	db := setupPayPalControllerTestDB(t)
	seedPayPalTopUp(t, db, "paypal_ref_bad_sig")

	body := `{
		"event_type": "PAYMENT.CAPTURE.COMPLETED",
		"resource": {
			"id": "CAPTURE_BAD_SIG",
			"invoice_id": "paypal_ref_bad_sig",
			"amount": {
				"currency_code": "USD",
				"value": "2.00"
			}
		}
	}`

	recorder := postPayPalWebhookWithVerifier(t, body, func(_, _, _ string, _ []byte, _ string) bool {
		return false
	})

	require.Equal(t, http.StatusUnauthorized, recorder.Code)

	var topUp model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "paypal_ref_bad_sig").First(&topUp).Error)
	require.Equal(t, common.TopUpStatusPending, topUp.Status)
}

func TestPayPalCaptureCompletedWebhookRejectsAmountMismatch(t *testing.T) {
	db := setupPayPalControllerTestDB(t)
	seedPayPalTopUp(t, db, "paypal_ref_amount_mismatch")

	body := `{
		"event_type": "PAYMENT.CAPTURE.COMPLETED",
		"resource": {
			"id": "CAPTURE_MISMATCH",
			"invoice_id": "paypal_ref_amount_mismatch",
			"amount": {
				"currency_code": "USD",
				"value": "1.00"
			}
		}
	}`

	recorder := postPayPalWebhook(t, body)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)

	var topUp model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "paypal_ref_amount_mismatch").First(&topUp).Error)
	require.Equal(t, common.TopUpStatusPending, topUp.Status)

	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	require.Equal(t, 100, user.Quota)
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

func TestPayPalApprovedWebhookReturns500WhenCaptureFails(t *testing.T) {
	setupPayPalControllerTestDB(t)

	originalCapture := payPalCaptureOrder
	payPalCaptureOrder = func(orderID string) (*service.PayPalCaptureResponse, error) {
		require.Equal(t, "PAYPAL_ORDER_CAPTURE_FAIL", orderID)
		return nil, fmt.Errorf("PayPal capture HTTP 503: service unavailable")
	}
	t.Cleanup(func() { payPalCaptureOrder = originalCapture })

	body := `{
		"event_type": "CHECKOUT.ORDER.APPROVED",
		"resource": {
			"id": "PAYPAL_ORDER_CAPTURE_FAIL"
		}
	}`

	recorder := postPayPalWebhook(t, body)
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
}

func TestPayPalApprovedWebhookReturns500WhenLocalCompletionFails(t *testing.T) {
	setupPayPalControllerTestDB(t)

	originalCapture := payPalCaptureOrder
	payPalCaptureOrder = func(orderID string) (*service.PayPalCaptureResponse, error) {
		require.Equal(t, "PAYPAL_ORDER_MISSING_LOCAL", orderID)
		return &service.PayPalCaptureResponse{
			Id:     "PAYPAL_ORDER_MISSING_LOCAL",
			Status: "COMPLETED",
			PurchaseUnits: []service.PayPalCapturedUnit{
				{
					InvoiceId: "missing_paypal_ref",
					Payments: &service.PayPalCapturedPayments{
						Captures: []service.PayPalCapture{
							{
								Id:     "CAPTURE_MISSING_LOCAL",
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
			"id": "PAYPAL_ORDER_MISSING_LOCAL"
		}
	}`

	recorder := postPayPalWebhook(t, body)
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
}

func TestPayPalApprovedWebhookAlreadyCapturedFallsBackToGetOrder(t *testing.T) {
	db := setupPayPalControllerTestDB(t)
	seedPayPalTopUp(t, db, "paypal_ref_approved_already_captured")

	originalCapture := payPalCaptureOrder
	payPalCaptureOrder = func(orderID string) (*service.PayPalCaptureResponse, error) {
		require.Equal(t, "PAYPAL_ORDER_APPROVED_ALREADY_CAPTURED", orderID)
		return nil, fmt.Errorf("PayPal capture HTTP 422: issue=ORDER_ALREADY_CAPTURED desc=order already captured")
	}
	t.Cleanup(func() { payPalCaptureOrder = originalCapture })

	originalGet := payPalGetOrder
	payPalGetOrder = func(orderID string) (*service.PayPalCaptureResponse, error) {
		require.Equal(t, "PAYPAL_ORDER_APPROVED_ALREADY_CAPTURED", orderID)
		return &service.PayPalCaptureResponse{
			Id:     "PAYPAL_ORDER_APPROVED_ALREADY_CAPTURED",
			Status: "COMPLETED",
			PurchaseUnits: []service.PayPalCapturedUnit{
				{
					InvoiceId: "paypal_ref_approved_already_captured",
					Payments: &service.PayPalCapturedPayments{
						Captures: []service.PayPalCapture{
							{
								Id:     "CAPTURE_APPROVED_ALREADY_CAPTURED",
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
	t.Cleanup(func() { payPalGetOrder = originalGet })

	body := `{
		"event_type": "CHECKOUT.ORDER.APPROVED",
		"resource": {
			"id": "PAYPAL_ORDER_APPROVED_ALREADY_CAPTURED"
		}
	}`

	recorder := postPayPalWebhook(t, body)
	require.Equal(t, http.StatusOK, recorder.Code)

	var topUp model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "paypal_ref_approved_already_captured").First(&topUp).Error)
	require.Equal(t, common.TopUpStatusSuccess, topUp.Status)
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

func TestPayPalCheckoutOrderCompletedWebhookUsesInvoiceIdWhenReferenceIdIsDefault(t *testing.T) {
	db := setupPayPalControllerTestDB(t)
	seedPayPalTopUp(t, db, "paypal_ref_invoice_default")

	body := `{
		"event_type": "CHECKOUT.ORDER.COMPLETED",
		"resource": {
			"id": "PAYPAL_ORDER_DEFAULT",
			"purchase_units": [{
				"reference_id": "DEFAULT",
				"invoice_id": "paypal_ref_invoice_default",
				"payments": {
					"captures": [{
						"id": "CAPTURE_DEFAULT",
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
	require.NoError(t, db.Where("trade_no = ?", "paypal_ref_invoice_default").First(&topUp).Error)
	require.Equal(t, common.TopUpStatusSuccess, topUp.Status)
}

func TestPayPalReturnCapturesOrderAndRedirectsSuccess(t *testing.T) {
	db := setupPayPalControllerTestDB(t)
	seedPayPalTopUp(t, db, "paypal_ref_return")

	captureCalls := 0
	originalCapture := payPalCaptureOrder
	payPalCaptureOrder = func(orderID string) (*service.PayPalCaptureResponse, error) {
		captureCalls++
		require.Equal(t, "PAYPAL_ORDER_RETURN", orderID)
		return &service.PayPalCaptureResponse{
			Id:     "PAYPAL_ORDER_RETURN",
			Status: "COMPLETED",
			PurchaseUnits: []service.PayPalCapturedUnit{
				{
					InvoiceId: "paypal_ref_return",
					Payments: &service.PayPalCapturedPayments{
						Captures: []service.PayPalCapture{
							{
								Id:     "CAPTURE_RETURN",
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

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/paypal/return?token=PAYPAL_ORDER_RETURN&PayerID=PAYER_123", nil)

	PayPalReturn(ctx)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "http://localhost:3000/console/topup?pay=success&show_history=true", recorder.Header().Get("Location"))
	require.Equal(t, 1, captureCalls)

	var topUp model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "paypal_ref_return").First(&topUp).Error)
	require.Equal(t, common.TopUpStatusSuccess, topUp.Status)
}

func TestPayPalReturnCaptureUsesInvoiceIdWhenReferenceIdIsDefault(t *testing.T) {
	db := setupPayPalControllerTestDB(t)
	seedPayPalTopUp(t, db, "paypal_ref_return_default")

	originalCapture := payPalCaptureOrder
	payPalCaptureOrder = func(orderID string) (*service.PayPalCaptureResponse, error) {
		require.Equal(t, "PAYPAL_ORDER_RETURN_DEFAULT", orderID)
		return &service.PayPalCaptureResponse{
			Id:     "PAYPAL_ORDER_RETURN_DEFAULT",
			Status: "COMPLETED",
			PurchaseUnits: []service.PayPalCapturedUnit{
				{
					ReferenceId: "DEFAULT",
					InvoiceId:   "paypal_ref_return_default",
					Payments: &service.PayPalCapturedPayments{
						Captures: []service.PayPalCapture{
							{
								Id:     "CAPTURE_RETURN_DEFAULT",
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

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/paypal/return?token=PAYPAL_ORDER_RETURN_DEFAULT", nil)

	PayPalReturn(ctx)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "http://localhost:3000/console/topup?pay=success&show_history=true", recorder.Header().Get("Location"))

	var topUp model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "paypal_ref_return_default").First(&topUp).Error)
	require.Equal(t, common.TopUpStatusSuccess, topUp.Status)
}

func TestPayPalCaptureCompletedWebhookFallsBackToGetOrderWhenInvoiceIdMissing(t *testing.T) {
	db := setupPayPalControllerTestDB(t)
	seedPayPalTopUp(t, db, "paypal_ref_no_invoice")

	originalGet := payPalGetOrder
	payPalGetOrder = func(orderID string) (*service.PayPalCaptureResponse, error) {
		require.Equal(t, "PAYPAL_ORDER_FALLBACK", orderID)
		return &service.PayPalCaptureResponse{
			Id:     "PAYPAL_ORDER_FALLBACK",
			Status: "COMPLETED",
			PurchaseUnits: []service.PayPalCapturedUnit{
				{
					InvoiceId: "paypal_ref_no_invoice",
					Payments: &service.PayPalCapturedPayments{
						Captures: []service.PayPalCapture{
							{Id: "CAP_FB", Status: "COMPLETED", Amount: service.PayPalAmount{CurrencyCode: "USD", Value: "2.00"}},
						},
					},
				},
			},
		}, nil
	}
	t.Cleanup(func() { payPalGetOrder = originalGet })

	body := `{
		"event_type": "PAYMENT.CAPTURE.COMPLETED",
		"resource": {
			"id": "CAP_FB",
			"amount": {"currency_code": "USD", "value": "2.00"},
			"supplementary_data": {"related_ids": {"order_id": "PAYPAL_ORDER_FALLBACK"}}
		}
	}`

	recorder := postPayPalWebhook(t, body)
	require.Equal(t, http.StatusOK, recorder.Code)

	var topUp model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "paypal_ref_no_invoice").First(&topUp).Error)
	require.Equal(t, common.TopUpStatusSuccess, topUp.Status)
}

func TestPayPalCaptureCompletedWebhookReturns500WhenGetOrderFails(t *testing.T) {
	setupPayPalControllerTestDB(t)

	originalGet := payPalGetOrder
	payPalGetOrder = func(orderID string) (*service.PayPalCaptureResponse, error) {
		return nil, fmt.Errorf("PayPal get order HTTP 503: service unavailable")
	}
	t.Cleanup(func() { payPalGetOrder = originalGet })

	body := `{
		"event_type": "PAYMENT.CAPTURE.COMPLETED",
		"resource": {
			"id": "CAP_ERR",
			"amount": {"currency_code": "USD", "value": "2.00"},
			"supplementary_data": {"related_ids": {"order_id": "PAYPAL_ORDER_ERR"}}
		}
	}`

	recorder := postPayPalWebhook(t, body)
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
}

func TestPayPalReturnOrderAlreadyCapturedVerifiesViaGetOrderThenRedirectsSuccess(t *testing.T) {
	db := setupPayPalControllerTestDB(t)
	seedPayPalTopUp(t, db, "paypal_ref_already_cap")

	originalCapture := payPalCaptureOrder
	payPalCaptureOrder = func(orderID string) (*service.PayPalCaptureResponse, error) {
		return nil, fmt.Errorf("PayPal capture HTTP 422: issue=ORDER_ALREADY_CAPTURED desc=order already captured")
	}
	t.Cleanup(func() { payPalCaptureOrder = originalCapture })

	originalGet := payPalGetOrder
	payPalGetOrder = func(orderID string) (*service.PayPalCaptureResponse, error) {
		require.Equal(t, "PAYPAL_ORDER_ALREADY_CAP", orderID)
		return &service.PayPalCaptureResponse{
			Id:     "PAYPAL_ORDER_ALREADY_CAP",
			Status: "COMPLETED",
			PurchaseUnits: []service.PayPalCapturedUnit{
				{
					InvoiceId: "paypal_ref_already_cap",
					Payments: &service.PayPalCapturedPayments{
						Captures: []service.PayPalCapture{
							{Id: "CAP_AC", Status: "COMPLETED", Amount: service.PayPalAmount{CurrencyCode: "USD", Value: "2.00"}},
						},
					},
				},
			},
		}, nil
	}
	t.Cleanup(func() { payPalGetOrder = originalGet })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/paypal/return?token=PAYPAL_ORDER_ALREADY_CAP", nil)

	PayPalReturn(ctx)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "http://localhost:3000/console/topup?pay=success&show_history=true", recorder.Header().Get("Location"))

	var topUp model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "paypal_ref_already_cap").First(&topUp).Error)
	require.Equal(t, common.TopUpStatusSuccess, topUp.Status)
}

func TestPayPalReturnWithoutTokenRedirectsFail(t *testing.T) {
	setupPayPalControllerTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/paypal/return", nil)

	PayPalReturn(ctx)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "http://localhost:3000/console/topup?pay=fail&show_history=true", recorder.Header().Get("Location"))
}

func TestPayPalReturnUnknownLocalOrderRedirectsPending(t *testing.T) {
	setupPayPalControllerTestDB(t)

	originalCapture := payPalCaptureOrder
	payPalCaptureOrder = func(orderID string) (*service.PayPalCaptureResponse, error) {
		require.Equal(t, "PAYPAL_ORDER_UNKNOWN", orderID)
		return &service.PayPalCaptureResponse{
			Id:     "PAYPAL_ORDER_UNKNOWN",
			Status: "COMPLETED",
			PurchaseUnits: []service.PayPalCapturedUnit{
				{
					InvoiceId: "missing_paypal_ref",
					Payments: &service.PayPalCapturedPayments{
						Captures: []service.PayPalCapture{
							{
								Id:     "CAPTURE_UNKNOWN",
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

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/paypal/return?token=PAYPAL_ORDER_UNKNOWN", nil)

	PayPalReturn(ctx)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "http://localhost:3000/console/topup?pay=pending&show_history=true", recorder.Header().Get("Location"))
}
