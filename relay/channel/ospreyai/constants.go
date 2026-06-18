package ospreyai

// OspreyAI channel constants
const (
	ChannelName = "OspreyAI"
)

// ModelList contains all models supported by OspreyAI adaptor
// Including models from OpenAI, Claude, Gemini, and other providers
var ModelList = []string{
	// OpenAI 模型
	"gpt-4o", "gpt-4-turbo", "gpt-4-32k", "gpt-4-8k", "gpt-4",
	"gpt-3.5-turbo-16k", "gpt-3.5-turbo",

	// Claude/Anthropic 模型
	"claude-3-5-sonnet-20241022", "claude-3-opus-20250219", "claude-3-sonnet-20240229",
	"claude-3-haiku-20240307", "claude-2.1", "claude-2",

	// Google Gemini 模型
	"gemini-2.0-flash", "gemini-1.5-pro", "gemini-1.5-flash", "gemini-pro",

	// 图像生成模型
	"dall-e-3", "dall-e-2",

	// 嵌入模型
	"text-embedding-3-large", "text-embedding-3-small", "text-embedding-ada-002",

	// 音频模型
	"whisper-1",

	// 其他模型（如果OspreyAI支持）
	"gpt-4-vision", "claude-instant",
}
