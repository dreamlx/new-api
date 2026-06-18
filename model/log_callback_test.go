package model

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCallbackTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &Log{}))

	origDB := DB
	origLogDB := LOG_DB
	origRedis := common.RedisEnabled
	DB = db
	LOG_DB = db
	common.RedisEnabled = false
	initCol()

	t.Cleanup(func() {
		DB = origDB
		LOG_DB = origLogDB
		common.RedisEnabled = origRedis
	})
	return db
}

func ptrCallbackExternalUserId(externalUserId string) *string {
	return &externalUserId
}

// ==================== HMAC Signature ====================

func TestGenerateHMACSignature(t *testing.T) {
	data := []byte(`{"event":"token_consumed","quota":1000}`)
	secret := "test_secret_key"

	sig := GenerateHMACSignatureForTest(data, secret)

	// Verify independently
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(data)
	expected := hex.EncodeToString(h.Sum(nil))

	assert.Equal(t, expected, sig)
	assert.Len(t, sig, 64) // SHA256 hex = 64 chars
}

func TestHMACSignatureDifferentSecrets(t *testing.T) {
	data := []byte(`{"event":"test"}`)
	sig1 := GenerateHMACSignatureForTest(data, "secret1")
	sig2 := GenerateHMACSignatureForTest(data, "secret2")
	assert.NotEqual(t, sig1, sig2)
}

func TestHMACSignatureDifferentData(t *testing.T) {
	secret := "same_secret"
	sig1 := GenerateHMACSignatureForTest([]byte(`{"a":1}`), secret)
	sig2 := GenerateHMACSignatureForTest([]byte(`{"a":2}`), secret)
	assert.NotEqual(t, sig1, sig2)
}

// ==================== SendConsumeCallback Conditions ====================

func TestCallbackSkipsWhenDisabled(t *testing.T) {
	db := setupCallbackTestDB(t)

	user := &User{Username: "cb_user", Email: "cb@test.com", ExternalUserId: ptrCallbackExternalUserId("ext_cb_001"), IsExternal: true}
	db.Create(user)

	token := &Token{
		UserId: user.Id, Key: "cbtoken001",
		Status: common.TokenStatusEnabled, CreatedTime: common.GetTimestamp(), ExpiredTime: -1,
		CallbackEnabled: false, CallbackUrl: "http://should-not-be-called.invalid/",
	}
	db.Create(token)

	log := &Log{Id: 1, ModelName: "test", Quota: 100}

	// Should not panic or make HTTP calls — just silently return
	SendConsumeCallback(user.Id, token.Id, log)
}

func TestCallbackSkipsWhenNoUrl(t *testing.T) {
	db := setupCallbackTestDB(t)

	user := &User{Username: "cb_user2", Email: "cb2@test.com", ExternalUserId: ptrCallbackExternalUserId("ext_cb_002"), IsExternal: true}
	db.Create(user)

	token := &Token{
		UserId: user.Id, Key: "cbtoken002",
		Status: common.TokenStatusEnabled, CreatedTime: common.GetTimestamp(), ExpiredTime: -1,
		CallbackEnabled: true, CallbackUrl: "", // empty URL
	}
	db.Create(token)

	log := &Log{Id: 2, ModelName: "test", Quota: 100}
	SendConsumeCallback(user.Id, token.Id, log)
	// No panic = pass
}

func TestCallbackSkipsNonExternalUser(t *testing.T) {
	db := setupCallbackTestDB(t)

	user := &User{Username: "internal_user", Email: "int@test.com"}
	db.Create(user)

	token := &Token{
		UserId: user.Id, Key: "cbtoken003",
		Status: common.TokenStatusEnabled, CreatedTime: common.GetTimestamp(), ExpiredTime: -1,
		CallbackEnabled: true, CallbackUrl: "http://example.com/cb",
	}
	db.Create(token)

	log := &Log{Id: 3, ModelName: "test", Quota: 100}
	SendConsumeCallback(user.Id, token.Id, log)
	// Should silently skip — no external_user_id
}

func TestCallbackSkipsInvalidTokenId(t *testing.T) {
	setupCallbackTestDB(t)
	log := &Log{Id: 4, ModelName: "test", Quota: 100}
	SendConsumeCallback(1, 99999, log) // non-existent token
	// No panic = pass
}

// ==================== SendConsumeCallback Happy Path ====================

func TestCallbackSendsCorrectPayload(t *testing.T) {
	db := setupCallbackTestDB(t)

	var mu sync.Mutex
	var receivedBody []byte
	var receivedHeaders http.Header

	// Start a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		receivedHeaders = r.Header.Clone()
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}))
	defer server.Close()

	user := &User{Username: "cb_happy", Email: "happy@test.com", ExternalUserId: ptrCallbackExternalUserId("ext_happy_001"), IsExternal: true}
	db.Create(user)

	token := &Token{
		UserId: user.Id, Key: "cbhappytoken",
		Status: common.TokenStatusEnabled, CreatedTime: common.GetTimestamp(), ExpiredTime: -1,
		CallbackEnabled: true, CallbackUrl: server.URL, CallbackSecret: "mysecret",
	}
	db.Create(token)

	log := &Log{
		Id: 10, ModelName: "deepseek-chat", Quota: 5000,
		PromptTokens: 500, CompletionTokens: 200,
	}

	SendConsumeCallback(user.Id, token.Id, log)

	mu.Lock()
	defer mu.Unlock()

	require.NotEmpty(t, receivedBody)

	// Verify JSON structure
	var payload map[string]interface{}
	err := common.Unmarshal(receivedBody, &payload)
	require.NoError(t, err)

	assert.Equal(t, "token_consumed", payload["event"])
	assert.Equal(t, "ext_happy_001", payload["external_user_id"])
	assert.Equal(t, "sk-cbhappytoken", payload["token_key"])
	assert.Equal(t, "deepseek-chat", payload["model"])
	assert.Equal(t, float64(500), payload["prompt_tokens"])
	assert.Equal(t, float64(200), payload["completion_tokens"])
	assert.Equal(t, float64(5000), payload["quota_consumed"])

	// Verify HMAC signature header
	sig := receivedHeaders.Get("X-Callback-Signature")
	assert.NotEmpty(t, sig)
	expectedSig := GenerateHMACSignatureForTest(receivedBody, "mysecret")
	assert.Equal(t, expectedSig, sig)

	// Verify Content-Type
	assert.Equal(t, "application/json", receivedHeaders.Get("Content-Type"))
}

func TestCallbackNoSignatureWhenNoSecret(t *testing.T) {
	db := setupCallbackTestDB(t)

	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(200)
	}))
	defer server.Close()

	user := &User{Username: "cb_nosec", Email: "nosec@test.com", ExternalUserId: ptrCallbackExternalUserId("ext_nosec"), IsExternal: true}
	db.Create(user)

	token := &Token{
		UserId: user.Id, Key: "cbnosectoken",
		Status: common.TokenStatusEnabled, CreatedTime: common.GetTimestamp(), ExpiredTime: -1,
		CallbackEnabled: true, CallbackUrl: server.URL, CallbackSecret: "", // no secret
	}
	db.Create(token)

	log := &Log{Id: 11, ModelName: "gpt-4", Quota: 1000}
	SendConsumeCallback(user.Id, token.Id, log)

	assert.Empty(t, receivedHeaders.Get("X-Callback-Signature"))
}
