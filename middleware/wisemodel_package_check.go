package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// WisemodelPackageCheck 检查 wisemodel token 是否有有效资源包。
// 阶段一（警告期）：仅打 WARNING 日志，不拦截请求。
// 阶段二（强制期）：将 c.AbortWithStatusJSON(403, ...) 取消注释，删除 c.Next()。
func WisemodelPackageCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenName, _ := c.Get("token_name")
		tokenKey, _ := c.Get("token_key")

		name, _ := tokenName.(string)
		key, _ := tokenKey.(string)

		isWisemodelToken := name == "wisemodel-token" || strings.HasPrefix(key, "wisemodel-")
		if !isWisemodelToken {
			c.Next()
			return
		}

		userId := c.GetInt("id")
		if userId == 0 {
			c.Next()
			return
		}

		packages, err := model.GetActiveWisemodelPackages(userId)
		if err != nil {
			logger.LogError(c, fmt.Sprintf("WisemodelPackageCheck: query failed for user %d: %s", userId, err.Error()))
			c.Next()
			return
		}

		if len(packages) == 0 {
			logger.LogWarn(c, fmt.Sprintf("WisemodelPackageCheck: user %d has wisemodel token but no active packages, request blocked", userId))
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"message": "没有有效的 Wisemodel 资源包，请购买后再使用",
					"type":    "insufficient_quota",
					"code":    "no_active_wisemodel_package",
				},
			})
			return
		}

		c.Next()
	}
}
