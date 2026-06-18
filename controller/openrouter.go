package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// ListOpenRouterModels GET /api/openrouter/v1/models
// 返回 OpenRouter 兼容格式的模型列表，无需认证
func ListOpenRouterModels(c *gin.Context) {
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
	c.JSON(http.StatusOK, gin.H{
		"data": models,
	})
}

// GetOpenRouterModelByID GET /api/openrouter/v1/models/:model
// 返回单个模型的 OpenRouter 格式信息，无需认证
func GetOpenRouterModelByID(c *gin.Context) {
	if !model.IsOpenRouterEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": gin.H{
				"message": "OpenRouter API is not enabled",
				"type":    "service_unavailable",
			},
		})
		return
	}

	modelID := c.Param("model")
	if modelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "model parameter is required",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	m, exists := model.GetOpenRouterModelByID(modelID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "model not found: " + modelID,
				"type":    "not_found_error",
			},
		})
		return
	}

	c.JSON(http.StatusOK, m)
}
