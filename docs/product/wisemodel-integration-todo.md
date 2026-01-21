# Wisemodel MaaS 平台接入开发任务清单

**版本**: v1.0
**创建日期**: 2025-01-21
**预计完成**: 2025-01-22

---

## 📋 项目概述

接入Wisemodel MaaS平台，实现6个API接口，支持用户绑定、资源包充值、使用统计等功能。

**核心设计原则**：
- ✅ 不破坏现有V1/V2 API
- ✅ 不修改核心计费逻辑（复用quota系统）
- ✅ 前端UI无需改动
- ✅ 上游同步友好（最小化核心代码修改）

**资源包本质**：组合优惠套餐 = 固定金额 + 可用模型列表（仅供展示）

---

## 🎯 核心转换规则

```
1 point = 500,000 quota = $1 USD
1 token = 500,000 quota = $1 USD (统一处理)
```

**示例**：
- 100 points → 50,000,000 quota → 充值到user.quota
- 可用模型列表：仅用于展示，不影响实际计费

---

## ✅ 阶段一：数据库设计（30分钟）

### Task 1.1: 扩展users表
- [ ] 增加 `wisemodel_key` 字段（VARCHAR(100)）
- [ ] 创建索引 `idx_users_wisemodel_key`
- [ ] 编写SQL迁移脚本

```sql
ALTER TABLE users ADD COLUMN wisemodel_key VARCHAR(100) DEFAULT '';
CREATE INDEX idx_users_wisemodel_key ON users(wisemodel_key);
```

---

### Task 1.2: 创建wisemodel_packages表
- [ ] 设计表结构
- [ ] 创建迁移脚本
- [ ] 添加索引

```sql
CREATE TABLE wisemodel_packages (
    id INT AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    package_id VARCHAR(50) NOT NULL UNIQUE,
    order_id VARCHAR(50) NOT NULL,

    -- 原始值（Wisemodel传来的，仅用于显示）
    original_points INT DEFAULT 0,
    original_tokens INT DEFAULT 0,

    -- 转换后的quota
    quota_granted BIGINT NOT NULL,

    -- 可用模型列表（仅供展示，不影响计费）
    available_models TEXT,

    amount DECIMAL(10,2) NOT NULL,
    is_free BOOLEAN DEFAULT false,
    valid_until TIMESTAMP NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    INDEX idx_user_id (user_id),
    INDEX idx_package_id (package_id),
    INDEX idx_order_id (order_id)
);
```

---

### Task 1.3: 扩展logs表（可选，用于统计）
- [ ] 增加 `wisemodel_package_id` 字段（VARCHAR(50)）
- [ ] 创建索引 `idx_logs_wisemodel_package_id`

```sql
ALTER TABLE logs ADD COLUMN wisemodel_package_id VARCHAR(50) DEFAULT '';
CREATE INDEX idx_logs_wisemodel_package_id ON logs(wisemodel_package_id);
```

---

## ✅ 阶段二：数据模型（30分钟）

### Task 2.1: 扩展User模型
- [ ] `model/user.go` 增加 `WisemodelKey` 字段
- [ ] 添加GORM标签
- [ ] 测试模型迁移

```go
type User struct {
    // ... 现有字段
    WisemodelKey string `json:"wisemodel_key" gorm:"type:varchar(100);default:''"`
}
```

---

### Task 2.2: 创建WisemodelPackage模型
- [ ] 新建文件 `model/wisemodel_package.go`
- [ ] 定义结构体
- [ ] 实现CRUD方法

```go
type WisemodelPackage struct {
    Id               int       `json:"id"`
    UserId           int       `json:"user_id"`
    PackageId        string    `json:"package_id"`
    OrderId          string    `json:"order_id"`
    OriginalPoints   int       `json:"original_points"`
    OriginalTokens   int       `json:"original_tokens"`
    QuotaGranted     int64     `json:"quota_granted"`
    AvailableModels  string    `json:"available_models"`
    Amount           float64   `json:"amount"`
    IsFree           bool      `json:"is_free"`
    ValidUntil       *time.Time `json:"valid_until"`
    CreatedAt        time.Time `json:"created_at"`
}
```

---

### Task 2.3: 扩展Log模型（可选）
- [ ] `model/log.go` 增加 `WisemodelPackageId` 字段
- [ ] 更新日志记录函数

---

## ✅ 阶段三：认证中间件（30分钟）

### Task 3.1: 创建Bearer Token认证中间件
- [ ] 新建文件 `middleware/wisemodel_auth.go`
- [ ] 实现Bearer Token解析
- [ ] 实现Token验证逻辑
- [ ] 错误处理

```go
func WisemodelAuth() gin.HandlerFunc {
    return func(c *gin.Context) {
        auth := c.GetHeader("Authorization")
        if !strings.HasPrefix(auth, "Bearer ") {
            c.JSON(401, gin.H{"message": "缺少Authorization头", "success": false})
            c.Abort()
            return
        }

        token := strings.TrimPrefix(auth, "Bearer ")
        if !isValidWisemodelToken(token) {
            c.JSON(401, gin.H{"message": "无效的Token", "success": false})
            c.Abort()
            return
        }
        c.Next()
    }
}
```

---

### Task 3.2: Token验证配置
- [ ] 添加环境变量 `WISEMODEL_API_TOKEN`
- [ ] 更新 `.env.dev` 示例
- [ ] 文档说明

---

## ✅ 阶段四：API接口实现（3小时）

### Task 4.1: 用户绑定接口 🔑
**路径**: `POST /api/user/bind`

- [ ] 新建文件 `controller/wisemodel_user.go`
- [ ] 实现 `WisemodelBind` 函数
- [ ] 参数验证：phone, wisemodel_key, username
- [ ] 逻辑：查找或创建用户，绑定wisemodel_key
- [ ] 响应格式：`{"message":"绑定成功","success":true}`

**关键代码**：
```go
func WisemodelBind(c *gin.Context) {
    var req struct {
        Phone        string `json:"phone" binding:"required"`
        WisemodelKey string `json:"wisemodel_key" binding:"required"`
        Username     string `json:"username" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"message": "参数错误", "success": false})
        return
    }

    // 查找或创建用户
    user := model.GetUserByPhone(req.Phone)
    if user == nil {
        user = &model.User{
            Phone:        req.Phone,
            Username:     req.Username,
            WisemodelKey: req.WisemodelKey,
            DisplayName:  req.Username,
            Role:         common.RoleCommonUser,
        }
        model.DB.Create(user)
    } else {
        user.WisemodelKey = req.WisemodelKey
        model.DB.Save(user)
    }

    c.JSON(200, gin.H{"message": "绑定成功", "success": true})
}
```

---

### Task 4.2: Wisemodel-key删除接口
**路径**: `POST /api/user/delete_wisemodel_key`

- [ ] 实现 `DeleteWisemodelKey` 函数
- [ ] 参数验证：phone
- [ ] 逻辑：清空用户的wisemodel_key字段
- [ ] 响应格式：`{"message":"删除成功","success":true}`

```go
func DeleteWisemodelKey(c *gin.Context) {
    var req struct {
        Phone string `json:"phone" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"message": "参数错误", "success": false})
        return
    }

    user := model.GetUserByPhone(req.Phone)
    if user == nil {
        c.JSON(404, gin.H{"message": "用户不存在", "success": false})
        return
    }

    user.WisemodelKey = ""
    model.DB.Save(user)

    c.JSON(200, gin.H{"message": "删除成功", "success": true})
}
```

---

### Task 4.3: Wisemodel-key更新接口
**路径**: `POST /api/user/update_wisemodel_key`

- [ ] 实现 `UpdateWisemodelKey` 函数
- [ ] 参数验证：phone, new_key
- [ ] 逻辑：更新用户的wisemodel_key
- [ ] 响应格式：`{"message":"更新成功","success":true}`

---

### Task 4.4: 创建订单接口（资源包充值）🔑
**路径**: `POST /api/orders/record`

- [ ] 新建文件 `controller/wisemodel_package.go`
- [ ] 实现 `CreateOrder` 函数
- [ ] 参数验证：order_id, package_count, packages数组
- [ ] 核心逻辑：
  - [ ] 遍历packages，转换points/tokens为quota
  - [ ] 充值到user.quota（复用现有逻辑）
  - [ ] 创建WisemodelPackage记录
  - [ ] 创建充值日志（LogTypeTopup）
- [ ] 响应格式：`{"message":"创建成功","success":true}`

**关键代码**：
```go
func CreateOrder(c *gin.Context) {
    var req struct {
        OrderId      string `json:"order_id" binding:"required"`
        PackageCount int    `json:"package_count" binding:"required"`
        Packages     []struct {
            Id         string     `json:"id" binding:"required"`
            Points     int        `json:"points"`
            Tokens     int        `json:"tokens"`
            Amount     float64    `json:"amount" binding:"required"`
            Phone      string     `json:"phone" binding:"required"`
            IsFree     bool       `json:"is_free" binding:"required"`
            ValidUntil string     `json:"valid_until" binding:"required"`
            CreatedAt  string     `json:"created_at" binding:"required"`
        } `json:"packages" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"message": "参数错误", "success": false})
        return
    }

    for _, pkg := range req.Packages {
        user := model.GetUserByPhone(pkg.Phone)
        if user == nil {
            c.JSON(404, gin.H{"message": fmt.Sprintf("用户 %s 不存在", pkg.Phone), "success": false})
            return
        }

        // 转换：1 point/token = 500,000 quota
        quota := int64(0)
        if pkg.Points > 0 {
            quota = int64(pkg.Points) * 500000
        } else if pkg.Tokens > 0 {
            quota = int64(pkg.Tokens) * 500000
        }

        // 充值
        user.Quota += quota
        model.DB.Save(user)

        // 解析时间
        validUntil, _ := time.Parse(time.RFC3339, pkg.ValidUntil)

        // 创建资源包记录
        wisemodelPkg := &model.WisemodelPackage{
            UserId:          user.Id,
            PackageId:       pkg.Id,
            OrderId:         req.OrderId,
            OriginalPoints:  pkg.Points,
            OriginalTokens:  pkg.Tokens,
            QuotaGranted:    quota,
            AvailableModels: "", // 可选：从配置或参数获取
            Amount:          pkg.Amount,
            IsFree:          pkg.IsFree,
            ValidUntil:      &validUntil,
        }
        model.DB.Create(wisemodelPkg)

        // 充值日志
        model.RecordLog(user.Id, model.LogTypeTopup,
            fmt.Sprintf("Wisemodel资源包充值: %s, 金额: $%.2f", pkg.Id, pkg.Amount))
    }

    c.JSON(200, gin.H{"message": "创建成功", "success": true})
}
```

---

### Task 4.5: 手机号更新接口
**路径**: `POST /api/user/update_phone`

- [ ] 实现 `UpdatePhone` 函数
- [ ] 参数验证：old_phone, new_phone
- [ ] 逻辑：
  - [ ] 查找old_phone用户
  - [ ] 检查new_phone是否已存在
  - [ ] 更新phone字段
- [ ] 响应格式：`{"message":"更新成功","success":true}`

---

### Task 4.6: 资源包使用情况接口 🔑
**路径**: `POST /api/user/package_usage`

- [ ] 实现 `GetPackageUsage` 函数
- [ ] 参数验证：phone
- [ ] 逻辑：
  - [ ] 查询用户的所有资源包
  - [ ] 从logs表聚合每个资源包的使用情况（按模型分组）
  - [ ] 计算剩余quota并转换回points/tokens
- [ ] 响应格式：
  ```json
  {
    "code": 200,
    "data": [
      {
        "package_id": "PKG001",
        "points": 10000,
        "remain_points": 9982,
        "amount": 18,
        "available_models": ["DeepSeek-V3", "DeepSeek-R1"],
        "details": [
          {"model_name": "DeepSeek-V3", "used_amount": 12},
          {"model_name": "DeepSeek-R1", "used_amount": 6}
        ]
      }
    ],
    "msg": "success"
  }
  ```

**关键代码**：
```go
func GetPackageUsage(c *gin.Context) {
    var req struct {
        Phone string `json:"phone" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"message": "参数错误", "success": false})
        return
    }

    user := model.GetUserByPhone(req.Phone)
    if user == nil {
        c.JSON(404, gin.H{"message": "用户不存在", "success": false})
        return
    }

    // 查询用户所有资源包
    var packages []model.WisemodelPackage
    model.DB.Where("user_id = ?", user.Id).Find(&packages)

    var data []map[string]interface{}
    for _, pkg := range packages {
        // 从logs聚合使用情况
        var logs []model.Log
        query := model.LOG_DB.Where("user_id = ? AND type = ?", user.Id, model.LogTypeConsumeQuota)
        if pkg.CreatedAt.Unix() > 0 {
            query = query.Where("created_at >= ?", pkg.CreatedAt)
        }
        query.Find(&logs)

        // 按模型聚合
        modelUsage := make(map[string]int64)
        totalUsed := int64(0)
        for _, log := range logs {
            modelUsage[log.ModelName] += log.Quota
            totalUsed += log.Quota
        }

        // 转换为响应格式
        details := []map[string]interface{}{}
        for model, quota := range modelUsage {
            details = append(details, map[string]interface{}{
                "model_name":  model,
                "used_amount": quota / 500000, // 转回points
            })
        }

        // 解析available_models
        availableModels := []string{}
        if pkg.AvailableModels != "" {
            availableModels = strings.Split(pkg.AvailableModels, ",")
        }

        data = append(data, map[string]interface{}{
            "package_id":       pkg.PackageId,
            "points":           pkg.OriginalPoints,
            "remain_points":    (pkg.QuotaGranted - totalUsed) / 500000,
            "amount":           totalUsed / 500000,
            "available_models": availableModels,
            "details":          details,
        })
    }

    c.JSON(200, gin.H{"code": 200, "data": data, "msg": "success"})
}
```

---

### Task 4.7: 取消授权接口
**路径**: `POST /api/user/delete_wisemodel_user`

- [ ] 实现 `DeleteWisemodelUser` 函数
- [ ] 参数验证：phone
- [ ] 逻辑：
  - [ ] 查找用户
  - [ ] 检查是否有付费订单（is_free=false AND amount>0）
  - [ ] 如果有付费订单，拒绝删除
  - [ ] 删除wisemodel_key、删除所有资源包记录
- [ ] 响应格式：`{"message":"删除成功","success":true}`

**关键代码**：
```go
func DeleteWisemodelUser(c *gin.Context) {
    var req struct {
        Phone string `json:"phone" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"message": "参数错误", "success": false})
        return
    }

    user := model.GetUserByPhone(req.Phone)
    if user == nil {
        c.JSON(404, gin.H{"message": "用户不存在", "success": false})
        return
    }

    // 检查付费订单
    var paidCount int64
    model.DB.Model(&model.WisemodelPackage{}).
        Where("user_id = ? AND is_free = ? AND amount > ?", user.Id, false, 0).
        Count(&paidCount)

    if paidCount > 0 {
        c.JSON(403, gin.H{"message": "存在付费订单，无法删除", "success": false})
        return
    }

    // 删除资源包记录
    model.DB.Where("user_id = ?", user.Id).Delete(&model.WisemodelPackage{})

    // 清空wisemodel_key
    user.WisemodelKey = ""
    model.DB.Save(user)

    c.JSON(200, gin.H{"message": "删除成功", "success": true})
}
```

---

## ✅ 阶段五：路由配置（30分钟）

### Task 5.1: 创建Wisemodel路由组
- [ ] 新建文件 `router/wisemodel-router.go`
- [ ] 配置Bearer Token认证中间件
- [ ] 注册7个API路由

```go
func SetWisemodelRouter(router *gin.Engine) {
    wisemodelGroup := router.Group("/api")
    wisemodelGroup.Use(middleware.WisemodelAuth())
    {
        // 用户管理
        wisemodelGroup.POST("/user/bind", controller.WisemodelBind)
        wisemodelGroup.POST("/user/delete_wisemodel_key", controller.DeleteWisemodelKey)
        wisemodelGroup.POST("/user/update_wisemodel_key", controller.UpdateWisemodelKey)
        wisemodelGroup.POST("/user/update_phone", controller.UpdatePhone)
        wisemodelGroup.POST("/user/delete_wisemodel_user", controller.DeleteWisemodelUser)

        // 资源包管理
        wisemodelGroup.POST("/orders/record", controller.CreateOrder)
        wisemodelGroup.POST("/user/package_usage", controller.GetPackageUsage)
    }
}
```

---

### Task 5.2: 集成到主路由
- [ ] 修改 `main.go` 或 `router/main.go`
- [ ] 调用 `SetWisemodelRouter(router)`

---

## ✅ 阶段六：测试（2小时）

### Task 6.1: 编写测试脚本
- [ ] 新建文件 `scripts/test-wisemodel-api.sh`
- [ ] 实现7个接口的curl测试用例
- [ ] 测试正常流程和异常情况

**测试场景**：
1. 用户绑定（新用户 + 已存在用户）
2. 创建订单（积分模式 + Token模式）
3. 查询使用情况
4. 更新wisemodel_key
5. 更新手机号
6. 删除wisemodel_key
7. 取消授权（有付费订单 + 无付费订单）

---

### Task 6.2: 单元测试
- [ ] `controller/wisemodel_user_test.go`
- [ ] `controller/wisemodel_package_test.go`
- [ ] 覆盖核心逻辑

---

### Task 6.3: 集成测试
- [ ] 完整业务流程测试
- [ ] 验证quota充值和扣减
- [ ] 验证日志记录

---

## ✅ 阶段七：文档（1小时）

### Task 7.1: API文档
- [ ] 新建文件 `docs/wisemodel-api-integration.md`
- [ ] 详细说明6个接口的用法
- [ ] 提供请求/响应示例

---

### Task 7.2: 集成指南
- [ ] 环境变量配置说明
- [ ] 数据库迁移步骤
- [ ] Wisemodel平台配置指南

---

### Task 7.3: 更新CLAUDE.md
- [ ] 记录开发过程
- [ ] 技术决策说明
- [ ] 测试结果总结

---

## 📊 进度跟踪

| 阶段 | 预计时间 | 实际时间 | 状态 |
|------|---------|---------|------|
| 阶段一：数据库设计 | 30分钟 | - | ⏸️ 待开始 |
| 阶段二：数据模型 | 30分钟 | - | ⏸️ 待开始 |
| 阶段三：认证中间件 | 30分钟 | - | ⏸️ 待开始 |
| 阶段四：API接口 | 3小时 | - | ⏸️ 待开始 |
| 阶段五：路由配置 | 30分钟 | - | ⏸️ 待开始 |
| 阶段六：测试 | 2小时 | - | ⏸️ 待开始 |
| 阶段七：文档 | 1小时 | - | ⏸️ 待开始 |
| **总计** | **8小时** | - | - |

---

## 🎯 验收标准

- [ ] 所有7个API接口实现完成
- [ ] Bearer Token认证正常工作
- [ ] 资源包充值正确转换为quota
- [ ] 使用情况查询准确
- [ ] 测试脚本全部通过（覆盖率100%）
- [ ] API文档完整
- [ ] 不影响现有V1/V2功能
- [ ] 代码通过编译，服务正常启动

---

## 📝 技术备注

### 转换规则
```
1 point = 500,000 quota = $1 USD
1 token = 500,000 quota = $1 USD
```

### 响应格式统一
所有接口成功响应：
```json
{"message": "操作成功", "success": true}
```

所有接口错误响应：
```json
{"message": "错误信息", "success": false}
```

### 环境变量
```bash
WISEMODEL_API_TOKEN=your_secret_token_here
```

### 资源包available_models配置
暂时硬编码在代码中（可后续迁移到数据库或配置文件）：
```go
// controller/wisemodel_package.go
var PackageModels = map[string]string{
    "PKG001": "DeepSeek-V3,DeepSeek-R1",
    "PKG002": "BAAI/bge-large-zh-v1.5,BAAI/bge-reranker-large",
}
```

---

## 🚀 开始开发

准备就绪，可以开始编码！按照阶段顺序逐步实现。

**下一步**: 执行阶段一 - 数据库设计
