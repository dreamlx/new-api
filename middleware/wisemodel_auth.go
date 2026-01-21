package middleware

import (
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// WisemodelAuth Bearer Token认证中间件
// 用于Wisemodel MaaS平台API认证
func WisemodelAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取Authorization header
		auth := c.GetHeader("Authorization")

		// 检查是否有Bearer前缀
		if !strings.HasPrefix(auth, "Bearer ") {
			c.JSON(401, gin.H{
				"message": "缺少Authorization头或格式错误",
				"success": false,
			})
			c.Abort()
			return
		}

		// 提取token
		token := strings.TrimPrefix(auth, "Bearer ")
		token = strings.TrimSpace(token)

		// 验证token
		if !isValidWisemodelToken(token) {
			c.JSON(401, gin.H{
				"message": "无效的Token",
				"success": false,
			})
			c.Abort()
			return
		}

		// 验证通过，继续处理请求
		c.Next()
	}
}

// isValidWisemodelToken 验证Wisemodel API Token是否有效
func isValidWisemodelToken(token string) bool {
	// 从环境变量获取配置的token
	expectedToken := os.Getenv("WISEMODEL_API_TOKEN")

	// 如果未配置token，拒绝所有请求（安全优先）
	if expectedToken == "" {
		return false
	}

	// 比对token
	return token == expectedToken
}
