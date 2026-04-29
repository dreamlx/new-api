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

// WisemodelPackageCheck 检查 wisemodel token 是否有有效资源包。
// DB 故障 → fail-closed (503)；无有效包/配额耗尽 → 403。
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

		// 归因计算使用全部包，并将 packages 原地按 FIFO 顺序排序（ValidUntil ASC, CreatedAt ASC）。
		// 必须在 FilterPackagesByModel 之前调用，确保 eligiblePackages 继承相同的 FIFO 顺序。
		attribution, err := model.CalculatePackageAttribution(userId, packages)
		if err != nil {
			logger.LogError(c, fmt.Sprintf("WisemodelPackageCheck: attribution failed for user %d: %s", userId, err.Error()))
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": gin.H{
					"message": "服务暂时不可用，请稍后重试",
					"type":    "server_error",
					"code":    "wisemodel_attribution_error",
				},
			})
			return
		}

		// 按请求模型过滤出可承载该请求的资源包子集（在 FIFO 排序后过滤，保留顺序）。
		// 通用包（AvailableModels 为空）可承载任意模型；
		// 专用包仅在请求模型出现在其列表中时才纳入候选。
		eligiblePackages := packages // 默认全部可用（解析失败或无模型名时不限制）
		if modelReq.Model != "" {
			eligiblePackages = model.FilterPackagesByModel(packages, modelReq.Model)
			if len(eligiblePackages) == 0 {
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
		selected := model.SelectPackageWithRemainingQuota(eligiblePackages, attribution)
		if selected == nil {
			logger.LogWarn(c, fmt.Sprintf("WisemodelPackageCheck: user %d has no remaining quota in any eligible package", userId))
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"message": "Wisemodel 资源包配额已耗尽，请购买新的资源包",
					"type":    "insufficient_quota",
					"code":    "wisemodel_quota_exhausted",
				},
			})
			return
		}
		c.Set("wisemodel_package_id", selected.PackageId)
		c.Next()
	}
}
