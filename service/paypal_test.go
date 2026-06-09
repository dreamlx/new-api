package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/require"
)

func TestVerifyPayPalWebhookSignatureUsesPayPalVerificationAPI(t *testing.T) {
	var verifyPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			require.Equal(t, "POST", r.Method)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-token"}`))
		case "/v1/notifications/verify-webhook-signature":
			require.Equal(t, "POST", r.Method)
			require.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))
			require.NoError(t, common.DecodeJson(r.Body, &verifyPayload))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"verification_status":"SUCCESS"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	originalBaseURL := payPalBaseURLOverride
	originalClientID := setting.PayPalClientId
	originalClientSecret := setting.PayPalClientSecret
	originalWebhookID := setting.PayPalWebhookSecret
	payPalBaseURLOverride = server.URL
	setting.PayPalClientId = "client-id"
	setting.PayPalClientSecret = "client-secret"
	setting.PayPalWebhookSecret = "webhook-id"
	t.Cleanup(func() {
		payPalBaseURLOverride = originalBaseURL
		setting.PayPalClientId = originalClientID
		setting.PayPalClientSecret = originalClientSecret
		setting.PayPalWebhookSecret = originalWebhookID
	})

	ok := VerifyPayPalWebhookSignature(
		"transmission-id",
		"2026-06-08T00:00:00Z",
		"https://api-m.sandbox.paypal.com/cert.pem",
		[]byte(`{"event_type":"PAYMENT.CAPTURE.COMPLETED"}`),
		"signature",
	)

	require.True(t, ok)
	require.Equal(t, "webhook-id", verifyPayload["webhook_id"])
	require.Equal(t, "transmission-id", verifyPayload["transmission_id"])
	require.Equal(t, "signature", verifyPayload["transmission_sig"])
	require.Equal(t, "2026-06-08T00:00:00Z", verifyPayload["transmission_time"])
	require.Equal(t, "https://api-m.sandbox.paypal.com/cert.pem", verifyPayload["cert_url"])
	require.NotNil(t, verifyPayload["webhook_event"])
}
