package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// ==================== Shared external (V1 + V2) test helpers ====================
//
// Strategy:
//   * Tests call setupTestRouter(t) / setupV2TestRouter(t). Those helpers
//     provision a default Platform row in the in-memory DB and store its
//     credentials in defaultTestPlatformHeaders.
//   * The existing doRequest helper auto-injects defaultTestPlatformHeaders,
//     so the ~40 existing test call sites in external_user_test.go and
//     v2_external_platform_test.go pick up auth for free with no per-call
//     migration.
//   * Tests that exercise the 401 branch use doRequestNoAuth, which skips
//     the default headers. Tests that exercise a custom (e.g. disabled)
//     platform use doAuthRequest with explicit headers.
//
// This trades a tiny bit of magic (package-level state) for keeping the diff
// to a manageable size, which is the right tradeoff for a one-off migration.

// defaultTestPlatformHeaders is consulted by doRequest. setupTestRouter(t)
// and setupV2TestRouter(t) populate it; ResetDefaultTestPlatform clears it.
var defaultTestPlatformHeaders map[string]string

// setupTestPlatform creates a platform row in the current test DB and
// returns (platform_id, plaintext_sk). The plaintext is not stored anywhere
// after this call; callers are expected to hold onto it for the test lifetime.
//
// platformIdSuffix is used to build a unique platform_id per test/subtest so
// concurrent or sequential subtests don't collide on the UNIQUE index.
func setupTestPlatform(t *testing.T, platformIdSuffix string) (platformId, platformSk string) {
	t.Helper()
	platformId = "testplt_" + platformIdSuffix
	platformSk = "pk_test_" + common.GetRandomString(32)

	// Ensure the shadow user exists; the middleware would lazily create it,
	// but doing it here makes test cases that assert on shadow_user_id stable.
	shadowUser, err := model.GetOrCreatePlatformShadowUser(platformId)
	if err != nil || shadowUser == nil {
		t.Fatalf("setupTestPlatform: failed to create shadow user: %v", err)
	}

	platform := &model.Platform{
		PlatformId:     platformId,
		Name:           "Test Platform " + platformIdSuffix,
		PlatformSkHash: model.HashPlatformSk(platformSk),
		ShadowUserId:   shadowUser.Id,
		Status:         common.UserStatusEnabled,
	}
	if err := platform.Insert(); err != nil {
		t.Fatalf("setupTestPlatform: failed to insert platform: %v", err)
	}
	return platformId, platformSk
}

// authHeaders returns the X-Platform-Id + X-Platform-Sk pair the middleware
// requires.
func authHeaders(platformId, platformSk string) map[string]string {
	return map[string]string{
		"X-Platform-Id": platformId,
		"X-Platform-Sk": platformSk,
	}
}

// doAuthRequest issues a request with auth headers injected. Mirrors the
// signature of the existing doRequest helper but adds a headers map argument.
func doAuthRequest(router *gin.Engine, method, path string, headers map[string]string, body interface{}) *httptest.ResponseRecorder {
	var buf *bytes.Buffer
	if body != nil {
		jsonData, _ := json.Marshal(body)
		buf = bytes.NewBuffer(jsonData)
	} else {
		buf = &bytes.Buffer{}
	}
	req, _ := http.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// disablePlatform flips a platform's status to disabled. Used by 403 branch tests.
func disablePlatform(t *testing.T, platformId string) {
	t.Helper()
	p, err := model.GetPlatformByPlatformId(platformId)
	if err != nil {
		t.Fatalf("disablePlatform: platform %q not found: %v", platformId, err)
	}
	if err := p.UpdateFields(map[string]interface{}{"status": common.UserStatusDisabled}); err != nil {
		t.Fatalf("disablePlatform: update failed: %v", err)
	}
}

// fmtV2LogPath returns the canonical V2 logs URL with optional query params
// appended. Centralized here so test files don't drift if the path changes again.
func fmtV2LogPath(query string) string {
	if query == "" {
		return "/api/v2/external/logs"
	}
	return fmt.Sprintf("/api/v2/external/logs?%s", query)
}

// installDefaultTestPlatform creates a platform and arms defaultTestPlatformHeaders
// so subsequent doRequest calls authenticate automatically. Suffix should be
// unique per test to avoid cross-test platform_id collisions on the in-memory DB.
func installDefaultTestPlatform(t *testing.T, suffix string) {
	t.Helper()
	pid, psk := setupTestPlatform(t, suffix)
	defaultTestPlatformHeaders = authHeaders(pid, psk)
}

// resetDefaultTestPlatform clears the auto-injection so a single test can
// exercise the no-auth-header path without leaking into siblings.
func resetDefaultTestPlatform() {
	defaultTestPlatformHeaders = nil
}

// doRequestNoAuth is doRequest minus the auto-injected default headers. Use
// in tests that assert on the missing-credentials -> 401 branch.
func doRequestNoAuth(router *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	saved := defaultTestPlatformHeaders
	defaultTestPlatformHeaders = nil
	defer func() { defaultTestPlatformHeaders = saved }()
	return doRequest(router, method, path, body)
}
