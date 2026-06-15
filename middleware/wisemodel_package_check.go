package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// WisemodelPackageCheck 是 wisemodel token 的存在性粗检：
//   - 是否 wisemodel token；
//   - 是否存在 active 资源包；
//   - 请求模型是否被某个 active 包覆盖。
//
// 它**不读取/比较任何额度数字、不选包、不设 wisemodel_package_id**——
// 额度门控与选包由 relay 预扣钩子(service.PrepareWisemodelPackageForPreConsume +
// PreConsumeWisemodelPkg 的原子扣减)单点负责，从而消除"双层账本背离"。
// DB 故障 → fail-closed(503)；无 active 包/模型不支持 → 403。
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
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": gin.H{
					"message": "服务暂时不可用，请稍后重试",
					"type":    "server_error",
					"code":    "wisemodel_db_unavailable",
				},
			})
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

		// 解析请求模型名（body 可重复读，不影响后续处理）。
		var modelReq struct {
			Model string `json:"model"`
		}
		if err := common.UnmarshalBodyReusable(c, &modelReq); err != nil {
			// 解析失败不阻断请求（如音频转录等非 JSON body）
			logger.LogWarn(c, fmt.Sprintf(
				"WisemodelPackageCheck: failed to extract model name for user %d: %s", userId, err.Error(),
			))
		}

		// 请求模型必须被某个 active 包覆盖（通用包 AvailableModels 为空，承载任意模型）。
		if modelReq.Model != "" {
			if len(model.FilterPackagesByModel(packages, modelReq.Model)) == 0 {
				logger.LogWarn(c, fmt.Sprintf(
					"WisemodelPackageCheck: user %d requested model '%s' not covered by any active package",
					userId, modelReq.Model,
				))
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": gin.H{
						"message": fmt.Sprintf("您的 Wisemodel 资源包不支持模型 %s，请检查资源包的可用模型列表", modelReq.Model),
						"type":    "permission_denied",
						"code":    "model_not_allowed_by_wisemodel_package",
					},
				})
				return
			}
		}

		// 存在性粗检通过；额度门控与选包交由 relay 预扣钩子单点处理。
		c.Next()
	}
}
