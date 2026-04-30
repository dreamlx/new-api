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

### Phase 2: Authentication & Routing (⏳ Pending)
- [ ] Complete SetupRequestHeader() implementation
- [ ] Complete GetRequestURL() implementation
- [ ] Header mapping table
- [ ] URL handling for special cases

### Phase 3: Response Processing (⏳ Pending)
- [ ] Protocol router implementation
- [ ] DoResponse() switch statement for 11+ protocol cases
- [ ] Error handling
- [ ] Streaming support

### Phase 4: Advanced Features (⏳ Pending)
- [ ] Protocol detection enhancement
- [ ] Error handling standardization
- [ ] Token counting
- [ ] Performance optimization

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
