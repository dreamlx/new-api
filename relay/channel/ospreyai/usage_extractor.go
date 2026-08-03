package ospreyai

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// UsageExtractor extracts token usage information from different protocol responses
type UsageExtractor struct{}

// ExtractUsageFromOpenAI extracts usage from OpenAI format response
func (ue *UsageExtractor) ExtractUsageFromOpenAI(data []byte) *dto.Usage {
	var response dto.OpenAITextResponse
	if err := common.Unmarshal(data, &response); err != nil {
		return nil
	}

	// Check if Usage exists (assuming Usage is a value type with zero check)
	if response.Usage.TotalTokens == 0 {
		return nil
	}

	return &dto.Usage{
		PromptTokens:     response.Usage.PromptTokens,
		CompletionTokens: response.Usage.CompletionTokens,
		TotalTokens:      response.Usage.TotalTokens,
	}
}

// ExtractUsageFromClaude extracts usage from Claude format response
func (ue *UsageExtractor) ExtractUsageFromClaude(data []byte) *dto.Usage {
	var response dto.ClaudeResponse
	if err := common.Unmarshal(data, &response); err != nil {
		return nil
	}

	if response.Usage == nil {
		return nil
	}

	return &dto.Usage{
		PromptTokens:     response.Usage.InputTokens,
		CompletionTokens: response.Usage.OutputTokens,
		TotalTokens:      response.Usage.InputTokens + response.Usage.OutputTokens,
	}
}

// ExtractUsageFromGemini extracts usage from Gemini format response
func (ue *UsageExtractor) ExtractUsageFromGemini(data []byte) *dto.Usage {
	var response dto.GeminiChatResponse
	if err := common.Unmarshal(data, &response); err != nil {
		return nil
	}

	// UsageMetadata might be a value type, so just check TotalTokenCount
	if response.UsageMetadata.TotalTokenCount == 0 {
		return nil
	}

	return &dto.Usage{
		PromptTokens:     response.UsageMetadata.PromptTokenCount,
		CompletionTokens: response.UsageMetadata.CandidatesTokenCount,
		TotalTokens:      response.UsageMetadata.TotalTokenCount,
	}
}

// ExtractUsage attempts to extract usage from response based on content
// This is a fallback method when protocol is unknown
func (ue *UsageExtractor) ExtractUsage(data []byte) *dto.Usage {
	// Try OpenAI format first (most common)
	if usage := ue.ExtractUsageFromOpenAI(data); usage != nil {
		return usage
	}

	// Try Claude format
	if usage := ue.ExtractUsageFromClaude(data); usage != nil {
		return usage
	}

	// Try Gemini format
	if usage := ue.ExtractUsageFromGemini(data); usage != nil {
		return usage
	}

	// No usage information found
	return nil
}
