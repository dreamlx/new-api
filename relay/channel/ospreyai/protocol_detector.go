package ospreyai

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// ProtocolDetector detects protocol format from request characteristics
type ProtocolDetector struct{}

// DetectFromURL detects protocol from request URL path
func (pd *ProtocolDetector) DetectFromURL(urlPath string) types.RelayFormat {
	path := strings.ToLower(urlPath)

	// Claude endpoints
	if strings.Contains(path, "/v1/messages") {
		return types.RelayFormatClaude
	}

	// OpenAI endpoints
	if strings.Contains(path, "/v1/chat/completions") {
		return types.RelayFormatOpenAI
	}

	if strings.Contains(path, "/v1/completions") {
		return types.RelayFormatOpenAI
	}

	if strings.Contains(path, "/v1/embeddings") {
		return types.RelayFormatEmbedding
	}

	if strings.Contains(path, "/v1/images/generations") {
		return types.RelayFormatOpenAIImage
	}

	if strings.Contains(path, "/v1/audio") {
		return types.RelayFormatOpenAIAudio
	}

	// Gemini endpoints
	if strings.Contains(path, "generativelanguage.googleapis.com") {
		return types.RelayFormatGemini
	}

	if strings.Contains(path, ":generateContent") {
		return types.RelayFormatGemini
	}

	if strings.Contains(path, ":embedContent") {
		return types.RelayFormatGemini
	}

	// Default to OpenAI
	return types.RelayFormatOpenAI
}

// DetectFromBody detects protocol from request body content
func (pd *ProtocolDetector) DetectFromBody(bodyData []byte) types.RelayFormat {
	if len(bodyData) == 0 {
		return types.RelayFormatOpenAI
	}

	// Parse JSON to check structure
	var body map[string]interface{}
	if err := common.Unmarshal(bodyData, &body); err != nil {
		return types.RelayFormatOpenAI
	}

	// Check for Claude-specific fields
	if _, hasMessages := body["messages"]; hasMessages {
		if _, hasModel := body["model"]; hasModel {
			if _, hasMaxTokens := body["max_tokens"]; hasMaxTokens {
				// Could be Claude
				return types.RelayFormatClaude
			}
		}
	}

	// Check for Gemini-specific fields
	if _, hasContents := body["contents"]; hasContents {
		return types.RelayFormatGemini
	}

	if _, hasText := body["text"]; hasText {
		if _, hasGenerateContentRequest := body["prompt"]; hasGenerateContentRequest {
			return types.RelayFormatGemini
		}
	}

	// Default to OpenAI
	return types.RelayFormatOpenAI
}

// DetectFromHeader detects protocol from request headers
func (pd *ProtocolDetector) DetectFromHeader(headers map[string]string) types.RelayFormat {
	// Check for protocol-specific headers
	if headers != nil {
		// Claude uses x-api-key
		if _, hasClaude := headers["x-api-key"]; hasClaude {
			return types.RelayFormatClaude
		}

		// Check for custom protocol hint header
		if protocol, ok := headers["x-protocol-hint"]; ok {
			switch strings.ToLower(protocol) {
			case "openai":
				return types.RelayFormatOpenAI
			case "claude":
				return types.RelayFormatClaude
			case "gemini":
				return types.RelayFormatGemini
			}
		}
	}

	return types.RelayFormatOpenAI
}

// DetectProtocol attempts multi-level protocol detection
// Tries URL → Body → Header → Default
func (pd *ProtocolDetector) DetectProtocol(urlPath string, bodyData []byte, headers map[string]string) types.RelayFormat {
	// Level 1: URL path detection (most reliable)
	if format := pd.DetectFromURL(urlPath); format != types.RelayFormatOpenAI {
		return format
	}

	// Level 2: Header detection
	if format := pd.DetectFromHeader(headers); format != types.RelayFormatOpenAI {
		return format
	}

	// Level 3: Body content detection
	if format := pd.DetectFromBody(bodyData); format != types.RelayFormatOpenAI {
		return format
	}

	// Default
	return types.RelayFormatOpenAI
}
