# GLM模型调用Panic错误修复

**日期**: 2025-08-20
**状态**: ✅ 已完成

## 问题描述

调用 GLM-4.5 等智谱模型时出现 `interface conversion: interface {} is nil, not types.OpenAIError` 错误，导致 500 panic。切换到 OpenRouter 的 GLM-4.5 能正常工作。

## 根本原因

- `service/error.go:108-109` 中错误处理逻辑有严重缺陷
- 先用 `NewErrorWithStatusCode()` 创建错误对象（`RelayError = nil`）
- 然后强制设置 `ErrorType = ErrorTypeOpenAIError`，造成类型不一致
- 调用 `ToOpenAIError()` 时尝试访问 `nil` 的 `RelayError` 导致 panic

## 修复方案

### 1. 修复根本问题 (`service/error.go:108-113`)

- 删除错误的 `ErrorType` 强制设置
- 正确构造 `OpenAIError` 对象并使用 `WithOpenAIError()` 创建错误

### 2. 添加防护措施 (`types/error.go:108-116, 139-159`)

- 在 `ToOpenAIError()` 和 `ToClaudeError()` 中添加 `nil` 检查
- 当 `RelayError` 为 `nil` 时返回通用错误格式，避免 panic

## 影响文件

- `service/error.go` - 修复错误处理逻辑
- `types/error.go` - 添加防护措施

## 测试验证

- ✅ 编译成功，无语法错误
- ✅ 后端服务启动正常
- ✅ 所有API路由正确注册
- ✅ 修复GLM等模型调用的panic问题
