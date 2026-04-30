package ospreyai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// TestSetupAuthHeaderOpenAI verifies OpenAI authentication header
func TestSetupAuthHeaderOpenAI(t *testing.T) {
	req := &http.Header{}
	apiKey := "test-openai-key"

	SetupAuthHeader(req, apiKey, constant.ChannelTypeOpenAI)

	auth := req.Get("Authorization")
	expected := "Bearer test-openai-key"

	if auth != expected {
		t.Errorf("OpenAI auth header: got %q, want %q", auth, expected)
	}
}

// TestSetupAuthHeaderClaude verifies Claude/Anthropic authentication headers
func TestSetupAuthHeaderClaude(t *testing.T) {
	req := &http.Header{}
	apiKey := "test-claude-key"

	SetupAuthHeader(req, apiKey, constant.ChannelTypeAnthropic)

	apiKeyHeader := req.Get("x-api-key")
	versionHeader := req.Get("anthropic-version")

	if apiKeyHeader != apiKey {
		t.Errorf("Claude x-api-key header: got %q, want %q", apiKeyHeader, apiKey)
	}

	if versionHeader != "2023-06-01" {
		t.Errorf("Claude anthropic-version header: got %q, want %q", versionHeader, "2023-06-01")
	}
}

// TestSetupAuthHeaderAzure verifies Azure authentication header
func TestSetupAuthHeaderAzure(t *testing.T) {
	req := &http.Header{}
	apiKey := "test-azure-key"

	SetupAuthHeader(req, apiKey, constant.ChannelTypeAzure)

	apiKeyHeader := req.Get("api-key")

	if apiKeyHeader != apiKey {
		t.Errorf("Azure api-key header: got %q, want %q", apiKeyHeader, apiKey)
	}
}

// TestIsApiKeyInQuery verifies query parameter detection
func TestIsApiKeyInQuery(t *testing.T) {
	tests := []struct {
		channelType int
		expected    bool
	}{
		{constant.ChannelTypeGemini, true},
		{constant.ChannelTypeOpenAI, false},
		{constant.ChannelTypeAnthropic, false},
	}

	for _, tt := range tests {
		result := IsApiKeyInQuery(tt.channelType)
		if result != tt.expected {
			t.Errorf("IsApiKeyInQuery(%d): got %v, want %v", tt.channelType, result, tt.expected)
		}
	}
}

// TestGetRequestURLPreservePath verifies URL path preservation
func TestGetRequestURLPreservePath(t *testing.T) {
	adaptor := &Adaptor{}

	tests := []struct {
		baseURL     string
		requestPath string
		name        string
	}{
		{
			baseURL:     "https://api.example.com",
			requestPath: "/v1/messages",
			name:        "Claude path",
		},
		{
			baseURL:     "https://api.example.com",
			requestPath: "/v1/chat/completions",
			name:        "OpenAI path",
		},
		{
			baseURL:     "https://api.example.com",
			requestPath: "/v1beta/models/test:generateContent",
			name:        "Gemini path",
		},
	}

	for _, tt := range tests {
		info := &relaycommon.RelayInfo{
			RequestURLPath: tt.requestPath,
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelBaseUrl: tt.baseURL,
				ChannelType:    constant.ChannelTypeOpenAI,
				ApiKey:         "test-key",
			},
		}

		url, err := adaptor.GetRequestURL(info)
		if err != nil {
			t.Errorf("%s: got error %v", tt.name, err)
			continue
		}

		// Path should be preserved
		if !strings.Contains(url, tt.requestPath) {
			t.Errorf("%s: URL %q should contain path %q", tt.name, url, tt.requestPath)
		}
	}
}

// TestGetRequestURLGeminiApiKey verifies API key in query parameter for Gemini
func TestGetRequestURLGeminiApiKey(t *testing.T) {
	adaptor := &Adaptor{}

	info := &relaycommon.RelayInfo{
		RequestURLPath: "/v1beta/models/gemini-pro:generateContent",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://generativelanguage.googleapis.com",
			ChannelType:    constant.ChannelTypeGemini,
			ApiKey:         "test-gemini-key",
		},
	}

	url, err := adaptor.GetRequestURL(info)
	if err != nil {
		t.Fatalf("GetRequestURL failed: %v", err)
	}

	// Should contain the path
	if !strings.Contains(url, "/v1beta/models/gemini-pro:generateContent") {
		t.Errorf("URL should contain path, got %q", url)
	}

	// Should contain API key in query parameter
	if !strings.Contains(url, "key=test-gemini-key") {
		t.Errorf("URL should contain API key in query parameter, got %q", url)
	}
}

func TestSetupRequestHeaderUsesRelayFormatClaude(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	adaptor := &Adaptor{}
	headers := http.Header{}
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOspreyAI,
			ApiKey:      "test-claude-key",
		},
	}

	err := adaptor.SetupRequestHeader(c, &headers, info)
	if err != nil {
		t.Fatalf("SetupRequestHeader failed: %v", err)
	}

	if got := headers.Get("x-api-key"); got != "test-claude-key" {
		t.Fatalf("x-api-key = %q, want %q", got, "test-claude-key")
	}
	if got := headers.Get("anthropic-version"); got != "2023-06-01" {
		t.Fatalf("anthropic-version = %q, want %q", got, "2023-06-01")
	}
	if got := headers.Get("Authorization"); got != "" {
		t.Fatalf("Authorization = %q, want empty", got)
	}
}

func TestGetRequestURLUsesRelayFormatGemini(t *testing.T) {
	adaptor := &Adaptor{}

	info := &relaycommon.RelayInfo{
		RelayFormat:    types.RelayFormatGemini,
		RequestURLPath: "/v1beta/models/gemini-pro:generateContent",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://generativelanguage.googleapis.com",
			ChannelType:    constant.ChannelTypeOspreyAI,
			ApiKey:         "test-gemini-key",
		},
	}

	url, err := adaptor.GetRequestURL(info)
	if err != nil {
		t.Fatalf("GetRequestURL failed: %v", err)
	}

	if !strings.Contains(url, "key=test-gemini-key") {
		t.Fatalf("URL should contain API key in query parameter, got %q", url)
	}
}

// TestGetChannelName verifies channel name
func TestGetChannelName(t *testing.T) {
	adaptor := &Adaptor{}
	name := adaptor.GetChannelName()

	if name != ChannelName {
		t.Errorf("GetChannelName: got %q, want %q", name, ChannelName)
	}
}

// TestGetModelList verifies model list is not empty
func TestGetModelList(t *testing.T) {
	adaptor := &Adaptor{}
	models := adaptor.GetModelList()

	if len(models) == 0 {
		t.Error("GetModelList should return non-empty list")
	}

	// Check for expected models
	hasGPT4 := false
	hasClaude := false

	for _, model := range models {
		if strings.Contains(model, "gpt-4") {
			hasGPT4 = true
		}
		if strings.Contains(model, "claude") {
			hasClaude = true
		}
	}

	if !hasGPT4 {
		t.Error("GetModelList should include GPT-4 models")
	}

	if !hasClaude {
		t.Error("GetModelList should include Claude models")
	}
}
