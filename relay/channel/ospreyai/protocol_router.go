package ospreyai

import (
	"github.com/QuantumNous/new-api/types"
)

// ProtocolRouter routes responses based on relay format
type ProtocolRouter struct {
}

// NewProtocolRouter creates a new protocol router
func NewProtocolRouter() *ProtocolRouter {
	return &ProtocolRouter{}
}

// RouteResponse determines the response handler based on format and stream status
func (pr *ProtocolRouter) RouteResponse(format types.RelayFormat, isStream bool) types.RelayFormat {
	// Phase 3: This will be used to select the appropriate handler
	// in DoResponse() method
	return format
}

// IsSupportedFormat checks if the format is supported
func (pr *ProtocolRouter) IsSupportedFormat(format types.RelayFormat) bool {
	switch format {
	case types.RelayFormatOpenAI:
		return true
	case types.RelayFormatClaude:
		return true
	case types.RelayFormatGemini:
		return true
	case types.RelayFormatOpenAIImage:
		return true
	case types.RelayFormatEmbedding:
		return true
	case types.RelayFormatOpenAIAudio:
		return true
	case types.RelayFormatRerank:
		return true
	case types.RelayFormatOpenAIResponses:
		return true
	case types.RelayFormatOpenAIResponsesCompaction:
		return true
	default:
		return false
	}
}
