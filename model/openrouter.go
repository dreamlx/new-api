package model

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"one-api/common"

	"gopkg.in/yaml.v3"
)

// ==================== OpenRouter 数据结构 ====================

// OpenRouterModel OpenRouter格式的模型信息
type OpenRouterModel struct {
	ID                  string                 `json:"id"`
	CanonicalSlug       string                 `json:"canonical_slug"`
	HuggingFaceID       *string                `json:"hugging_face_id"`
	Name                string                 `json:"name"`
	Created             int64                  `json:"created"`
	Description         string                 `json:"description"`
	ContextLength       int                    `json:"context_length"`
	Architecture        ModelArchitecture      `json:"architecture"`
	Pricing             ModelPricing           `json:"pricing"`
	TopProvider         ModelTopProvider       `json:"top_provider"`
	PerRequestLimits    interface{}            `json:"per_request_limits"`
	SupportedParameters []string               `json:"supported_parameters"`
	DefaultParameters   map[string]interface{} `json:"default_parameters"`
}

// ModelArchitecture 模型架构信息
type ModelArchitecture struct {
	Modality         string   `json:"modality" yaml:"modality"`
	InputModalities  []string `json:"input_modalities" yaml:"input_modalities"`
	OutputModalities []string `json:"output_modalities" yaml:"output_modalities"`
	Tokenizer        string   `json:"tokenizer" yaml:"tokenizer"`
	InstructType     *string  `json:"instruct_type" yaml:"instruct_type"`
}

// ModelPricing 模型计费信息
type ModelPricing struct {
	Prompt            string `json:"prompt" yaml:"prompt"`
	Completion        string `json:"completion" yaml:"completion"`
	Request           string `json:"request" yaml:"request"`
	Image             string `json:"image" yaml:"image"`
	WebSearch         string `json:"web_search" yaml:"web_search"`
	InternalReasoning string `json:"internal_reasoning" yaml:"internal_reasoning"`
}

// ModelTopProvider 提供方能力信息
type ModelTopProvider struct {
	ContextLength       int   `json:"context_length" yaml:"context_length"`
	MaxCompletionTokens *int  `json:"max_completion_tokens" yaml:"max_completion_tokens"`
	IsModerated         bool  `json:"is_moderated" yaml:"is_moderated"`
}

// ==================== 配置结构 ====================

// OpenRouterConfig OpenRouter全局配置
type OpenRouterConfig struct {
	Enabled  bool                              `yaml:"enabled"`
	CacheTTL int                               `yaml:"cache_ttl"` // 秒
	Fallback OpenRouterFallbackConfig          `yaml:"fallback"`
	Models   map[string]OpenRouterModelConfig  `yaml:"models"`
}

// OpenRouterFallbackConfig 默认值配置
type OpenRouterFallbackConfig struct {
	UseDefaults          bool              `yaml:"use_defaults"`
	DefaultContextLength int               `yaml:"default_context_length"`
	DefaultPricing       ModelPricing      `yaml:"default_pricing"`
	DefaultArchitecture  ModelArchitecture `yaml:"default_architecture"`
}

// OpenRouterModelConfig 单个模型的配置
type OpenRouterModelConfig struct {
	CanonicalSlug       string                 `yaml:"canonical_slug"`
	HuggingFaceID       *string                `yaml:"hugging_face_id"`
	Name                string                 `yaml:"name"`
	Description         string                 `yaml:"description"`
	ContextLength       int                    `yaml:"context_length"`
	Architecture        ModelArchitecture      `yaml:"architecture"`
	Pricing             ModelPricing           `yaml:"pricing"`
	TopProvider         ModelTopProvider       `yaml:"top_provider"`
	SupportedParameters []string               `yaml:"supported_parameters"`
	DefaultParameters   map[string]interface{} `yaml:"default_parameters"`
}

// ==================== 缓存实现 ====================

// OpenRouterCache OpenRouter模型缓存
type OpenRouterCache struct {
	models         map[string]*OpenRouterModel
	modelList      []*OpenRouterModel
	lastUpdateTime time.Time
	mutex          sync.RWMutex
	ttl            time.Duration
	config         *OpenRouterConfig
	configPath     string
}

var (
	openRouterCache *OpenRouterCache
	cacheOnce       sync.Once
)

// GetOpenRouterCache 获取OpenRouter缓存单例
func GetOpenRouterCache() *OpenRouterCache {
	cacheOnce.Do(func() {
		configPath := os.Getenv("OPENROUTER_CONFIG_PATH")
		if configPath == "" {
			configPath = filepath.Join("config", "openrouter.yaml")
		}

		openRouterCache = &OpenRouterCache{
			models:     make(map[string]*OpenRouterModel),
			modelList:  make([]*OpenRouterModel, 0),
			ttl:        5 * time.Minute, // 默认5分钟
			configPath: configPath,
		}

		// 初始化加载配置
		if err := openRouterCache.loadConfig(); err != nil {
			common.SysLog("OpenRouter: Failed to load config: " + err.Error())
		}

		// 初始化刷新缓存
		openRouterCache.refreshCache()
	})
	return openRouterCache
}

// GetAllModels 获取所有OpenRouter格式的模型
func (c *OpenRouterCache) GetAllModels() []*OpenRouterModel {
	c.mutex.RLock()
	expired := time.Since(c.lastUpdateTime) > c.ttl
	models := c.modelList
	c.mutex.RUnlock()

	if expired {
		go c.refreshCache() // 异步刷新，不阻塞
	}

	return models
}

// GetModel 获取单个模型
func (c *OpenRouterCache) GetModel(modelName string) (*OpenRouterModel, bool) {
	c.mutex.RLock()
	expired := time.Since(c.lastUpdateTime) > c.ttl
	model, exists := c.models[modelName]
	c.mutex.RUnlock()

	if expired {
		go c.refreshCache()
	}

	return model, exists
}

// GetConfig 获取当前配置
func (c *OpenRouterCache) GetConfig() *OpenRouterConfig {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.config
}

// IsEnabled 检查OpenRouter功能是否启用
func (c *OpenRouterCache) IsEnabled() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.config != nil && c.config.Enabled
}

// ReloadConfig 热重载配置
func (c *OpenRouterCache) ReloadConfig() error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.refreshCache()
	common.SysLog("OpenRouter: Config reloaded successfully")
	return nil
}

// UpdateConfig 更新配置并保存
func (c *OpenRouterCache) UpdateConfig(newConfig *OpenRouterConfig) error {
	c.mutex.Lock()
	c.config = newConfig
	c.mutex.Unlock()

	// 保存到文件
	data, err := yaml.Marshal(newConfig)
	if err != nil {
		return err
	}

	if err := os.WriteFile(c.configPath, data, 0644); err != nil {
		return err
	}

	// 刷新缓存
	c.refreshCache()
	common.SysLog("OpenRouter: Config updated and saved")
	return nil
}

// ==================== 内部方法 ====================

// loadConfig 从文件加载配置
func (c *OpenRouterCache) loadConfig() error {
	data, err := os.ReadFile(c.configPath)
	if err != nil {
		// 配置文件不存在时使用默认配置
		c.mutex.Lock()
		c.config = c.getDefaultConfig()
		c.mutex.Unlock()
		common.SysLog("OpenRouter: Using default config (file not found: " + c.configPath + ")")
		return nil
	}

	var config OpenRouterConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}

	c.mutex.Lock()
	c.config = &config
	if config.CacheTTL > 0 {
		c.ttl = time.Duration(config.CacheTTL) * time.Second
	}
	c.mutex.Unlock()

	common.SysLog("OpenRouter: Config loaded from " + c.configPath)
	return nil
}

// getDefaultConfig 获取默认配置
func (c *OpenRouterCache) getDefaultConfig() *OpenRouterConfig {
	return &OpenRouterConfig{
		Enabled:  true,
		CacheTTL: 300,
		Fallback: OpenRouterFallbackConfig{
			UseDefaults:          true,
			DefaultContextLength: 8192,
			DefaultPricing: ModelPricing{
				Prompt:            "0.00001",
				Completion:        "0.00002",
				Request:           "0",
				Image:             "0",
				WebSearch:         "0",
				InternalReasoning: "0",
			},
			DefaultArchitecture: ModelArchitecture{
				Modality:         "text->text",
				InputModalities:  []string{"text"},
				OutputModalities: []string{"text"},
				Tokenizer:        "Unknown",
				InstructType:     nil,
			},
		},
		Models: make(map[string]OpenRouterModelConfig),
	}
}

// refreshCache 刷新缓存
func (c *OpenRouterCache) refreshCache() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// 检查是否刚刚刷新过（避免重复刷新）
	if time.Since(c.lastUpdateTime) < time.Second {
		return
	}

	newModels := make(map[string]*OpenRouterModel)
	newModelList := make([]*OpenRouterModel, 0)

	// 1. 从数据库获取启用的模型列表
	enabledModels := c.getEnabledModelsFromDB()

	// 2. 为每个启用的模型生成OpenRouter格式数据
	for _, modelName := range enabledModels {
		openRouterModel := c.buildOpenRouterModel(modelName)
		newModels[modelName] = openRouterModel
		newModelList = append(newModelList, openRouterModel)
	}

	c.models = newModels
	c.modelList = newModelList
	c.lastUpdateTime = time.Now()

	common.SysLog(fmt.Sprintf("OpenRouter: Cache refreshed, %d models loaded", len(newModelList)))
}

// getEnabledModelsFromDB 从数据库获取启用的模型列表
func (c *OpenRouterCache) getEnabledModelsFromDB() []string {
	// 获取所有启用渠道的模型
	channels, err := GetAllChannels(0, 0, true, false)
	if err != nil {
		common.SysLog("OpenRouter: Failed to get channels: " + err.Error())
		return []string{}
	}

	modelSet := make(map[string]bool)
	for _, channel := range channels {
		if channel.Status != 1 {
			continue // 跳过未启用的渠道
		}
		// 解析渠道支持的模型
		var models []string
		if err := json.Unmarshal([]byte(channel.Models), &models); err != nil {
			// 尝试逗号分隔格式
			models = splitModelsString(channel.Models)
		}
		for _, m := range models {
			if m != "" {
				modelSet[m] = true
			}
		}
	}

	// 转换为列表
	result := make([]string, 0, len(modelSet))
	for model := range modelSet {
		result = append(result, model)
	}
	return result
}

// splitModelsString 按逗号分隔模型字符串
func splitModelsString(models string) []string {
	result := []string{}
	for _, m := range strings.Split(models, ",") {
		m = strings.TrimSpace(m)
		if m != "" {
			result = append(result, m)
		}
	}
	return result
}

// buildOpenRouterModel 构建单个模型的OpenRouter格式数据
func (c *OpenRouterCache) buildOpenRouterModel(modelName string) *OpenRouterModel {
	// 1. 检查是否有配置文件中的扩展信息
	if c.config != nil {
		if modelConfig, exists := c.config.Models[modelName]; exists {
			return c.buildFromConfig(modelName, &modelConfig)
		}
	}

	// 2. 使用默认值
	return c.buildDefault(modelName)
}

// buildFromConfig 从配置构建模型
func (c *OpenRouterCache) buildFromConfig(modelName string, config *OpenRouterModelConfig) *OpenRouterModel {
	name := config.Name
	if name == "" {
		name = modelName
	}

	canonicalSlug := config.CanonicalSlug
	if canonicalSlug == "" {
		canonicalSlug = modelName
	}

	return &OpenRouterModel{
		ID:                  modelName,
		CanonicalSlug:       canonicalSlug,
		HuggingFaceID:       config.HuggingFaceID,
		Name:                name,
		Created:             time.Now().Unix(),
		Description:         config.Description,
		ContextLength:       config.ContextLength,
		Architecture:        config.Architecture,
		Pricing:             config.Pricing,
		TopProvider:         config.TopProvider,
		PerRequestLimits:    nil,
		SupportedParameters: config.SupportedParameters,
		DefaultParameters:   config.DefaultParameters,
	}
}

// buildDefault 使用默认值构建模型
func (c *OpenRouterCache) buildDefault(modelName string) *OpenRouterModel {
	fallback := c.config.Fallback

	// 默认支持参数
	defaultParams := []string{
		"max_tokens",
		"temperature",
		"top_p",
		"frequency_penalty",
		"presence_penalty",
		"stop",
	}

	return &OpenRouterModel{
		ID:            modelName,
		CanonicalSlug: modelName,
		HuggingFaceID: nil,
		Name:          modelName,
		Created:       time.Now().Unix(),
		Description:   "",
		ContextLength: fallback.DefaultContextLength,
		Architecture:  fallback.DefaultArchitecture,
		Pricing:       fallback.DefaultPricing,
		TopProvider: ModelTopProvider{
			ContextLength:       fallback.DefaultContextLength,
			MaxCompletionTokens: nil,
			IsModerated:         false,
		},
		PerRequestLimits:    nil,
		SupportedParameters: defaultParams,
		DefaultParameters: map[string]interface{}{
			"temperature": 0.7,
		},
	}
}

// ==================== 公共辅助函数 ====================

// GetOpenRouterModels 获取所有OpenRouter格式模型（便捷函数）
func GetOpenRouterModels() []*OpenRouterModel {
	return GetOpenRouterCache().GetAllModels()
}

// GetOpenRouterModel 获取单个OpenRouter格式模型（便捷函数）
func GetOpenRouterModel(modelName string) (*OpenRouterModel, bool) {
	return GetOpenRouterCache().GetModel(modelName)
}

// IsOpenRouterEnabled 检查OpenRouter是否启用（便捷函数）
func IsOpenRouterEnabled() bool {
	return GetOpenRouterCache().IsEnabled()
}
