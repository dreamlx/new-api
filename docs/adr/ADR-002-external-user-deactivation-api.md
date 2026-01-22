# ADR-002: 外部用户注销API设计

**状态**: 已批准
**日期**: 2026-01-22
**决策者**: 项目团队

---

## 背景

下游平台（如CEC）需要在用户注销时同步注销New API中的用户。当前问题：

1. **没有外部用户注销API**：只有管理员删除接口，需要登录session
2. **软删除会阻止重新注册**：`CheckUserExistOrDeleted`使用`Unscoped()`检查已删除用户
3. **唯一性字段占用**：`Username`、`ExternalUserId`等字段在软删除后仍被占用

### 用户场景

```
1. 用户在下游平台注销账号
2. 下游平台调用New API注销接口
3. 用户后来想重新注册
4. 期望：可以正常注册，不会提示"用户名已存在"
```

---

## 决策

### 采用方案C：软删除 + 释放唯一性字段

注销时：
1. **修改唯一性字段**：添加`deleted_`前缀和时间戳，释放原值
2. **清空关联字段**：微信/支付宝等ID清空
3. **禁用所有Token**：防止注销后Token继续使用
4. **软删除用户**：设置`deleted_at`，保留历史数据

### 字段处理策略

| 字段 | 处理方式 | 原因 |
|------|---------|------|
| `Username` | 改名：`deleted_{原值}_{时间戳}` | 数据库唯一约束 |
| `ExternalUserId` | 改名：`deleted_{原值}_{时间戳}` | 代码唯一性检查 |
| `AccessToken` | 设为`nil` | 数据库唯一约束 |
| `AffCode` | 设为`nil` | 数据库唯一约束 |
| `WechatOpenId` | 清空`""` | 无唯一约束，清空即可 |
| `WechatUnionId` | 清空`""` | 无唯一约束，清空即可 |
| `AlipayUserId` | 清空`""` | 无唯一约束，清空即可 |
| `Phone` | 清空`""` | 无唯一约束，清空即可 |
| `Email` | 清空`""` | 无唯一约束，清空即可 |
| `Status` | 设为`Disabled` | 禁用账号 |

### API设计

```
DELETE /api/user/external/:external_user_id
```

**请求**：无body

**响应（成功）**：
```json
{
  "success": true,
  "message": "用户已注销",
  "data": {
    "user_id": 123,
    "original_external_user_id": "cec_user_456",
    "deleted_external_user_id": "deleted_cec_user_456_1705912345",
    "tokens_disabled": 5,
    "deleted_at": "2026-01-22T12:34:56Z"
  }
}
```

**响应（用户不存在）**：
```json
{
  "success": false,
  "message": "用户不存在"
}
```

**响应（已注销）**：
```json
{
  "success": false,
  "message": "用户已注销"
}
```

---

## 实现细节

### Model层函数

```go
// model/user.go

// DeactivateExternalUser 注销外部用户（软删除+释放唯一字段）
func DeactivateExternalUser(externalUserId string) (*User, int, error) {
    // 1. 查找用户
    user, err := GetUserByExternalUserId(externalUserId)
    if err != nil {
        return nil, 0, err
    }

    // 2. 生成时间戳后缀
    timestamp := time.Now().Unix()

    // 3. 释放唯一性字段
    user.Username = fmt.Sprintf("deleted_%s_%d", user.Username, timestamp)
    user.ExternalUserId = fmt.Sprintf("deleted_%s_%d", externalUserId, timestamp)
    user.AccessToken = nil
    user.AffCode = nil

    // 4. 清空关联字段
    user.WechatOpenId = ""
    user.WechatUnionId = ""
    user.AlipayUserId = ""
    user.Phone = ""
    user.Email = ""

    // 5. 禁用账号
    user.Status = common.UserStatusDisabled

    // 6. 保存更改
    DB.Save(user)

    // 7. 禁用所有Token
    tokensDisabled := DisableAllTokensByUserId(user.Id)

    // 8. 软删除
    DB.Delete(user)

    return user, tokensDisabled, nil
}
```

### Controller层函数

```go
// controller/external_user.go

// DeleteExternalUser 注销外部用户
// DELETE /api/user/external/:external_user_id
func DeleteExternalUser(c *gin.Context) {
    externalUserId := c.Param("external_user_id")

    // 调用model层函数
    user, tokensDisabled, err := model.DeactivateExternalUser(externalUserId)

    // 返回响应
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "用户已注销",
        "data": gin.H{
            "user_id": user.Id,
            "original_external_user_id": externalUserId,
            "deleted_external_user_id": user.ExternalUserId,
            "tokens_disabled": tokensDisabled,
            "deleted_at": user.DeletedAt,
        },
    })
}
```

### 路由配置

```go
// router/api-router.go
externalRoute.DELETE("/:external_user_id", controller.DeleteExternalUser)
```

---

## 测试计划

### 单元测试

1. **正常注销**：用户存在，注销成功
2. **用户不存在**：返回错误
3. **重复注销**：已注销用户再次注销，返回错误
4. **重新注册**：注销后使用相同external_user_id重新注册，成功
5. **Token禁用验证**：注销后Token不可用

### curl测试

```bash
# 1. 创建测试用户
curl -X POST http://localhost:3000/api/user/external/sync \
  -H "Content-Type: application/json" \
  -d '{"external_user_id":"test_deactivate_001","username":"testuser001"}'

# 2. 注销用户
curl -X DELETE http://localhost:3000/api/user/external/test_deactivate_001

# 3. 验证重新注册
curl -X POST http://localhost:3000/api/user/external/sync \
  -H "Content-Type: application/json" \
  -d '{"external_user_id":"test_deactivate_001","username":"testuser001_new"}'
```

---

## 影响分析

### 代码变更

| 文件 | 变更内容 |
|------|---------|
| `model/user.go` | 新增`DeactivateExternalUser`函数 |
| `model/user.go` | 新增`GetUserByExternalUserId`函数（如不存在） |
| `model/token.go` | 新增`DisableAllTokensByUserId`函数 |
| `controller/external_user.go` | 新增`DeleteExternalUser`控制器 |
| `router/api-router.go` | 新增DELETE路由 |

### 向后兼容性

- ✅ 新增API，不影响现有接口
- ✅ 软删除保留历史数据
- ✅ 注销后可重新注册

### 安全考虑

- ⚠️ 该接口应限制访问（IP白名单或API密钥）
- ⚠️ 注销操作应记录审计日志

---

## 替代方案（未采用）

### 方案A：硬删除
- **未采用原因**：丢失历史数据，无法审计

### 方案B：软删除+复用账号
- **未采用原因**：消费记录延续可能不符合预期

### 方案D：状态标记
- **未采用原因**：与现有GORM软删除机制不一致

---

## 修订历史

| 版本 | 日期 | 变更内容 |
|------|------|---------|
| 1.0 | 2026-01-22 | 初稿，确定方案C |
