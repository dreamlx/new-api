package controller

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PackageModels 资源包ID与可用模型的映射
// 可后续迁移到数据库配置
var PackageModels = map[string]string{
	"PKG001": "DeepSeek-V3,DeepSeek-R1",
	"PKG002": "BAAI/bge-large-zh-v1.5,BAAI/bge-reranker-large",
	// 可以添加更多资源包配置
}

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
			ValidUntil string  `json:"valid_until" binding:"required"`
			CreatedAt  string  `json:"created_at" binding:"required"`
		} `json:"packages" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{
			"message": "参数错误: " + err.Error(),
			"success": false,
		})
		return
	}

	// 验证package_count与实际packages数量是否一致
	if req.PackageCount != len(req.Packages) {
		c.JSON(400, gin.H{
			"message": fmt.Sprintf("package_count(%d)与实际packages数量(%d)不一致", req.PackageCount, len(req.Packages)),
			"success": false,
		})
		return
	}

	// 处理每个资源包
	for _, pkg := range req.Packages {
		// 查找用户
		user := model.GetUserByPhone(pkg.Phone)
		if user == nil {
			c.JSON(404, gin.H{
				"message": fmt.Sprintf("用户 %s 不存在，请先调用绑定接口", pkg.Phone),
				"success": false,
			})
			return
		}

		// 转换：1 point/token = 500,000 quota = $1
		quota := int64(0)
		originalPoints := 0
		originalTokens := 0

		if pkg.Points > 0 {
			quota = int64(pkg.Points) * 500000
			originalPoints = pkg.Points
		} else if pkg.Tokens > 0 {
			quota = int64(pkg.Tokens) * 500000
			originalTokens = pkg.Tokens
		} else {
			c.JSON(400, gin.H{
				"message": fmt.Sprintf("资源包 %s 必须提供points或tokens", pkg.Id),
				"success": false,
			})
			return
		}

		// 解析时间（Fix 1：在事务之前校验，避免无效数据入库）
		validUntil, err := time.Parse(time.RFC3339, pkg.ValidUntil)
		if err != nil {
			c.JSON(400, gin.H{
				"message": "valid_until格式错误，应为RFC3339格式: " + err.Error(),
				"success": false,
			})
			return
		}

		// Fix 1：valid_until 必须是未来时间
		now := time.Now()
		if !validUntil.After(now) {
			c.JSON(400, gin.H{
				"message": fmt.Sprintf("资源包 %s 的valid_until必须是未来时间，当前值: %s", pkg.Id, pkg.ValidUntil),
				"success": false,
			})
			return
		}

		// 获取可用模型列表
		availableModels := ""
		if models, exists := PackageModels[pkg.Id]; exists {
			availableModels = models
		}

		// 追加纳秒时间戳确保 DB 唯一性，允许同一 package_id 多次合法提交
		internalPackageId := fmt.Sprintf("%s_%d", pkg.Id, time.Now().UnixNano())

		// Fix 2：将quota更新和包记录创建包在同一事务中，保证原子性
		wisemodelPkg := &model.WisemodelPackage{
			UserId:            user.Id,
			PackageId:         internalPackageId,
			OriginalPackageId: pkg.Id,
			OrderId:           req.OrderId,
			OriginalPoints:    originalPoints,
			OriginalTokens:    originalTokens,
			QuotaGranted:      quota,
			AvailableModels:   availableModels,
			Amount:            pkg.Amount,
			IsFree:            pkg.IsFree,
			ValidUntil:        &validUntil,
		}

		err = model.DB.Transaction(func(tx *gorm.DB) error {
			// 步骤1：更新用户quota
			if err := tx.Model(&model.User{}).Where("id = ?", user.Id).
				UpdateColumn("quota", gorm.Expr("quota + ?", quota)).Error; err != nil {
				return err
			}
			// 步骤2：创建资源包记录
			return tx.Create(wisemodelPkg).Error
		})
		if err != nil {
			c.JSON(500, gin.H{
				"message": "创建订单失败（事务回滚）: " + err.Error(),
				"success": false,
			})
			return
		}

		// 创建充值日志
		content := fmt.Sprintf("Wisemodel资源包充值: %s", pkg.Id)
		if pkg.Amount > 0 {
			content += fmt.Sprintf(", 金额: $%.2f", pkg.Amount)
		} else {
			content += ", 免费资源包"
		}

		model.RecordLog(user.Id, model.LogTypeTopup, content)
	}

	c.JSON(200, gin.H{
		"message": "创建成功",
		"success": true,
	})
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

	attribution, err := model.CalculatePackageAttribution(user.Id, packages)
	if err != nil {
		c.JSON(500, gin.H{"message": "查询资源包消费失败: " + err.Error(), "success": false})
		return
	}

	// 构建响应
	data := model.BuildPackageUsageRows(packages, attribution)

	c.JSON(200, gin.H{"code": 200, "data": data, "msg": "success"})
}
