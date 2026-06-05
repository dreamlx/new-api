package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

// Some OpenAI-compatible upstreams (observed: exchangetoken) return non-standard
// usage where completion_tokens EXCLUDES reasoning_tokens and
// total_tokens = prompt + completion + reasoning. Without normalization the
// reasoning portion is silently dropped from billing.
// Invariant we enforce: billable output = total - prompt.

func TestApplyUsagePostProcessing_FoldsReasoningWhenTotalExceedsPromptPlusCompletion(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenAI,
		},
	}
	usage := &dto.Usage{
		PromptTokens:     17,
		CompletionTokens: 330,
		TotalTokens:      1468,
	}

	applyUsagePostProcessing(info, usage, nil)

	require.Equal(t, 1451, usage.CompletionTokens, "completion should be folded to total-prompt (330 + 1121 reasoning)")
	require.Equal(t, 17, usage.PromptTokens)
	require.Equal(t, 1468, usage.TotalTokens)
}

func TestApplyUsagePostProcessing_NoopForStandardUsage(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenAI,
		},
	}
	usage := &dto.Usage{
		PromptTokens:     10,
		CompletionTokens: 20,
		TotalTokens:      30,
	}

	applyUsagePostProcessing(info, usage, nil)

	require.Equal(t, 20, usage.CompletionTokens, "standard usage (total == prompt+completion) must not be altered")
}

func TestApplyUsagePostProcessing_NoopWhenTotalIsZero(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenAI,
		},
	}
	usage := &dto.Usage{
		PromptTokens:     10,
		CompletionTokens: 20,
		TotalTokens:      0,
	}

	applyUsagePostProcessing(info, usage, nil)

	require.Equal(t, 20, usage.CompletionTokens, "missing/zero total_tokens must not trigger normalization")
}
