package common

import (
	"net/http/httptest"
	"testing"

	common2 "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"

	"github.com/gin-gonic/gin"
)

func TestRelayInfoGetFinalRequestRelayFormatPrefersExplicitFinal(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToConversionChain(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToRelayFormat(t *testing.T) {
	info := &RelayInfo{
		RelayFormat: types.RelayFormatGemini,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatGemini), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatNilReceiver(t *testing.T) {
	var info *RelayInfo
	require.Equal(t, types.RelayFormat(""), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoInitChannelMetaEnablesStreamOptionsForOspreyAI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	common2.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOspreyAI)
	common2.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://example.com")
	common2.SetContextKey(c, constant.ContextKeyChannelKey, "test-key")

	info := &RelayInfo{}
	info.InitChannelMeta(c)

	require.NotNil(t, info.ChannelMeta)
	require.True(t, info.SupportStreamOptions)
}

// TestRelayInfoInitChannelMetaEnablesStreamOptionsForOpenRouter pins issue #18:
// OpenRouter must be in streamSupportedChannels so include_usage is forwarded
// to upstream and the native final-usage-chunk path (HandleFinalResponse) can
// activate for OpenRouter-backed streamed /chat/completions.
func TestRelayInfoInitChannelMetaEnablesStreamOptionsForOpenRouter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	common2.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenRouter)
	common2.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://openrouter.ai")
	common2.SetContextKey(c, constant.ContextKeyChannelKey, "test-key")

	info := &RelayInfo{}
	info.InitChannelMeta(c)

	require.NotNil(t, info.ChannelMeta)
	require.True(t, info.SupportStreamOptions, "OpenRouter must support stream options (issue #18)")
}
