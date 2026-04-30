package ospreyai

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// Adaptor supports multi-protocol passthrough (no format conversion)
type Adaptor struct{}

// ─── 1. Initialization ──────────────────────────────────────────────────

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	// Initialization if needed in future
	// Usually empty for passthrough adaptor
}

// ─── 2. URL and Header ──────────────────────────────────────────────────

// GetRequestURL preserves client original request path
// This is the core of multi-protocol passthrough:
// client accesses what path, forward what path to upstream
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return relaycommon.GetFullRequestURL(
		info.ChannelBaseUrl,
		info.RequestURLPath, // ← Preserve original path
		info.ChannelType,
	), nil
}

// SetupRequestHeader sets appropriate auth headers based on protocol
// Different upstream APIs expect different auth methods
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	// Generic header setup
	channel.SetupApiRequestHeader(info, c, req)

	// Setup auth header based on channel type
	SetupAuthHeader(req, info.ApiKey, info.ChannelType)

	return nil
}

// ─── 3. Request Conversion (8 methods) ──────────────────────────────────
// In passthrough mode, these are not called
// But they must be implemented to satisfy the Adaptor interface

// ConvertOpenAIRequest returns error in passthrough mode
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("ospreyai adaptor: only supports pass-through mode, not format conversion")
}

// ConvertClaudeRequest returns error in passthrough mode
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("ospreyai adaptor: only supports pass-through mode, not format conversion")
}

// ConvertGeminiRequest returns error in passthrough mode
func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("ospreyai adaptor: only supports pass-through mode, not format conversion")
}

// ConvertAudioRequest returns error in passthrough mode
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("ospreyai adaptor: only supports pass-through mode, not format conversion")
}

// ConvertImageRequest returns error in passthrough mode
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, errors.New("ospreyai adaptor: only supports pass-through mode, not format conversion")
}

// ConvertEmbeddingRequest returns error in passthrough mode
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("ospreyai adaptor: only supports pass-through mode, not format conversion")
}

// ConvertRerankRequest returns error in passthrough mode
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("ospreyai adaptor: only supports pass-through mode, not format conversion")
}

// ConvertOpenAIResponsesRequest returns error in passthrough mode
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("ospreyai adaptor: only supports pass-through mode, not format conversion")
}

// ─── 4. Request Execution ──────────────────────────────────────────────

// DoRequest executes HTTP request using standard implementation
// Calls GetRequestURL() and SetupRequestHeader()
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// ─── 5. Response Handling (Most Critical!) ─────────────────────────────

// DoResponse routes response based on relay format and calls appropriate handler
// This is the most complex part of multi-protocol adaptor
// To be implemented in Phase 3: Response Processing
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	// Phase 3: Implement protocol routing with switch statement
	// For now, return error indicating not yet implemented
	return nil, types.NewError(
		fmt.Errorf("ospreyai adaptor: DoResponse not yet implemented (Phase 3)"),
		types.ErrorCodeInvalidRequest,
		types.ErrOptionWithSkipRetry(),
	)
}

// ─── 6. Metadata ────────────────────────────────────────────────────

// GetModelList returns all models supported by this channel
// Note: Should be populated based on actual channel configuration
func (a *Adaptor) GetModelList() []string {
	return []string{
		// OpenAI models
		"gpt-4o", "gpt-4-turbo", "gpt-4", "gpt-3.5-turbo",
		// Claude models
		"claude-3-5-sonnet-20241022", "claude-3-opus-20250219", "claude-3-sonnet-20240229",
		// More models can be added based on actual support
	}
}

// GetChannelName returns channel name
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
