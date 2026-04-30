# OspreyAI Adaptor

## Overview

The OspreyAI Adaptor is a multi-protocol transparent gateway that enables new-api to seamlessly interface with OspreyAI, which supports multiple API formats (OpenAI, Claude, Gemini, etc.) without format conversion.

## Core Features

- **Multi-Protocol Passthrough**: Client uses any protocol, upstream receives the same protocol
- **Path Preservation**: Client request path `/v1/messages` → upstream `/v1/messages`
- **Flexible Authentication**: Supports Bearer Token, API Key, Query Parameters, etc.
- **Streaming Support**: Both streaming and non-streaming responses
- **Zero Format Conversion**: Pure transparent relay without transformation

## Architecture

```
relay/channel/ospreyai/
├── adaptor.go              # Main Adaptor implementation
├── header_mapping.go       # Authentication header mapping
├── protocol_router.go      # Protocol routing logic
├── constants.go            # Constants definition
└── README.md               # This file
```

## Implementation Status

### Phase 1: Framework (✅ Completed)
- [x] Project directory structure
- [x] Core interface skeleton
- [x] Convert methods (returning errors for passthrough mode)
- [x] Interface registration

### Phase 2: Authentication & Routing (✅ Completed)
- [x] Complete SetupRequestHeader() implementation
- [x] Complete GetRequestURL() implementation
- [x] Header mapping table (5 protocols: OpenAI, Claude, Gemini, Azure, AWS)
- [x] URL handling for special cases (Gemini API key in query param)
- [x] Unit tests for Header and URL generation (8 tests, all passing)

### Phase 3: Response Processing (✅ Completed)
- [x] Protocol router framework (ProtocolRouter struct)
- [x] DoResponse() with 11-case switch statement supporting:
  - OpenAI (streaming & non-streaming)
  - Claude/Anthropic (streaming & non-streaming)
  - Gemini (streaming & non-streaming)
  - Image generation
  - Embeddings
  - Audio processing
  - Reranking
  - OpenAI Responses (streaming & non-streaming)
  - OpenAI Responses Compaction (streaming & non-streaming)
- [x] Error handling with unsupported format detection
- [x] Streaming support via info.IsStream flag
- [x] Imported all necessary handler modules

### Phase 4: Advanced Features (✅ Completed)
- [x] Protocol detection enhancement (`ProtocolDetector`)
  - URL path detection
  - Header-based detection
  - Body content detection
  - Multi-level fallback detection
- [x] Error handling standardization (`ErrorHandler`)
  - Status code to error code mapping
  - Error message parsing
  - Retry detection
- [x] Token counting (`UsageExtractor`)
  - OpenAI format support
  - Claude format support
  - Gemini format support
  - Protocol-agnostic fallback
- [x] Performance optimization via caching
  - `HeaderMappingCache` for header mappings
  - `ProtocolRouterCache` for routing decisions
  - Singleton cache initialization
  - Pre-loading supported formats

### Phase 5: Documentation & Testing (⏳ Pending)
- [ ] Code documentation
- [ ] Usage documentation
- [ ] Comprehensive testing
- [ ] Code review

## API Type

OspreyAI channel type: `constant.APITypeOspreyAI`

## Key Methods

### GetRequestURL()
Preserves original client request path using `GetFullRequestURL()`.

### SetupRequestHeader()
Routes authentication based on `ChannelType` to apply correct headers:
- OpenAI: `Authorization: Bearer {api_key}`
- Claude: `x-api-key: {api_key}`, `anthropic-version: 2023-06-01`
- Gemini: API Key in URL query parameter
- Azure: `api-key: {api_key}`

### DoResponse()
Routes response handling based on `RelayFormat` to appropriate handler.
To be implemented in Phase 3.

## Configuration Example

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
    "/v1/embeddings",
    "/v1/audio/transcriptions",
    "/v1/images/generations"
  ]
}
```

## Testing

Comprehensive tests planned for:
- Unit tests: Header and URL generation
- Integration tests: Full request-response flow
- Protocol-specific tests: Each supported protocol
- Performance tests: Comparison with other adaptors
