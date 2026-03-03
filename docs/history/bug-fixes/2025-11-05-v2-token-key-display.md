# V2消费流水Token显示修复

**日期**: 2025-11-05
**状态**: ✅ 已完成

## 问题背景

**用户反馈**：
- V2消费流水接口返回的 `token_key` 显示 "v2-platform-asd-token"（Token名称）
- 应该显示实际的Token密钥以便识别和追踪

**影响**：
- 无法识别具体使用的是哪个Token
- 前端无法做Token级别的使用分析

## 修复内容

**修复位置**：`controller/v2_external_user.go:345-389`

**逻辑优化**：

修复前：
```go
tokenKey := log.TokenName  // 优先使用Token名称❌
if tokenKey == "" {
    // 才尝试获取实际密钥
    token, _ := model.GetTokenById(log.TokenId)
    tokenKey = token.Key
}
```

修复后：
```go
// 优先获取实际Token密钥✅
if token, err := model.GetTokenById(log.TokenId); err == nil {
    fullKey := token.Key
    if !strings.HasPrefix(fullKey, "sk-") {
        fullKey = "sk-" + fullKey
    }
    // 完整密钥显示
    tokenKey = fullKey
} else if log.TokenName != "" {
    tokenKey = log.TokenName  // 降级为备选
} else {
    tokenKey = "unknown"
}
```

**关键改进**：

1. **优先级调整**：
   - ✅ 第1优先：实际Token密钥
   - ✅ 第2优先：Token名称（备选）
   - ✅ 第3优先：显示 "unknown"

2. **完整密钥显示**：
   - 返回完整Token密钥（带sk-前缀）
   - 便于对方平台匹配识别

3. **兼容性**：
   - ✅ 自动添加 `sk-` 前缀
   - ✅ 处理Token不存在情况
   - ✅ 处理Token太短情况

## 修复效果

**修复前**：
```json
{
  "log_id": "74393",
  "token_key": "v2-platform-asd-token",  ❌ Token名称
  "model_name": "deepseek-r1-0528"
}
```

**修复后**：
```json
{
  "log_id": "74393",
  "token_key": "sk-52cf690bb7054a92a91d85940cdd9c32",  ✅ 完整的实际密钥
  "model_name": "deepseek-r1-0528"
}
```

## 影响文件

**代码修改**：
- `controller/v2_external_user.go` - Token密钥显示逻辑优化（+4 -9）

## 技术要点

1. **优先级设计**：实际密钥 > Token名称 > unknown
2. **完整密钥显示**：返回完整Token密钥（带sk-前缀），便于对方平台匹配识别
3. **自动前缀**：如果数据库中不含sk-前缀，自动添加
4. **兼容性处理**：多种边界情况处理

## 需求变更说明

**初版**：使用脱敏显示（sk-前8位****后4位）
**变更原因**：对方平台需要完整的Token密钥进行匹配识别
**最终方案**：直接返回完整Token密钥（sk-52cf690bb7054a92a91d85940cdd9c32）

## Git提交

- `a74ef25c` - fix(v2-api): 修复消费流水接口token_key显示错误
- `ac3323f4` - docs: 记录V2消费流水Token显示修复
- `6d91490d` - refactor(v2-api): 修改消费流水token_key显示为完整密钥
