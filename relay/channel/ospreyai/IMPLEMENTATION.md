# OspreyAI Adaptor Implementation Guide

## Architecture Overview

The OspreyAI Adaptor is a comprehensive multi-protocol transparent relay system that enables new-api to seamlessly interface with OspreyAI, which supports multiple API formats (OpenAI, Claude, Gemini, etc.) without format conversion.

### Core Design Principles

1. **Transparency**: Pass through client requests exactly as-is to upstream
2. **Protocol Agnosticism**: Support multiple protocols without conversion
3. **Performance**: Cache frequently accessed mappings and routing decisions
4. **Maintainability**: Modular architecture with clear separation of concerns
5. **Extensibility**: Easy to add new protocols or error handling strategies

## Module Breakdown

### 1. adaptor.go - Main Adaptor Implementation

**Primary Responsibilities:**
- Implement Adaptor interface required by relay framework
- Route requests to upstream via `DoRequest()`
- Dispatch responses to appropriate handlers via `DoResponse()`

**Key Methods:**

```go
// GetRequestURL() - Core passthrough logic
// Preserves client original request path
// Special handling for protocols with API key in query (Gemini)

// SetupRequestHeader() - Authentication header setup
// Routes to protocol-specific header setup
// Supports runtime and channel-level overrides

// DoResponse() - Response routing (11+ protocols)
// Dispatches to openai.*, claude.*, gemini.* handlers
// Handles both streaming and non-streaming responses

// GetModelList() - Returns supported models
// Used for model availability checking

// GetChannelName() - Returns "OspreyAI"
```

**Interface Compliance:**
- All 8 ConvertXxx methods return errors (not used in passthrough mode)
- Implements `channel.Adaptor` interface fully

### 2. header_mapping.go - Authentication Header Management

**Responsibilities:**
- Define header requirements for each protocol
- Map channel types to authentication methods
- Replace API key placeholders in header templates

**Data Structure:**

```go
type HeaderMapping struct {
    Protocol          string            // e.g., "OpenAI"
    Headers           map[string]string // Header name → template
    ApiKeyInQuery     bool              // Is API key in URL query?
    ApiKeyQueryParam  string            // Query param name
}
```

**Supported Protocols:**

| Protocol | Auth Method | Header |
|----------|-------------|--------|
| OpenAI | Bearer Token | `Authorization: Bearer {api_key}` |
| Claude | API Key + Version | `x-api-key: {api_key}`, `anthropic-version: 2023-06-01` |
| Gemini | Query Parameter | `key={api_key}` (in URL) |
| Azure | API Key Header | `api-key: {api_key}` |
| AWS | Signature-based | (handled separately) |

**Key Functions:**

```go
// GetHeaderMappings() - Returns all protocol mappings
// SetupAuthHeader() - Apply headers to request
// IsApiKeyInQuery() - Check if protocol uses query param API key
// GetApiKeyQueryParam() - Get query param name for API key
// replaceApiKeyPlaceholder() - Replace {api_key} in templates
```

### 3. protocol_router.go - Response Format Routing

**Responsibilities:**
- Define which response formats are supported
- Enable response handler selection

**Supported Response Formats (11+):**

1. `RelayFormatOpenAI` - OpenAI chat completions
2. `RelayFormatClaude` - Claude/Anthropic messages
3. `RelayFormatGemini` - Google Gemini
4. `RelayFormatOpenAIImage` - DALL-E image generation
5. `RelayFormatEmbedding` - Text embeddings
6. `RelayFormatOpenAIAudio` - Audio processing
7. `RelayFormatRerank` - Reranking
8. `RelayFormatOpenAIResponses` - OpenAI Responses format
9. `RelayFormatOpenAIResponsesCompaction` - Compressed responses
10. Additional: Streaming variants for applicable formats

**Key Methods:**

```go
// NewProtocolRouter() - Create router instance
// RouteResponse() - Select handler for format
// IsSupportedFormat() - Check format support
```

### 4. error_handler.go - Error Normalization

**Responsibilities:**
- Convert different error formats to standard new-api errors
- Map HTTP status codes to error codes
- Determine retry eligibility

**Error Code Mapping:**

| HTTP Status | Error Code | Retryable |
|-------------|-----------|-----------|
| 400 | InvalidRequest | No |
| 401 | InvalidRequest | No |
| 403 | InsufficientUserQuota | No |
| 404 | InvalidRequest | No |
| 429 | InvalidRequest | Yes |
| 500-503 | BadResponseStatusCode | Yes |

**Key Methods:**

```go
// HandleError() - Create standardized error
// parseErrorMessage() - Extract error details from body
// mapStatusToErrorCode() - HTTP status → error code
// IsRetryableError() - Determine if should retry
```

### 5. usage_extractor.go - Token Usage Extraction

**Responsibilities:**
- Extract token usage from different protocol responses
- Support three major API formats
- Provide protocol-agnostic fallback

**Token Count Fields:**

```go
type Usage struct {
    PromptTokens     int // Input tokens
    CompletionTokens int // Output tokens
    TotalTokens      int // Sum
}
```

**Extraction Methods:**

| Protocol | Prompt Field | Completion Field | Total Field |
|----------|--------------|------------------|------------|
| OpenAI | `usage.prompt_tokens` | `usage.completion_tokens` | `usage.total_tokens` |
| Claude | `usage.input_tokens` | `usage.output_tokens` | Calculated |
| Gemini | `usage_metadata.prompt_token_count` | `usage_metadata.candidates_token_count` | `usage_metadata.total_token_count` |

**Key Methods:**

```go
// ExtractUsageFromOpenAI() - Parse OpenAI responses
// ExtractUsageFromClaude() - Parse Claude responses
// ExtractUsageFromGemini() - Parse Gemini responses
// ExtractUsage() - Protocol-agnostic fallback
```

### 6. protocol_detector.go - Smart Protocol Detection

**Responsibilities:**
- Detect protocol from request characteristics
- Support multiple detection strategies
- Provide intelligent fallback routing

**Detection Strategies (in priority order):**

1. **URL Path Detection** - Analyze endpoint path
   - `/v1/messages` → Claude
   - `/v1/chat/completions` → OpenAI
   - `:generateContent` → Gemini

2. **Header Detection** - Check protocol-specific headers
   - `x-api-key` → Claude
   - `x-protocol-hint` → Explicit protocol specification

3. **Body Detection** - Analyze JSON structure
   - `messages + max_tokens` → Claude
   - `contents` → Gemini
   - Default → OpenAI

**Key Methods:**

```go
// DetectFromURL() - URL-based detection
// DetectFromBody() - Content-based detection
// DetectFromHeader() - Header-based detection
// DetectProtocol() - Multi-level detection with fallback
```

### 7. cache.go - Performance Optimization

**Responsibilities:**
- Cache header mappings to avoid repeated lookups
- Cache protocol routing decisions
- Provide thread-safe singleton access

**Cache Types:**

1. **HeaderMappingCache**
   - Caches channel type → header mapping
   - Singleton pattern
   - Thread-safe with RWMutex

2. **ProtocolRouterCache**
   - Caches format support decisions
   - Pre-loads all supported formats at startup
   - Minimizes reflection overhead

**Performance Impact:**
- Header lookup: O(1) cached vs O(n) map search
- Protocol detection: Eliminates repeated struct allocations
- Pre-initialization reduces first-request latency

**Key Functions:**

```go
// GetHeaderMappingCache() - Access header cache singleton
// GetProtocolRouterCache() - Access router cache singleton
// InitializeCaches() - Pre-populate all caches at startup
// InitializeHeaderMappings() - Load all channel types
// InitializeSupportedFormats() - Load all formats
```

### 8. constants.go - Configuration

**Responsibilities:**
- Define channel name constant

```go
const ChannelName = "OspreyAI"
```

## Usage Examples

### Configuration

```json
{
  "name": "OspreyAI Channel",
  "type": "ospreyai",
  "base_url": "https://api.ospreyai.com",
  "api_key": "your_api_key_here",
  "support_multiple_protocols": true,
  "supported_protocols": [
    "/v1/messages",
    "/v1/chat/completions",
    "/v1/embeddings"
  ]
}
```

### Request Flow

```
Client Request
  ↓
Router selects OspreyAI Adaptor
  ↓
Adaptor.GetRequestURL()
  - Preserve original path: /v1/messages
  - Handle API key in query if needed
  ↓
Adaptor.SetupRequestHeader()
  - Get from HeaderMappingCache
  - Apply protocol-specific auth
  ↓
Adaptor.DoRequest()
  - HTTP POST to upstream with preserved path
  ↓
Upstream Response (same protocol as request)
  ↓
Adaptor.DoResponse()
  - Match RelayFormat to select handler
  - Call openai.*, claude.*, or gemini.* handler
  ↓
Client receives response in same protocol
```

### Error Handling Flow

```
Upstream returns error response
  ↓
ErrorHandler.HandleError()
  - Parse status code
  - Map to error code
  - Create NewAPIError
  ↓
Check IsRetryableError()
  - 429, 5xx → can retry
  - 4xx → skip retry
  ↓
Return to client
```

### Token Counting Flow

```
Upstream returns response
  ↓
UsageExtractor.ExtractUsage()
  - Try OpenAI format first
  - Fall back to Claude
  - Fall back to Gemini
  ↓
Return Usage{PromptTokens, CompletionTokens, TotalTokens}
  ↓
Billing system uses for quota deduction
```

## Testing Strategy

### Unit Tests (adaptor_test.go)

- Header setup for each protocol
- URL path preservation
- API key query parameter handling
- Channel name retrieval
- Model list generation

### Integration Tests (to be added)

- Full request-response cycle per protocol
- Streaming response handling
- Error response handling
- Token counting accuracy

### End-to-End Tests (to be added)

- Claude client → OspreyAI → Anthropic
- OpenAI client → OspreyAI → OpenAI
- Gemini client → OspreyAI → Google Gemini

## Performance Characteristics

### Time Complexity

- `GetRequestURL()`: O(n) where n = URL length (string scan)
- `SetupRequestHeader()`: O(m) where m = number of headers (map iteration)
- `DoResponse()`: O(1) (switch statement)
- Header lookup (cached): O(1)
- Protocol detection (multi-level): O(n + m + p) fallback

### Space Complexity

- Header mappings: O(p) where p = number of protocols (5-10)
- Supported formats: O(f) where f = number of formats (10+)
- Per-request overhead: O(1) additional structs

### Optimization Opportunities

1. Pre-initialize caches at channel creation
2. Profile header mapping frequency
3. Consider request body caching for retry scenarios
4. Parallel detection strategies if latency-sensitive

## Security Considerations

1. **API Key Handling**
   - API keys in headers only (except Gemini URL param)
   - No logging of API keys
   - Use standard new-api key management

2. **Request/Response Integrity**
   - Preserve original formats (no modification)
   - Transparent passthrough reduces transformation bugs
   - Minimal attack surface (no custom parsing)

3. **Error Information Leakage**
   - Mask sensitive error details
   - Use standard error codes
   - Follow new-api error handling conventions

## Future Enhancements

1. **Protocol Additions**
   - Add more upstream protocols
   - Extend header_mapping.go with new mappings
   - Add detection patterns to protocol_detector.go

2. **Error Handling**
   - Per-protocol error response parsers
   - Custom error transformation
   - Rate limit header extraction

3. **Performance**
   - Request/response compression
   - Connection pooling
   - Circuit breaker pattern

4. **Monitoring**
   - Per-protocol metrics
   - Error rate tracking
   - Latency profiling by format

## Maintenance Notes

- Keep header mappings updated when upstream APIs change
- Add tests when new protocols are supported
- Monitor error handling for new error types
- Review caching effectiveness periodically
