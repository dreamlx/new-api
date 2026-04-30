package ospreyai

import (
	"net/http"

	"github.com/QuantumNous/new-api/constant"
)

// SetupAuthHeader sets authentication headers based on channel type
func SetupAuthHeader(req *http.Header, apiKey string, channelType int) {
	switch channelType {
	case constant.ChannelTypeOpenAI:
		// OpenAI: Bearer Token
		req.Set("Authorization", "Bearer "+apiKey)

	case constant.ChannelTypeAnthropic:
		// Claude/Anthropic: API Key + Version
		req.Set("x-api-key", apiKey)
		req.Set("anthropic-version", "2023-06-01")

	case constant.ChannelTypeGemini:
		// Gemini: API Key in query param (handled in URL)
		// No header needed

	case constant.ChannelTypeAzure:
		// Azure: API Key in header
		req.Set("api-key", apiKey)

	case constant.ChannelTypeAws:
		// AWS: Signature-based (not set here)
		// No header needed

	default:
		// Default: use Bearer Token
		req.Set("Authorization", "Bearer "+apiKey)
	}
}
