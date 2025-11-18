# Token独立额度恢复实施检查清单

**创建日期**: 2025-01-18
**实施目标**: 恢复New API的Token独立额度机制，支持V1/V2混合使用场景
**置信度**: 5/5

---

## 📋 总览

### 核心决议

1. **✅ 完全切换回Token独立额度**（不是开关模式）
2. **✅ 无需回调功能**（用户明确不需要CEC回调）
3. **✅ V1/V2 API混合使用**
   - V1: 用户充值到User → 分配到Token → Token独立计费
   - V2: 平台Token直接创建 → **UnlimitedQuota=true**（无限额度）
4. **✅ 数据迁移策略**：
   - 单Token用户：100%分配
   - 多Token用户：按比例分配
   - 最低保障：$20 (10,000,000 quota)

### 关键发现

#### V1 API严重Bug 🐛
**位置**: `controller/external_user.go:353`
**问题**: Token创建时复制User余额，未扣除，造成额度膨胀
**影响**: 创建N个Token = 额度膨胀N倍

```go
// ❌ 错误代码
RemainQuota: user.Quota,  // 复制用户余额，但未从User扣除！
```

#### V2 API缺失功能 ⚠️
**位置**: `controller/v2_external_user.go:190-198`
**问题**: 未使用New API原生的 `UnlimitedQuota` 功能
**影响**: V2平台Token仍受额度限制

---

## 🎯 实施步骤

### ⚠️ 实施前准备

- [ ] **备份生产数据库**（必须！）
  ```bash
  mysqldump -u root -p new_api_prod > backup_$(date +%Y%m%d_%H%M%S).sql
  ```
- [ ] **确认测试环境可用**
- [ ] **通知前端团队**：V1 Token创建接口将新增required参数

---

### 步骤1：修复核心计费逻辑 ⭐ P0

**目标**: Token消费扣减Token.RemainQuota（而非User余额）

#### 1.1 修复Token验证逻辑

**文件**: `model/token.go`
**位置**: 第99-110行

**现有代码**:
```go
// 检查用户余额而不是Token余额（Token共享用户余额）
if !token.UnlimitedQuota {
    userQuota, err := GetUserQuota(token.UserId, false)
    if err != nil {
        return token, fmt.Errorf("获取用户余额失败: %v", err)
    }
    if userQuota <= 0 {
        keyPrefix := key[:3]
        keySuffix := key[len(key)-3:]
        return token, errors.New(fmt.Sprintf("[sk-%s***%s] 用户余额已用尽，当前余额: %d", keyPrefix, keySuffix, userQuota))
    }
}
```

**修改为**:
```go
// 恢复：检查Token余额而不是User余额
if !token.UnlimitedQuota {
    if token.RemainQuota <= 0 {
        token.Status = common.TokenStatusExhausted
        token.SelectUpdate()
        keyPrefix := key[:3]
        keySuffix := key[len(key)-3:]
        return token, errors.New(fmt.Sprintf(
            "[sk-%s***%s] 令牌额度已用尽，当前余额: %d",
            keyPrefix, keySuffix, token.RemainQuota))
    }
}
```

**验证**:
- [ ] 代码修改完成
- [ ] 编译通过
- [ ] 本地测试：Token余额为0时返回"令牌额度已用尽"错误

---

#### 1.2 修复预扣费逻辑

**文件**: `service/quota.go`
**位置**: 第466-491行

**现有代码**:
```go
func PreConsumeTokenQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
    // ... 省略前面代码

    // Token共享用户余额，检查并扣减用户余额
    if !relayInfo.TokenUnlimited {
        userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
        if err != nil {
            return fmt.Errorf("获取用户余额失败: %v", err)
        }
        if userQuota < quota {
            return fmt.Errorf("用户余额不足，当前余额: %s，需要: %s", logger.FormatQuota(userQuota), logger.FormatQuota(quota))
        }
        err = model.DecreaseUserQuota(relayInfo.UserId, quota)
        if err != nil {
            return fmt.Errorf("扣减用户余额失败: %v", err)
        }
    }
    return nil
}
```

**修改为**:
```go
func PreConsumeTokenQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
    if quota < 0 {
        return errors.New("quota 不能为负数！")
    }
    if relayInfo.IsPlayground || relayInfo.TokenUnlimited {
        return nil
    }

    // 恢复：扣减Token余额而不是User余额
    token, err := model.GetTokenById(relayInfo.TokenId)
    if err != nil {
        return fmt.Errorf("获取Token失败: %v", err)
    }

    if token.RemainQuota < quota {
        return fmt.Errorf("Token余额不足，当前余额: %s，需要: %s",
            logger.FormatQuota(token.RemainQuota),
            logger.FormatQuota(quota))
    }

    err = model.DecreaseTokenQuota(token.Id, token.Key, quota)
    if err != nil {
        return fmt.Errorf("扣减Token余额失败: %v", err)
    }

    return nil
}
```

**验证**:
- [ ] 代码修改完成
- [ ] 编译通过
- [ ] 本地测试：消费时扣减Token.RemainQuota

---

#### 1.3 修复后扣费逻辑

**文件**: `service/quota.go`
**位置**: 第493-520行

**现有代码**:
```go
func PostConsumeQuota(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool) (err error) {
    if quota > 0 {
        err = model.DecreaseUserQuota(relayInfo.UserId, quota)
    } else {
        err = model.IncreaseUserQuota(relayInfo.UserId, -quota, false)
    }
    if err != nil {
        return err
    }

    // Token共享用户余额，不再单独扣减Token额度

    if sendEmail {
        // ...
    }
    return nil
}
```

**修改为**:
```go
func PostConsumeQuota(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool) (err error) {
    // 恢复：扣减Token余额而不是User余额
    if quota > 0 {
        err = model.DecreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota)
    } else {
        err = model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, -quota)
    }
    if err != nil {
        return err
    }

    // 移除User余额扣减逻辑（关键修改）

    if sendEmail {
        if (quota + preConsumedQuota) != 0 {
            checkAndSendQuotaNotify(relayInfo, quota, preConsumedQuota)
        }
    }
    return nil
}
```

**验证**:
- [ ] 代码修改完成
- [ ] 编译通过
- [ ] 本地测试：消费后Token.RemainQuota减少，User.Quota不变

---

### 步骤2：修复V1和V2 Token创建逻辑 ⭐ P1

#### 2.1 修复V1 Token创建（额度分配）

**文件**: `controller/external_user.go`

**位置1**: 第67-72行（请求结构）

**修改前**:
```go
type ExternalUserTokenRequest struct {
    ExternalUserId string `json:"external_user_id" binding:"required,min=1,max=100"`
    TokenName      string `json:"token_name" binding:"required,min=1,max=100"`
    ExpiresInDays  int    `json:"expires_in_days" binding:"omitempty,min=1,max=3650"`
}
```

**修改后**:
```go
type ExternalUserTokenRequest struct {
    ExternalUserId string `json:"external_user_id" binding:"required,min=1,max=100"`
    TokenName      string `json:"token_name" binding:"required,min=1,max=100"`
    ExpiresInDays  int    `json:"expires_in_days" binding:"omitempty,min=1,max=3650"`
    AllocatedQuota int    `json:"allocated_quota" binding:"required,min=0"` // 新增
}
```

**验证**:
- [ ] 请求结构修改完成

---

**位置2**: 第317-382行（Token创建逻辑）

**修改前**:
```go
func CreateExternalUserToken(c *gin.Context) {
    // ... 验证代码

    // 创建Token
    token := &model.Token{
        UserId:         user.Id,
        Key:            common.GetRandomString(32),
        Name:           req.TokenName,
        CreatedTime:    common.GetTimestamp(),
        AccessedTime:   common.GetTimestamp(),
        ExpiredTime:    common.GetTimestamp() + int64(expiresInDays*24*3600),
        Status:         common.TokenStatusEnabled,
        RemainQuota:    user.Quota,  // ❌ Bug: 复制用户余额，未扣除
        UnlimitedQuota: false,
    }

    if err := model.DB.Create(token).Error; err != nil {
        // ...
    }
    // ...
}
```

**修改后**:
```go
func CreateExternalUserToken(c *gin.Context) {
    var req ExternalUserTokenRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "success": false,
            "message": "请求参数错误: " + err.Error(),
        })
        return
    }

    // 查找用户
    user := &model.User{}
    if err := model.DB.Where("external_user_id = ?", req.ExternalUserId).First(user).Error; err != nil {
        c.JSON(http.StatusNotFound, gin.H{
            "success": false,
            "message": "用户不存在",
        })
        return
    }

    // ✅ 新增：验证User余额是否充足
    if user.Quota < req.AllocatedQuota {
        c.JSON(http.StatusBadRequest, gin.H{
            "success": false,
            "message": fmt.Sprintf("用户余额不足，当前余额: %d, 需要分配: %d",
                user.Quota, req.AllocatedQuota),
        })
        return
    }

    // 设置默认过期时间
    expiresInDays := req.ExpiresInDays
    if expiresInDays == 0 {
        expiresInDays = 365
    }

    // ✅ 新增：开启事务，确保原子性
    tx := model.DB.Begin()
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
        }
    }()

    // ✅ 新增：从User余额中扣除
    if err := tx.Model(user).Update("quota", user.Quota - req.AllocatedQuota).Error; err != nil {
        tx.Rollback()
        common.SysError("扣减User余额失败: " + err.Error())
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "message": "创建Token失败",
        })
        return
    }

    // 创建Token（使用分配的quota）
    token := &model.Token{
        UserId:         user.Id,
        Key:            common.GetRandomString(32),
        Name:           req.TokenName,
        CreatedTime:    common.GetTimestamp(),
        AccessedTime:   common.GetTimestamp(),
        ExpiredTime:    common.GetTimestamp() + int64(expiresInDays*24*3600),
        Status:         common.TokenStatusEnabled,
        RemainQuota:    req.AllocatedQuota,  // ✅ 使用分配的quota
        UnlimitedQuota: false,
    }

    if err := tx.Create(token).Error; err != nil {
        tx.Rollback()
        common.SysError("创建Token失败: " + err.Error())
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "message": "创建Token失败",
        })
        return
    }

    // ✅ 提交事务
    if err := tx.Commit().Error; err != nil {
        common.SysError("提交事务失败: " + err.Error())
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "message": "创建Token失败",
        })
        return
    }

    // 记录日志
    common.SysLog(fmt.Sprintf("为外部用户创建Token成功: %s, Token名称: %s, 分配额度: %d",
        req.ExternalUserId, req.TokenName, req.AllocatedQuota))

    // 重新获取User信息
    model.DB.First(user, user.Id)

    // 构造响应
    response := ExternalUserTokenResponse{
        Success: true,
        Message: "Token创建成功",
    }
    response.Data.TokenId = token.Id
    response.Data.AccessKey = "sk-" + token.Key
    response.Data.TokenName = token.Name
    response.Data.ExpiresAt = token.ExpiredTime
    response.Data.RemainQuota = token.RemainQuota  // ✅ 返回Token自己的额度

    c.JSON(http.StatusOK, response)
}
```

**验证**:
- [ ] 代码修改完成
- [ ] 编译通过
- [ ] 本地测试：创建Token后User.Quota减少，Token.RemainQuota正确

---

#### 2.2 修复V2 Token创建（启用unlimited）

**文件**: `controller/v2_external_user.go`

**位置1**: 第190-198行（创建新Token）

**修改前**:
```go
token := &model.Token{
    UserId:      user.Id,
    Key:         tokenKey,
    Name:        fmt.Sprintf("v2-platform-%s-token", req.PlatformId),
    RemainQuota: req.InitialQuota,
    UsedQuota:   0,
    CreatedTime: time.Now().Unix(),
    Status:      1,
}
```

**修改后**:
```go
token := &model.Token{
    UserId:         user.Id,
    Key:            tokenKey,
    Name:           fmt.Sprintf("v2-platform-%s-token", req.PlatformId),
    UnlimitedQuota: true,           // ✅ V2平台Token无限额度
    RemainQuota:    0,               // ✅ unlimited时设为0
    UsedQuota:      0,
    CreatedTime:    time.Now().Unix(),
    Status:         1,
}
```

**验证**:
- [ ] 代码修改完成
- [ ] 编译通过

---

**位置2**: 第137行附近（更新现有Token）

**修改前**:
```go
err = model.UpdateTokenQuota(existingToken.Id, req.InitialQuota)
```

**修改后**:
```go
// 更新为unlimited模式
err = model.DB.Model(existingToken).Updates(map[string]interface{}{
    "unlimited_quota": true,  // ✅ 设为unlimited
    "remain_quota":    0,     // ✅ 设为0
}).Error
```

**验证**:
- [ ] 代码修改完成
- [ ] 编译通过
- [ ] 本地测试：V2 Token创建后 UnlimitedQuota=true

---

### 步骤3：数据迁移脚本 ⭐ P0

**目标**: 现有Token按比例分配User余额，最低保障$20

#### 3.1 创建迁移脚本

**文件**: `scripts/migrate-token-quota.sql` (新建)

```sql
-- Token独立额度迁移脚本
-- 策略：按用户的Token数量比例分配User余额，最低保障$20
-- 执行前请先备份数据库！

SET @min_quota = 10000000;  -- $20 = 10,000,000 quota

-- 步骤1：为每个User计算Token分配方案
DROP TEMPORARY TABLE IF EXISTS temp_token_allocation;
CREATE TEMPORARY TABLE temp_token_allocation AS
SELECT
    t.id as token_id,
    t.user_id,
    u.quota as user_quota,
    COUNT(*) OVER (PARTITION BY t.user_id) as user_token_count,
    -- 按比例分配
    CASE
        WHEN COUNT(*) OVER (PARTITION BY t.user_id) = 1 THEN u.quota
        ELSE FLOOR(u.quota / COUNT(*) OVER (PARTITION BY t.user_id))
    END as allocated_quota,
    -- 保障最低$20
    CASE
        WHEN COUNT(*) OVER (PARTITION BY t.user_id) = 1 THEN GREATEST(u.quota, @min_quota)
        ELSE GREATEST(FLOOR(u.quota / COUNT(*) OVER (PARTITION BY t.user_id)), @min_quota)
    END as final_quota
FROM tokens t
LEFT JOIN users u ON t.user_id = u.id
WHERE t.deleted_at IS NULL
  AND t.unlimited_quota = 0;

-- 步骤2：更新Token的RemainQuota
UPDATE tokens t
INNER JOIN temp_token_allocation a ON t.id = a.token_id
SET t.remain_quota = a.final_quota;

-- 步骤3：验证额度分配（可选）
SELECT
    u.id as user_id,
    u.username,
    u.quota as user_original_quota,
    COUNT(t.id) as token_count,
    SUM(t.remain_quota) as total_allocated_quota,
    SUM(t.remain_quota) - u.quota as quota_diff,
    CASE
        WHEN SUM(t.remain_quota) > u.quota THEN '额度补齐（最低$20保障）'
        WHEN COUNT(t.id) = 1 THEN '单Token（100%分配）'
        ELSE '多Token（按比例分配）'
    END as allocation_type
FROM users u
LEFT JOIN tokens t ON t.user_id = u.id AND t.deleted_at IS NULL AND t.unlimited_quota = 0
GROUP BY u.id, u.username, u.quota;

-- 清理临时表
DROP TEMPORARY TABLE IF EXISTS temp_token_allocation;
```

**验证**:
- [ ] 脚本创建完成
- [ ] 在测试环境执行成功
- [ ] 验证查询显示分配正确

---

#### 3.2 创建回滚脚本（可选）

**文件**: `scripts/rollback-token-quota.sql` (新建)

```sql
-- 回滚到Token共享User余额模式
-- 将所有Token的RemainQuota设置为对应User的quota

UPDATE tokens t
INNER JOIN users u ON t.user_id = u.id
SET t.remain_quota = u.quota
WHERE t.deleted_at IS NULL
  AND t.unlimited_quota = 0;
```

**验证**:
- [ ] 回滚脚本创建完成
- [ ] 在测试环境验证回滚成功

---

### 步骤4：更新API文档 ⭐ P2

#### 4.1 更新V1 API文档

**文件**: `docs/external-user-api.md`

**位置**: 第126-150行（Token创建接口）

**添加说明**:
```markdown
### 3.1 创建 Access Key

**请求参数**:
```json
{
  "external_user_id": "string, required, 外部用户ID",
  "token_name": "string, required, Token名称",
  "allocated_quota": "number, required, 从用户余额中分配的quota",
  "expires_in_days": "number, optional, 有效期天数，默认365"
}
```

**参数说明**:
- `allocated_quota`：从User余额中分配给Token的quota
  - 必须 ≤ User当前余额
  - 分配后，User.Quota会减少相应的额度
  - Token消费时扣减Token.RemainQuota（而非User余额）
  - 额度守恒：User减少 = Token增加
- 推荐值：$20 = 10,000,000 quota

**示例**：
```bash
# 用户有 $50 (25,000,000 quota)
# 创建Token分配 $20 (10,000,000 quota)

POST /api/user/external/token
{
  "external_user_id": "user_001",
  "token_name": "My API Token",
  "allocated_quota": 10000000,
  "expires_in_days": 365
}

# 结果：
# - User余额：$50 → $30 (15,000,000 quota)
# - Token余额：$20 (10,000,000 quota)
```

**错误处理**:
```json
{
  "success": false,
  "message": "用户余额不足，当前余额: 5000000, 需要分配: 10000000"
}
```
```

**验证**:
- [ ] V1 API文档更新完成

---

#### 4.2 更新V2 API文档

**文件**: `docs/external-user-api-v2.md`

**位置**: 第38-65行（Token授权接口）

**添加说明**:
```markdown
### V2平台Token额度策略

**重要**: V2平台Token使用**无限额度模式**

```json
POST /v2/external/tokens/authorize
{
  "platform_id": "asd",
  "token_key": "sk-abc123def456789xyz",
  "initial_quota": 0  // 可选，V2平台Token无限额度，此参数无效
}
```

**说明**:
- V2平台Token自动设置 `unlimited_quota=true`
- 不受额度限制，可以无限使用
- 适用于平台自己负责计费的场景
- `initial_quota` 参数会被忽略
- Token.RemainQuota 固定为 0（unlimited时无意义）
- Token.UsedQuota 仍会记录用量统计

**为什么V2使用unlimited?**
- V2平台（如asd）自己负责计费
- New API只作为转发网关
- 不需要New API控制额度
- 避免因额度不足导致业务中断
```

**验证**:
- [ ] V2 API文档更新完成

---

### 步骤5：测试验证 ⭐ P1

#### 5.1 单元测试场景

**测试文件**: `controller/external_user_test.go` (新增)

```go
package controller

import "testing"

func TestCreateExternalUserToken_QuotaAllocation(t *testing.T) {
    // 测试额度分配逻辑
    // 1. 创建用户，充值$50
    // 2. 创建Token，分配$20
    // 3. 验证User余额减少到$30
    // 4. 验证Token余额为$20
    // 5. 消费$10
    // 6. 验证Token余额减少到$10
    // 7. 验证User余额仍为$30（不变）
}

func TestCreateExternalUserToken_InsufficientQuota(t *testing.T) {
    // 测试余额不足情况
    // 1. 创建用户，充值$10
    // 2. 尝试创建Token，分配$20
    // 3. 验证返回错误："用户余额不足"
}

func TestV2TokenAuthorize_UnlimitedQuota(t *testing.T) {
    // 测试V2 Token无限额度
    // 1. 调用V2授权接口
    // 2. 验证Token.UnlimitedQuota=true
    // 3. 验证Token.RemainQuota=0
    // 4. 消费后验证不受额度限制
}
```

**验证**:
- [ ] 单元测试编写完成
- [ ] 所有测试通过

---

#### 5.2 集成测试脚本

**文件**: `scripts/test-token-quota-migration.sh` (新建)

```bash
#!/bin/bash

# Token独立额度集成测试

BASE_URL="http://localhost:3000"

echo "=== 测试1：V1充值 → 创建Token → 消费 ==="

# 1. 创建用户
curl -X POST "$BASE_URL/api/user/external/sync" \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "test_quota_user",
    "username": "test_quota_user",
    "email": "test@example.com"
  }'

# 2. 充值$50
curl -X POST "$BASE_URL/api/user/external/topup" \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "test_quota_user",
    "amount_usd": 50.0,
    "payment_id": "test_payment_001"
  }'

# 3. 创建Token，分配$20
RESPONSE=$(curl -X POST "$BASE_URL/api/user/external/token" \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "test_quota_user",
    "token_name": "Test Token",
    "allocated_quota": 10000000,
    "expires_in_days": 365
  }')

TOKEN=$(echo $RESPONSE | jq -r '.data.access_key')
echo "Token创建成功: $TOKEN"

# 4. 使用Token调用API
curl -X POST "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "Hello"}]
  }'

echo "=== 测试2：V2 Token unlimited模式 ==="

# 创建V2 Token
curl -X POST "$BASE_URL/v2/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d '{
    "platform_id": "asd",
    "token_key": "sk-test-unlimited-001",
    "initial_quota": 0
  }'

# 使用V2 Token调用
curl -X POST "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer sk-test-unlimited-001" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

**验证**:
- [ ] 测试脚本创建完成
- [ ] 测试脚本执行成功
- [ ] 所有场景验证通过

---

#### 5.3 手动测试检查表

**V1 API测试**:
- [ ] 用户充值成功，User.Quota增加
- [ ] 创建Token时User.Quota减少
- [ ] Token.RemainQuota等于allocated_quota
- [ ] 余额不足时创建Token失败
- [ ] 消费时Token.RemainQuota减少
- [ ] 消费时User.Quota不变
- [ ] Token余额耗尽时返回错误

**V2 API测试**:
- [ ] V2 Token创建成功
- [ ] Token.UnlimitedQuota=true
- [ ] Token.RemainQuota=0
- [ ] 消费不受额度限制
- [ ] Token.UsedQuota正常增加

**数据迁移测试**:
- [ ] 单Token用户：Token获得100% User余额
- [ ] 多Token用户：Token按比例分配
- [ ] 所有Token至少$20
- [ ] 验证查询显示正确

---

## 🚨 注意事项

### 前端需要同步更新

**V1 Token创建接口变更**:
- 新增required参数：`allocated_quota`
- 前端需要让用户选择或输入分配额度
- 建议默认值：$20 (10,000,000 quota)

**通知前端团队**:
```
主题：V1 Token创建接口变更通知

变更内容：
POST /api/user/external/token
新增必填参数：allocated_quota (number, 从用户余额分配的quota)

影响：
- 旧的请求将返回400错误（缺少required参数）
- 需要在UI上添加额度分配输入/选择

建议实现：
- 显示用户当前余额
- 提供预设选项：$10, $20, $50, $100
- 或允许用户自定义输入
- 验证：allocated_quota <= 用户当前余额

测试环境：http://test-api.example.com
上线时间：待定
```

---

### 数据库备份

**执行迁移前必须备份**:
```bash
# MySQL
mysqldump -u root -p new_api_prod > backup_$(date +%Y%m%d_%H%M%S).sql

# PostgreSQL
pg_dump -U postgres new_api_prod > backup_$(date +%Y%m%d_%H%M%S).sql
```

**验证备份**:
- [ ] 备份文件创建成功
- [ ] 备份文件大小合理
- [ ] 可以在测试环境恢复

---

### 实施顺序建议

**推荐顺序**:
```
1. 备份数据库 ⚠️
   ↓
2. 在测试环境实施全部步骤
   ↓
3. 测试环境完整验证
   ↓
4. 通知前端团队更新（至少提前3天）
   ↓
5. 生产环境实施步骤1（核心计费逻辑）
   ↓
6. 生产环境实施步骤2.2（V2 Token）
   ↓
7. 生产环境实施步骤3（数据迁移）
   ↓
8. 验证生产环境
   ↓
9. 前端上线V1接口更新
   ↓
10. 生产环境实施步骤2.1（V1 Token创建）
   ↓
11. 完整测试验证
```

---

## ✅ 完成标准

### 代码层面
- [ ] 所有代码修改完成
- [ ] 编译通过，无错误
- [ ] 所有单元测试通过
- [ ] 代码审查通过

### 功能层面
- [ ] V1 Token从User分配额度
- [ ] V2 Token使用unlimited模式
- [ ] 消费扣减Token.RemainQuota
- [ ] User余额不受消费影响
- [ ] 数据迁移完成

### 文档层面
- [ ] API文档更新完成
- [ ] 前端通知发送
- [ ] 实施记录完整

### 测试层面
- [ ] 测试环境验证通过
- [ ] 生产环境验证通过
- [ ] 所有场景测试通过

---

## 📊 实施记录

### 实施日期

- [ ] 步骤1完成日期: ___________
- [ ] 步骤2.1完成日期: ___________
- [ ] 步骤2.2完成日期: ___________
- [ ] 步骤3完成日期: ___________
- [ ] 步骤4完成日期: ___________
- [ ] 步骤5完成日期: ___________

### 问题记录

**遇到的问题**:
```
1.
2.
3.
```

**解决方案**:
```
1.
2.
3.
```

---

## 🔗 相关文档

- [CLAUDE.md](../CLAUDE.md) - 项目核心记忆
- [external-user-api.md](./external-user-api.md) - V1 API文档
- [external-user-api-v2.md](./external-user-api-v2.md) - V2 API文档
- [development-guide.md](./development-guide.md) - 开发指南

---

**最后更新**: 2025-01-18
**文档版本**: 1.0
**负责人**: ___________
