package ospreyai

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// Adaptor supports multi-protocol passthrough (no format conversion)
type Adaptor struct{}

func detectUpstreamProtocol(info *relaycommon.RelayInfo) UpstreamProtocol {
	if info == nil {
		return ProtocolOpenAI
	}
	if info.RelayFormat != "" {
		return ProtocolFromRelayFormat(info.RelayFormat)
	}
	return protocolFromChannelType(info.ChannelType)
}

func rewriteClaudePassthroughPath(requestPath string) string {
	if requestPath == "" {
		return "/anthropic/v1/messages"
	}
	if strings.HasPrefix(requestPath, "/anthropic/") {
		return requestPath
	}
	if strings.HasPrefix(requestPath, "/v1/") {
		return "/anthropic" + requestPath
	}
	return requestPath
}

// ─── 1. Initialization ──────────────────────────────────────────────────

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	// Initialization if needed in future
	// Usually empty for passthrough adaptor
}

// ─── 2. URL and Header ──────────────────────────────────────────────────

// GetRequestURL preserves client original request path and handles API key in query param
// This is the core of multi-protocol passthrough:
// client accesses what path, forward what path to upstream
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	protocol := detectUpstreamProtocol(info)

	// Get full URL preserving original path
	requestPath := info.RequestURLPath
	if protocol == ProtocolClaude {
		requestPath = rewriteClaudePassthroughPath(requestPath)
	}

	fullURL := relaycommon.GetFullRequestURL(
		info.ChannelBaseUrl,
		requestPath,
		info.ChannelType,
	)

	// Special handling: add API key to query parameter if needed (e.g., Gemini)
	if IsApiKeyInQueryByProtocol(protocol) {
		paramName := GetApiKeyQueryParamByProtocol(protocol)

		// Check if URL already has query parameters
		separator := "?"
		for _, char := range fullURL {
			if char == '?' {
				separator = "&"
				break
			}
		}

		fullURL += separator + paramName + "=" + info.ApiKey
	}

	return fullURL, nil
}

// SetupRequestHeader sets appropriate auth headers based on protocol
// Different upstream APIs expect different auth methods
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	protocol := detectUpstreamProtocol(info)

	// Generic header setup (sets content-type, user-agent, etc.)
	channel.SetupApiRequestHeader(info, c, req)

	// Setup protocol-specific auth header based on channel type
	SetupAuthHeaderByProtocol(req, info.ApiKey, protocol)

	// Apply runtime headers override if enabled
	// This allows dynamic header manipulation at request time
	if info.UseRuntimeHeadersOverride && info.RuntimeHeadersOverride != nil {
		for key, value := range info.RuntimeHeadersOverride {
			if strValue, ok := value.(string); ok {
				req.Set(key, strValue)
			}
		}
	}

	// Apply headers override from channel settings
	// This allows channel-specific header customization
	if info.HeadersOverride != nil {
		for key, value := range info.HeadersOverride {
			if strValue, ok := value.(string); ok {
				req.Set(key, strValue)
			}
		}
	}

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
// Supports 11+ protocol formats with streaming and non-streaming variants
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	// Record final response format for logging and billing
	info.FinalRequestRelayFormat = info.RelayFormat

	// Route response handling based on client request format (RelayFormat)
	// Each format may have stream/non-stream variants

	switch info.RelayFormat {

	// ─── OpenAI Protocol ───────────────────────────────────────────
	case types.RelayFormatOpenAI:
		if info.IsStream {
			return openai.OaiStreamHandler(c, info, resp)
		}
		return openai.OpenaiHandler(c, info, resp)

	// ─── Claude/Anthropic Protocol ────────────────────────────────
	case types.RelayFormatClaude:
		if info.IsStream {
			return claude.ClaudeStreamHandler(c, resp, info)
		}
		return claude.ClaudeHandler(c, resp, info)

	// ─── Gemini Protocol ───────────────────────────────────────────
	case types.RelayFormatGemini:
		if info.IsStream {
			return gemini.GeminiChatStreamHandler(c, info, resp)
		}
		return gemini.GeminiChatHandler(c, info, resp)

	// ─── Image Generation ─────────────────────────────────────────
	case types.RelayFormatOpenAIImage:
		return openai.OpenaiHandlerWithUsage(c, info, resp)

	// ─── Embedding ────────────────────────────────────────────────
	case types.RelayFormatEmbedding:
		return openai.OpenaiHandler(c, info, resp)

	// ─── Audio Processing ─────────────────────────────────────────
	case types.RelayFormatOpenAIAudio:
		return openai.OpenaiHandler(c, info, resp)

	// ─── Reranking ────────────────────────────────────────────────
	case types.RelayFormatRerank:
		return openai.OpenaiHandler(c, info, resp)

	// ─── OpenAI Responses Format ──────────────────────────────────
	case types.RelayFormatOpenAIResponses:
		if info.IsStream {
			return openai.OaiResponsesStreamHandler(c, info, resp)
		}
		return openai.OaiResponsesHandler(c, info, resp)

	// ─── OpenAI Responses Compaction ──────────────────────────────
	case types.RelayFormatOpenAIResponsesCompaction:
		if info.IsStream {
			return openai.OaiResponsesStreamHandler(c, info, resp)
		}
		return openai.OaiResponsesHandler(c, info, resp)

	// ─── Error Handling ───────────────────────────────────────────
	default:
		return nil, types.NewError(
			fmt.Errorf("ospreyai adaptor: unsupported relay format: %s", info.RelayFormat),
			types.ErrorCodeInvalidRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
}

// ─── 6. Metadata ────────────────────────────────────────────────────

// GetModelList returns all models supported by this channel
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName returns channel name
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
