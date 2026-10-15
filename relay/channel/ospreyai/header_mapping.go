package ospreyai

import (
	"net/http"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
)

type UpstreamProtocol string

const (
	ProtocolOpenAI UpstreamProtocol = "openai"
	ProtocolClaude UpstreamProtocol = "claude"
	ProtocolGemini UpstreamProtocol = "gemini"
	ProtocolAzure  UpstreamProtocol = "azure"
	ProtocolAWS    UpstreamProtocol = "aws"
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
func GetHeaderMappings() map[UpstreamProtocol]*HeaderMapping {
	return map[UpstreamProtocol]*HeaderMapping{
		ProtocolOpenAI: {
			Protocol: "OpenAI",
			Headers: map[string]string{
				"Authorization": "Bearer {api_key}",
			},
			ApiKeyInQuery: false,
		},
		ProtocolClaude: {
			Protocol: "Claude/Anthropic",
			Headers: map[string]string{
				"x-api-key":         "{api_key}",
				"anthropic-version": "2023-06-01",
				"anthropic-beta":    "interleaved-thinking-2025-05-14",
			},
			ApiKeyInQuery: false,
		},
		ProtocolGemini: {
			Protocol: "Gemini",
			Headers:  map[string]string{
				// API Key is placed in URL query parameter
				// No headers needed
			},
			ApiKeyInQuery:    true,
			ApiKeyQueryParam: "key",
		},
		ProtocolAzure: {
			Protocol: "Azure OpenAI",
			Headers: map[string]string{
				"api-key": "{api_key}",
			},
			ApiKeyInQuery: false,
		},
		ProtocolAWS: {
			Protocol: "AWS Bedrock",
			Headers:  map[string]string{
				// AWS uses signature-based authentication
				// Headers are set during request signing
			},
			ApiKeyInQuery: false,
		},
	}
}

func protocolFromChannelType(channelType int) UpstreamProtocol {
	switch channelType {
	case constant.ChannelTypeAnthropic:
		return ProtocolClaude
	case constant.ChannelTypeGemini:
		return ProtocolGemini
	case constant.ChannelTypeAzure:
		return ProtocolAzure
	case constant.ChannelTypeAws:
		return ProtocolAWS
	case constant.ChannelTypeOpenAI:
		return ProtocolOpenAI
	default:
		return ProtocolOpenAI
	}
}

func ProtocolFromRelayFormat(relayFormat types.RelayFormat) UpstreamProtocol {
	switch relayFormat {
	case types.RelayFormatClaude:
		return ProtocolClaude
	case types.RelayFormatGemini:
		return ProtocolGemini
	default:
		return ProtocolOpenAI
	}
}

// SetupAuthHeader sets authentication headers based on channel type
// This is the main function called by SetupRequestHeader()
func SetupAuthHeader(req *http.Header, apiKey string, channelType int) {
	SetupAuthHeaderByProtocol(req, apiKey, protocolFromChannelType(channelType))
}

func SetupAuthHeaderByProtocol(req *http.Header, apiKey string, protocol UpstreamProtocol) {
	mappings := GetHeaderMappings()

	if mapping, exists := mappings[protocol]; exists {
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
	return IsApiKeyInQueryByProtocol(protocolFromChannelType(channelType))
}

func IsApiKeyInQueryByProtocol(protocol UpstreamProtocol) bool {
	mappings := GetHeaderMappings()
	if mapping, exists := mappings[protocol]; exists {
		return mapping.ApiKeyInQuery
	}
	return false
}

// GetApiKeyQueryParam returns the query parameter name for API key
func GetApiKeyQueryParam(channelType int) string {
	return GetApiKeyQueryParamByProtocol(protocolFromChannelType(channelType))
}

func GetApiKeyQueryParamByProtocol(protocol UpstreamProtocol) string {
	mappings := GetHeaderMappings()
	if mapping, exists := mappings[protocol]; exists {
		return mapping.ApiKeyQueryParam
	}
	return "key"
}
