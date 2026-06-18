package model

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gopkg.in/yaml.v3"
)

// ==================== OpenRouter 数据结构 ====================

// OpenRouterModel OpenRouter 格式的模型信息
type OpenRouterModel struct {
	ID                  string                 `json:"id"`
	CanonicalSlug       string                 `json:"canonical_slug"`
	HuggingFaceID       *string                `json:"hugging_face_id"`
	Name                string                 `json:"name"`
	Created             int64                  `json:"created"`
	Description         string                 `json:"description"`
	ContextLength       int                    `json:"context_length"`
	Architecture        OpenRouterArchitecture `json:"architecture"`
	Pricing             OpenRouterPricing      `json:"pricing"`
	TopProvider         OpenRouterTopProvider  `json:"top_provider"`
	PerRequestLimits    interface{}            `json:"per_request_limits"`
	SupportedParameters []string               `json:"supported_parameters"`
	DefaultParameters   map[string]interface{} `json:"default_parameters"`
}

// OpenRouterArchitecture 模型架构信息
type OpenRouterArchitecture struct {
	Modality         string   `json:"modality" yaml:"modality"`
	InputModalities  []string `json:"input_modalities" yaml:"input_modalities"`
	OutputModalities []string `json:"output_modalities" yaml:"output_modalities"`
	Tokenizer        string   `json:"tokenizer" yaml:"tokenizer"`
	InstructType     *string  `json:"instruct_type" yaml:"instruct_type"`
}

// OpenRouterPricing 模型计费信息
type OpenRouterPricing struct {
	Prompt            string `json:"prompt" yaml:"prompt"`
	Completion        string `json:"completion" yaml:"completion"`
	Request           string `json:"request" yaml:"request"`
	Image             string `json:"image" yaml:"image"`
	WebSearch         string `json:"web_search" yaml:"web_search"`
	InternalReasoning string `json:"internal_reasoning" yaml:"internal_reasoning"`
}

// OpenRouterTopProvider 提供方能力信息
type OpenRouterTopProvider struct {
	ContextLength       int  `json:"context_length" yaml:"context_length"`
	MaxCompletionTokens *int `json:"max_completion_tokens" yaml:"max_completion_tokens"`
	IsModerated         bool `json:"is_moderated" yaml:"is_moderated"`
}

// ==================== YAML 配置结构 ====================

// openRouterConfig OpenRouter 全局配置（仅内部使用）
type openRouterConfig struct {
	Enabled  bool                               `yaml:"enabled"`
	CacheTTL int                                `yaml:"cache_ttl"`
	Fallback openRouterFallbackConfig           `yaml:"fallback"`
	Models   map[string]openRouterModelConfig   `yaml:"models"`
}

// openRouterFallbackConfig 默认值配置
type openRouterFallbackConfig struct {
	UseDefaults          bool                   `yaml:"use_defaults"`
	DefaultContextLength int                    `yaml:"default_context_length"`
	DefaultPricing       OpenRouterPricing      `yaml:"default_pricing"`
	DefaultArchitecture  OpenRouterArchitecture `yaml:"default_architecture"`
}

// openRouterModelConfig 单个模型的 YAML 配置
type openRouterModelConfig struct {
	CanonicalSlug       string                 `yaml:"canonical_slug"`
	HuggingFaceID       *string                `yaml:"hugging_face_id"`
	Name                string                 `yaml:"name"`
	Description         string                 `yaml:"description"`
	ContextLength       int                    `yaml:"context_length"`
	Architecture        OpenRouterArchitecture `yaml:"architecture"`
	Pricing             OpenRouterPricing      `yaml:"pricing"`
	TopProvider         OpenRouterTopProvider  `yaml:"top_provider"`
	SupportedParameters []string               `yaml:"supported_parameters"`
	DefaultParameters   map[string]interface{} `yaml:"default_parameters"`
}

// ==================== 缓存实现 ====================

// openRouterCache TTL 内存缓存
type openRouterCache struct {
	models         map[string]*OpenRouterModel
	modelList      []*OpenRouterModel
	lastUpdateTime time.Time
	ttl            time.Duration
	config         *openRouterConfig
	configPath     string
	mu             sync.RWMutex
	rebuilding     atomic.Bool // 防止并发触发多个重建 goroutine
}

var (
	orCache     *openRouterCache
	orCacheOnce sync.Once
)

func getOpenRouterCache() *openRouterCache {
	orCacheOnce.Do(func() {
		configPath := os.Getenv("OPENROUTER_CONFIG_PATH")
		if configPath == "" {
			configPath = filepath.Join("config", "openrouter.yaml")
		}
		c := &openRouterCache{
			models:     make(map[string]*OpenRouterModel),
			modelList:  make([]*OpenRouterModel, 0),
			ttl:        5 * time.Minute,
			configPath: configPath,
		}
		if err := c.loadConfig(); err != nil {
			common.SysLog("OpenRouter: failed to load config: " + err.Error())
		}
		c.doRebuild() // 启动时同步执行，后续走异步路径
		orCache = c
	})
	return orCache
}

// loadConfig 从 YAML 文件加载配置，文件不存在时使用内置默认值
func (c *openRouterCache) loadConfig() error {
	data, err := os.ReadFile(c.configPath)
	if err != nil {
		c.mu.Lock()
		c.config = defaultOpenRouterConfig()
		c.mu.Unlock()
		common.SysLog("OpenRouter: config file not found, using defaults (" + c.configPath + ")")
		return nil
	}

	var cfg openRouterConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse yaml: %w", err)
	}

	c.mu.Lock()
	c.config = &cfg
	if cfg.CacheTTL > 0 {
		c.ttl = time.Duration(cfg.CacheTTL) * time.Second
	}
	c.mu.Unlock()

	common.SysLog("OpenRouter: config loaded from " + c.configPath)
	return nil
}

// triggerRebuildAsync 在 TTL 过期时触发后台重建，atomic CAS 保证同时只有一个 goroutine 在运行
func (c *openRouterCache) triggerRebuildAsync() {
	if c.rebuilding.CompareAndSwap(false, true) {
		go func() {
			defer c.rebuilding.Store(false)
			c.doRebuild()
		}()
	}
}

// doRebuild 重建模型缓存：DB 查询在锁外执行，写锁仅用于最终数据交换
func (c *openRouterCache) doRebuild() {
	// 1. 先用读锁快照当前 config，避免后续持锁做耗时操作
	c.mu.RLock()
	cfg := c.config
	c.mu.RUnlock()

	// 2. DB 查询完全在锁外执行，不阻塞任何读操作
	enabledModels := GetEnabledModels()

	// 3. 构建新缓存数据（纯内存操作，无需锁）
	newModels := make(map[string]*OpenRouterModel, len(enabledModels))
	newList := make([]*OpenRouterModel, 0, len(enabledModels))
	for _, name := range enabledModels {
		m := buildModelWithConfig(name, cfg)
		newModels[name] = m
		newList = append(newList, m)
	}

	// 4. 仅在数据交换时持写锁，时间极短
	c.mu.Lock()
	c.models = newModels
	c.modelList = newList
	c.lastUpdateTime = time.Now()
	c.mu.Unlock()

	common.SysLog(fmt.Sprintf("OpenRouter: cache rebuilt, %d models", len(newList)))
}

// buildModelWithConfig 接受外部传入的 config 快照，避免在锁内访问 c.config
func buildModelWithConfig(name string, cfg *openRouterConfig) *OpenRouterModel {
	if cfg != nil {
		if modelCfg, ok := cfg.Models[name]; ok {
			return buildFromYAML(name, &modelCfg)
		}
	}
	return buildDefault(name, cfg)
}

func buildFromYAML(name string, cfg *openRouterModelConfig) *OpenRouterModel {
	displayName := cfg.Name
	if displayName == "" {
		displayName = name
	}
	slug := cfg.CanonicalSlug
	if slug == "" {
		slug = name
	}
	return &OpenRouterModel{
		ID:                  name,
		CanonicalSlug:       slug,
		HuggingFaceID:       cfg.HuggingFaceID,
		Name:                displayName,
		Created:             time.Now().Unix(),
		Description:         cfg.Description,
		ContextLength:       cfg.ContextLength,
		Architecture:        cfg.Architecture,
		Pricing:             cfg.Pricing,
		TopProvider:         cfg.TopProvider,
		PerRequestLimits:    nil,
		SupportedParameters: cfg.SupportedParameters,
		DefaultParameters:   cfg.DefaultParameters,
	}
}

func buildDefault(name string, cfg *openRouterConfig) *OpenRouterModel {
	fb := defaultFallback()
	if cfg != nil {
		fb = cfg.Fallback
	}
	return &OpenRouterModel{
		ID:            name,
		CanonicalSlug: name,
		HuggingFaceID: nil,
		Name:          name,
		Created:       time.Now().Unix(),
		Description:   "",
		ContextLength: fb.DefaultContextLength,
		Architecture:  fb.DefaultArchitecture,
		Pricing:       fb.DefaultPricing,
		TopProvider: OpenRouterTopProvider{
			ContextLength:       fb.DefaultContextLength,
			MaxCompletionTokens: nil,
			IsModerated:         false,
		},
		PerRequestLimits: nil,
		SupportedParameters: []string{
			"max_tokens", "temperature", "top_p",
			"frequency_penalty", "presence_penalty", "stop",
		},
		DefaultParameters: map[string]interface{}{"temperature": 0.7},
	}
}

// ==================== 缓存读取（带 TTL 惰性刷新）====================

func (c *openRouterCache) getAllModels() []*OpenRouterModel {
	c.mu.RLock()
	expired := time.Since(c.lastUpdateTime) > c.ttl
	list := c.modelList
	c.mu.RUnlock()

	if expired {
		c.triggerRebuildAsync()
	}
	return list
}

func (c *openRouterCache) getModelByID(id string) (*OpenRouterModel, bool) {
	c.mu.RLock()
	expired := time.Since(c.lastUpdateTime) > c.ttl
	m, ok := c.models[id]
	c.mu.RUnlock()

	if expired {
		c.triggerRebuildAsync()
	}
	return m, ok
}

func (c *openRouterCache) isEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config == nil || c.config.Enabled
}

// ==================== 公共辅助函数 ====================

// GetOpenRouterModels 返回所有 OpenRouter 格式的启用模型列表
func GetOpenRouterModels() []*OpenRouterModel {
	return getOpenRouterCache().getAllModels()
}

// GetOpenRouterModelByID 按 ID 返回单个模型信息
func GetOpenRouterModelByID(id string) (*OpenRouterModel, bool) {
	return getOpenRouterCache().getModelByID(id)
}

// IsOpenRouterEnabled 检查 OpenRouter API 是否启用
func IsOpenRouterEnabled() bool {
	return getOpenRouterCache().isEnabled()
}

// ==================== 内置默认值 ====================

func defaultOpenRouterConfig() *openRouterConfig {
	return &openRouterConfig{
		Enabled:  true,
		CacheTTL: 300,
		Fallback: defaultFallback(),
		Models:   make(map[string]openRouterModelConfig),
	}
}

func defaultFallback() openRouterFallbackConfig {
	return openRouterFallbackConfig{
		UseDefaults:          true,
		DefaultContextLength: 8192,
		DefaultPricing: OpenRouterPricing{
			Prompt:            "0.00001",
			Completion:        "0.00002",
			Request:           "0",
			Image:             "0",
			WebSearch:         "0",
			InternalReasoning: "0",
		},
		DefaultArchitecture: OpenRouterArchitecture{
			Modality:         "text->text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
			Tokenizer:        "Unknown",
			InstructType:     nil,
		},
	}
}
