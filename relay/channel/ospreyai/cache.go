package ospreyai

import (
	"sync"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
)

// HeaderMappingCache caches header mappings to avoid repeated lookups
type HeaderMappingCache struct {
	mu       sync.RWMutex
	mappings map[int]*HeaderMapping
	cached   bool
}

// NewHeaderMappingCache creates a new header mapping cache
func NewHeaderMappingCache() *HeaderMappingCache {
	return &HeaderMappingCache{
		mappings: make(map[int]*HeaderMapping),
		cached:   false,
	}
}

// GetMapping returns cached or fresh header mapping
func (hmc *HeaderMappingCache) GetMapping(channelType int) *HeaderMapping {
	hmc.mu.RLock()
	if hmc.cached {
		if mapping, exists := hmc.mappings[channelType]; exists {
			hmc.mu.RUnlock()
			return mapping
		}
	}
	hmc.mu.RUnlock()

	// Not in cache, get fresh mapping
	mappings := GetHeaderMappings()
	mapping := mappings[channelType]

	// Store in cache
	hmc.mu.Lock()
	hmc.mappings[channelType] = mapping
	if len(hmc.mappings) == len(mappings) {
		hmc.cached = true
	}
	hmc.mu.Unlock()

	return mapping
}

// ProtocolRouterCache caches protocol routing decisions
type ProtocolRouterCache struct {
	mu                sync.RWMutex
	formatSupported   map[string]bool
	routerInitialized bool
}

// NewProtocolRouterCache creates a new protocol router cache
func NewProtocolRouterCache() *ProtocolRouterCache {
	return &ProtocolRouterCache{
		formatSupported:   make(map[string]bool),
		routerInitialized: false,
	}
}

// IsFormatSupported checks if format is supported (cached)
// Accepts string and converts to RelayFormat
func (prc *ProtocolRouterCache) IsFormatSupported(format string) bool {
	prc.mu.RLock()
	if supported, exists := prc.formatSupported[format]; exists {
		prc.mu.RUnlock()
		return supported
	}
	prc.mu.RUnlock()

	// Determine if format is supported
	router := NewProtocolRouter()
	// Convert string to RelayFormat with explicit cast
	relayFormat := types.RelayFormat(format)
	supported := router.IsSupportedFormat(relayFormat)

	// Cache result
	prc.mu.Lock()
	prc.formatSupported[format] = supported
	prc.mu.Unlock()

	return supported
}

// GlobalCaches provides singleton access to caches
var (
	headerMappingCache *HeaderMappingCache
	protocolRouterCache *ProtocolRouterCache
	cacheOnce sync.Once
)

// GetHeaderMappingCache returns singleton header mapping cache
func GetHeaderMappingCache() *HeaderMappingCache {
	cacheOnce.Do(func() {
		headerMappingCache = NewHeaderMappingCache()
	})
	return headerMappingCache
}

// GetProtocolRouterCache returns singleton protocol router cache
func GetProtocolRouterCache() *ProtocolRouterCache {
	cacheOnce.Do(func() {
		protocolRouterCache = NewProtocolRouterCache()
	})
	return protocolRouterCache
}

// InitializeSupportedFormats pre-populates cache with all supported formats
func InitializeSupportedFormats() {
	cache := GetProtocolRouterCache()

	supportedFormats := []string{
		"openai",
		"claude",
		"gemini",
		"image",
		"embedding",
		"audio",
		"rerank",
		"responses",
		"responses_compaction",
	}

	for _, format := range supportedFormats {
		cache.formatSupported[format] = true
	}
}

// InitializeHeaderMappings pre-populates cache with all channel types
func InitializeHeaderMappings() {
	cache := GetHeaderMappingCache()

	channelTypes := []int{
		constant.ChannelTypeOpenAI,
		constant.ChannelTypeAnthropic,
		constant.ChannelTypeGemini,
		constant.ChannelTypeAzure,
		constant.ChannelTypeAws,
	}

	for _, channelType := range channelTypes {
		cache.GetMapping(channelType)
	}
}

// InitializeCaches initializes all caches (call once at startup)
func InitializeCaches() {
	InitializeSupportedFormats()
	InitializeHeaderMappings()
}
