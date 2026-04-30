package ospreyai

import (
	"net/http"

	"github.com/QuantumNous/new-api/constant"
)

// HeaderMapping defines authentication headers for different protocols
type HeaderMapping struct {
	// Protocol name
	Protocol string
	// Headers to be set
	Headers map[string]string
	// Whether API key should be in query parameter instead of header
	ApiKeyInQuery bool
	// Query parameter name for API key
	ApiKeyQueryParam string
}

// GetHeaderMappings returns all supported protocol header mappings
func GetHeaderMappings() map[int]*HeaderMapping {
	return map[int]*HeaderMapping{
		constant.ChannelTypeOpenAI: {
			Protocol: "OpenAI",
			Headers: map[string]string{
				"Authorization": "Bearer {api_key}",
			},
			ApiKeyInQuery: false,
		},
		constant.ChannelTypeAnthropic: {
			Protocol: "Claude/Anthropic",
			Headers: map[string]string{
				"x-api-key":             "{api_key}",
				"anthropic-version":     "2023-06-01",
				"anthropic-beta":        "interleaved-thinking-2025-05-14",
			},
			ApiKeyInQuery: false,
		},
		constant.ChannelTypeGemini: {
			Protocol: "Gemini",
			Headers: map[string]string{
				// API Key is placed in URL query parameter
				// No headers needed
			},
			ApiKeyInQuery: true,
			ApiKeyQueryParam: "key",
		},
		constant.ChannelTypeAzure: {
			Protocol: "Azure OpenAI",
			Headers: map[string]string{
				"api-key": "{api_key}",
			},
			ApiKeyInQuery: false,
		},
		constant.ChannelTypeAws: {
			Protocol: "AWS Bedrock",
			Headers: map[string]string{
				// AWS uses signature-based authentication
				// Headers are set during request signing
			},
			ApiKeyInQuery: false,
		},
	}
}

// SetupAuthHeader sets authentication headers based on channel type
// This is the main function called by SetupRequestHeader()
func SetupAuthHeader(req *http.Header, apiKey string, channelType int) {
	mappings := GetHeaderMappings()

	if mapping, exists := mappings[channelType]; exists {
		// Apply all headers from the mapping
		for key, value := range mapping.Headers {
			if value != "" {
				// Replace {api_key} placeholder with actual API key
				if value == "{api_key}" {
					req.Set(key, apiKey)
				} else {
					// Replace placeholder in template string
					expandedValue := replaceApiKeyPlaceholder(value, apiKey)
					req.Set(key, expandedValue)
				}
			}
		}
	} else {
		// Default: use Bearer Token for unknown types
		req.Set("Authorization", "Bearer "+apiKey)
	}
}

// replaceApiKeyPlaceholder replaces {api_key} in template strings
func replaceApiKeyPlaceholder(template string, apiKey string) string {
	if template == "{api_key}" {
		return apiKey
	}
	// Replace {api_key} in templates like "Bearer {api_key}"
	result := ""
	i := 0
	for i < len(template) {
		if i+9 <= len(template) && template[i:i+9] == "{api_key}" {
			result += apiKey
			i += 9
		} else {
			result += string(template[i])
			i++
		}
	}
	return result
}

// IsApiKeyInQuery checks if this channel type uses API key in query parameter
func IsApiKeyInQuery(channelType int) bool {
	mappings := GetHeaderMappings()
	if mapping, exists := mappings[channelType]; exists {
		return mapping.ApiKeyInQuery
	}
	return false
}

// GetApiKeyQueryParam returns the query parameter name for API key
func GetApiKeyQueryParam(channelType int) string {
	mappings := GetHeaderMappings()
	if mapping, exists := mappings[channelType]; exists {
		return mapping.ApiKeyQueryParam
	}
	return "key"
}
