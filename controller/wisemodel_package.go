package controller

import (
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CreateOrder 创建订单接口（资源包充值）
// POST /api/wisemodel/orders/record
func CreateOrder(c *gin.Context) {
	var req struct {
		OrderId      string `json:"order_id" binding:"required"`
		PackageCount int    `json:"package_count" binding:"required"`
		Packages     []struct {
			Id         string  `json:"id" binding:"required"`
			Points     int     `json:"points"`
			Tokens     int     `json:"tokens"`
			Amount     float64 `json:"amount" binding:"required"`
			Phone      string  `json:"phone" binding:"required"`
			IsFree     bool    `json:"is_free"`
			ModelNames string  `json:"model_names"`
			ValidUntil string  `json:"valid_until" binding:"required"`
			CreatedAt  string  `json:"created_at" binding:"required"`
		} `json:"packages" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"message": "参数错误: " + err.Error(), "success": false})
		return
	}

	if req.PackageCount != len(req.Packages) {
		c.JSON(400, gin.H{
			"message": fmt.Sprintf("package_count(%d)与实际packages数量(%d)不一致", req.PackageCount, len(req.Packages)),
			"success": false,
		})
		return
	}

	for _, pkg := range req.Packages {
		user := model.GetUserByPhone(pkg.Phone)
		if user == nil {
			c.JSON(404, gin.H{
				"message": fmt.Sprintf("用户 %s 不存在，请先调用绑定接口", pkg.Phone),
				"success": false,
			})
			return
		}

		quota := int64(0)
		originalPoints := 0
		originalTokens := 0

		if pkg.Points > 0 {
			quota = int64(float64(pkg.Points) / model.WisemodelPointsPerUnit * common.QuotaPerUnit)
			originalPoints = pkg.Points
		} else if pkg.Tokens > 0 {
			quota = int64(float64(pkg.Tokens) / model.WisemodelTokensPerUnit * common.QuotaPerUnit)
			originalTokens = pkg.Tokens
		} else {
			c.JSON(400, gin.H{
				"message": fmt.Sprintf("资源包 %s 必须提供points或tokens", pkg.Id),
				"success": false,
			})
			return
		}

		validUntil, err := time.Parse(time.RFC3339, pkg.ValidUntil)
		if err != nil {
			c.JSON(400, gin.H{
				"message": "valid_until格式错误，应为RFC3339格式: " + err.Error(),
				"success": false,
			})
			return
		}
		if !validUntil.After(time.Now()) {
			c.JSON(400, gin.H{
				"message": fmt.Sprintf("资源包 %s 的valid_until必须是未来时间，当前值: %s", pkg.Id, pkg.ValidUntil),
				"success": false,
			})
			return
		}

		availableModels := strings.TrimSpace(pkg.ModelNames)
		internalPackageId := fmt.Sprintf("%s_%d", pkg.Id, time.Now().UnixNano())

		wisemodelPkg := &model.WisemodelPackage{
			UserId:            user.Id,
			PackageId:         internalPackageId,
			OriginalPackageId: pkg.Id,
			OrderId:           req.OrderId,
			OriginalPoints:    originalPoints,
			OriginalTokens:    originalTokens,
			QuotaGranted:      quota,
			RemainQuota:       quota,
			AvailableModels:   availableModels,
			Amount:            pkg.Amount,
			IsFree:            pkg.IsFree,
			ValidUntil:        &validUntil,
		}

		err = model.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&model.User{}).Where("id = ?", user.Id).
				UpdateColumn("quota", gorm.Expr("quota + ?", quota)).Error; err != nil {
				return err
			}
			return tx.Create(wisemodelPkg).Error
		})
		if err != nil {
			c.JSON(500, gin.H{"message": "创建订单失败（事务回滚）: " + err.Error(), "success": false})
			return
		}

		_ = model.InvalidateUserCache(user.Id)

		content := fmt.Sprintf("Wisemodel资源包充值: %s", pkg.Id)
		if pkg.Amount > 0 {
			content += fmt.Sprintf(", 金额: $%.2f", pkg.Amount)
		} else {
			content += ", 免费资源包"
		}
		model.RecordLog(user.Id, model.LogTypeTopup, content)
	}

	c.JSON(200, gin.H{"message": "创建成功", "success": true})
}

// GetPackageUsage 资源包使用情况接口
// POST /api/wisemodel/user/package_usage
func GetPackageUsage(c *gin.Context) {
	var req struct {
		Phone string `json:"phone" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"message": "参数错误: " + err.Error(), "success": false})
		return
	}

	user := model.GetUserByPhone(req.Phone)
	if user == nil {
		c.JSON(404, gin.H{"message": "用户不存在", "success": false})
		return
	}

	packages, err := model.GetWisemodelPackagesByUserId(user.Id)
	if err != nil {
		c.JSON(500, gin.H{"message": "查询资源包失败: " + err.Error(), "success": false})
		return
	}
	if len(packages) == 0 {
		c.JSON(200, gin.H{"code": 200, "data": []interface{}{}, "msg": "success"})
		return
	}

	pkgIds := make([]string, len(packages))
	for i, p := range packages {
		pkgIds[i] = p.PackageId
	}
	modelMap, err := model.GetModelUsageByPackages(pkgIds)
	if err != nil {
		c.JSON(500, gin.H{"message": "查询模型使用明细失败: " + err.Error(), "success": false})
		return
	}

	// 剩余额度直接取自单账本 remain_quota（与门控同源）。
	data := model.BuildPackageUsageRows(packages, modelMap)
	c.JSON(200, gin.H{"code": 200, "data": data, "msg": "success"})
}
