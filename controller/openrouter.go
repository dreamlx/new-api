package controller

import (
	"net/http"

	"one-api/model"

	"github.com/gin-gonic/gin"
)

// OpenRouterModelsResponse OpenRouter models接口响应格式
type OpenRouterModelsResponse struct {
	Data []*model.OpenRouterModel `json:"data"`
}

// ListOpenRouterModels GET /openrouter/v1/models - 获取OpenRouter格式的模型列表
func ListOpenRouterModels(c *gin.Context) {
	// 检查功能是否启用
	if !model.IsOpenRouterEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": gin.H{
				"message": "OpenRouter API is not enabled",
				"type":    "service_unavailable",
			},
		})
		return
	}

	models := model.GetOpenRouterModels()

	c.JSON(http.StatusOK, OpenRouterModelsResponse{
		Data: models,
	})
}

// GetOpenRouterModel GET /openrouter/v1/models/:model - 获取单个模型信息
func GetOpenRouterModel(c *gin.Context) {
	// 检查功能是否启用
	if !model.IsOpenRouterEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": gin.H{
				"message": "OpenRouter API is not enabled",
				"type":    "service_unavailable",
			},
		})
		return
	}

	modelName := c.Param("model")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "model parameter is required",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	modelInfo, exists := model.GetOpenRouterModel(modelName)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "model not found: " + modelName,
				"type":    "not_found_error",
			},
		})
		return
	}

	c.JSON(http.StatusOK, modelInfo)
}

// GetOpenRouterConfig GET /openrouter/config - 获取OpenRouter配置（管理接口）
func GetOpenRouterConfig(c *gin.Context) {
	cache := model.GetOpenRouterCache()
	config := cache.GetConfig()

	if config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to get config",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    config,
	})
}

// UpdateOpenRouterConfig PUT /openrouter/config - 更新OpenRouter配置（管理接口）
func UpdateOpenRouterConfig(c *gin.Context) {
	var newConfig model.OpenRouterConfig
	if err := c.ShouldBindJSON(&newConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid config format: " + err.Error(),
		})
		return
	}

	cache := model.GetOpenRouterCache()
	if err := cache.UpdateConfig(&newConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update config: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Config updated successfully",
	})
}

// ReloadOpenRouterConfig POST /openrouter/config/reload - 重新加载配置（管理接口）
func ReloadOpenRouterConfig(c *gin.Context) {
	cache := model.GetOpenRouterCache()
	if err := cache.ReloadConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to reload config: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Config reloaded successfully",
	})
}

// GetOpenRouterStatus GET /openrouter/status - 获取OpenRouter状态
func GetOpenRouterStatus(c *gin.Context) {
	cache := model.GetOpenRouterCache()
	config := cache.GetConfig()
	models := cache.GetAllModels()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"enabled":      config != nil && config.Enabled,
			"models_count": len(models),
			"cache_ttl":    config.CacheTTL,
		},
	})
}
