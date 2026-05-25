package middleware

import (
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// ExternalAPIAuth is a unified Bearer Token authentication middleware
// for external platform APIs (V1, V2, WiseModel).
// Token is configured via the specified environment variable.
func ExternalAPIAuth(envVar string) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			c.JSON(401, gin.H{
				"message": "缺少Authorization头或格式错误",
				"success": false,
			})
			c.Abort()
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		expectedToken := os.Getenv(envVar)

		if expectedToken == "" || token != expectedToken {
			c.JSON(401, gin.H{
				"message": "无效的Token",
				"success": false,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
