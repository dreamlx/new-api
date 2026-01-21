package controller

import (
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
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
			Id         string `json:"id" binding:"required"`
			Points     int    `json:"points"`
			Tokens     int    `json:"tokens"`
			Amount     float64 `json:"amount" binding:"required"`
			Phone      string `json:"phone" binding:"required"`
			IsFree     bool   `json:"is_free"`
			ValidUntil string `json:"valid_until" binding:"required"`
			CreatedAt  string `json:"created_at" binding:"required"`
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

		// 充值到用户quota
		user.Quota += int(quota)
		err := model.DB.Save(user).Error
		if err != nil {
			c.JSON(500, gin.H{
				"message": "充值失败: " + err.Error(),
				"success": false,
			})
			return
		}

		// 解析时间
		validUntil, err := time.Parse(time.RFC3339, pkg.ValidUntil)
		if err != nil {
			c.JSON(400, gin.H{
				"message": "valid_until格式错误，应为RFC3339格式: " + err.Error(),
				"success": false,
			})
			return
		}

		// 获取可用模型列表
		availableModels := ""
		if models, exists := PackageModels[pkg.Id]; exists {
			availableModels = models
		}

		// 创建资源包记录
		wisemodelPkg := &model.WisemodelPackage{
			UserId:          user.Id,
			PackageId:       pkg.Id,
			OrderId:         req.OrderId,
			OriginalPoints:  originalPoints,
			OriginalTokens:  originalTokens,
			QuotaGranted:    quota,
			AvailableModels: availableModels,
			Amount:          pkg.Amount,
			IsFree:          pkg.IsFree,
			ValidUntil:      &validUntil,
		}

		err = model.CreateWisemodelPackage(wisemodelPkg)
		if err != nil {
			c.JSON(500, gin.H{
				"message": "创建资源包记录失败: " + err.Error(),
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
		c.JSON(400, gin.H{
			"message": "参数错误: " + err.Error(),
			"success": false,
		})
		return
	}

	// 查找用户
	user := model.GetUserByPhone(req.Phone)
	if user == nil {
		c.JSON(404, gin.H{
			"message": "用户不存在",
			"success": false,
		})
		return
	}

	// 获取用户的所有资源包
	packages, err := model.GetWisemodelPackagesByUserId(user.Id)
	if err != nil {
		c.JSON(500, gin.H{
			"message": "查询资源包失败: " + err.Error(),
			"success": false,
		})
		return
	}

	var data []map[string]interface{}

	for _, pkg := range packages {
		// 从logs表统计该资源包创建后的所有消费
		var logs []model.Log
		query := model.LOG_DB.Where("user_id = ? AND type = ?", user.Id, model.LogTypeConsume)

		// 只统计资源包创建后的消费
		if !pkg.CreatedAt.IsZero() {
			query = query.Where("created_at >= ?", pkg.CreatedAt)
		}

		query.Find(&logs)

		// 按模型聚合使用量
		modelUsage := make(map[string]int)
		totalUsed := int(0)

		for _, log := range logs {
			modelName := log.ModelName
			if modelName == "" {
				modelName = "unknown"
			}
			modelUsage[modelName] += log.Quota
			totalUsed += log.Quota
		}

		// 构建details数组
		details := []map[string]interface{}{}
		for modelName, quotaUsed := range modelUsage {
			detail := make(map[string]interface{})
			detail["model_name"] = modelName

			// 根据资源包类型返回对应字段
			if pkg.OriginalPoints > 0 {
				detail["used_amount"] = quotaUsed / 500000 // quota转回points
			} else {
				detail["used_amount_tokens"] = quotaUsed / 500000 // quota转回tokens
			}

			details = append(details, detail)
		}

		// 解析available_models
		availableModels := []string{}
		if pkg.AvailableModels != "" {
			for _, model := range strings.Split(pkg.AvailableModels, ",") {
				model = strings.TrimSpace(model)
				if model != "" {
					availableModels = append(availableModels, model)
				}
			}
		}

		// 构建响应数据
		packageData := make(map[string]interface{})
		packageData["package_id"] = pkg.PackageId
		packageData["available_models"] = availableModels
		packageData["details"] = details

		// 根据资源包类型返回不同字段
		if pkg.OriginalPoints > 0 {
			// 积分模式
			packageData["points"] = pkg.OriginalPoints
			packageData["remain_points"] = (pkg.QuotaGranted - int64(totalUsed)) / 500000
			packageData["amount"] = totalUsed / 500000
		} else {
			// Token模式
			packageData["tokens"] = pkg.OriginalTokens
			packageData["remain_tokens"] = (pkg.QuotaGranted - int64(totalUsed)) / 500000
			packageData["amount_tokens"] = totalUsed / 500000
		}

		data = append(data, packageData)
	}

	c.JSON(200, gin.H{
		"code": 200,
		"data": data,
		"msg":  "success",
	})
}
