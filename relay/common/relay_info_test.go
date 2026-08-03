package common

import (
	"net/http/httptest"
	"testing"

	common2 "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
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

func TestRelayInfoMetaTypedNilReceiver(t *testing.T) {
	var info *RelayInfo
	var meta convmeta.Meta = info

	assert.Empty(t, meta.GetOriginModelName())
	assert.Empty(t, meta.GetUpstreamModelName())
	assert.False(t, meta.HasChannelMeta())
	assert.Zero(t, meta.GetChannelID())
	assert.Zero(t, meta.GetChannelType())
	assert.False(t, meta.GetIsStream())
	assert.Empty(t, meta.GetReasoningEffort())
	assert.Zero(t, meta.GetEstimatePromptTokens())
	assert.Zero(t, meta.GetSendResponseCount())

	assert.NotPanics(t, func() {
		meta.SetReasoningEffort("high")
		meta.IncrSendResponseCount()
		meta.AppendRequestConversion(types.RelayFormatClaude)
	})

	firstState := meta.EnsureClaudeConvertInfo()
	secondState := meta.EnsureClaudeConvertInfo()
	require.NotNil(t, firstState)
	require.NotNil(t, secondState)
	assert.Equal(t, convmeta.LastMessageTypeNone, firstState.LastMessagesType)
	assert.NotSame(t, firstState, secondState)

	firstOptions := meta.ConvOptions()
	secondOptions := meta.ConvOptions()
	require.NotNil(t, firstOptions)
	require.NotNil(t, secondOptions)
	assert.NotSame(t, firstOptions, secondOptions)
	assert.NotNil(t, firstOptions.Claude.DefaultMaxTokens)
	assert.NotNil(t, firstOptions.Gemini.SupportsImagine)
	assert.NotNil(t, firstOptions.Gemini.SafetySetting)
	assert.NotNil(t, firstOptions.PreserveThinkingSuffix)
}
