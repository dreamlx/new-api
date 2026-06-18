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
// Accepts RelayFormat type (which is string)
func (pr *ProtocolRouter) IsSupportedFormat(format types.RelayFormat) bool {
	supportedFormats := map[types.RelayFormat]bool{
		types.RelayFormatOpenAI:                   true,
		types.RelayFormatClaude:                   true,
		types.RelayFormatGemini:                   true,
		types.RelayFormatOpenAIImage:              true,
		types.RelayFormatEmbedding:                true,
		types.RelayFormatOpenAIAudio:              true,
		types.RelayFormatRerank:                   true,
		types.RelayFormatOpenAIResponses:          true,
		types.RelayFormatOpenAIResponsesCompaction: true,
	}

	return supportedFormats[format]
}
