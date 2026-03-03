# GLM模型错误处理和模型支持修复

**日期**: 2025-08-21
**状态**: ✅ 已完成

## 问题背景

服务器GLM渠道调用时出现 `'str object' has no attribute 'items'` 错误，导致500错误。虽然该错误实际来自后端sglang服务，但排查过程发现了New API自身的错误处理逻辑缺陷。

## 发现的逻辑问题

1. **RelayErrorHandler架构缺陷**: 预创建不完整的错误对象，`RelayError` 为 `nil` 但 `ErrorType` 被设置
2. **对象生命周期管理**: 错误对象创建和替换逻辑不一致，存在使用不完整对象的风险
3. **类型安全隐患**: `ToOpenAIError()` 类型断言时可能访问 `nil` 指针
4. **模型支持不完整**: GLM-4.5系列模型未在智谱渠道支持列表中

## 修复方案

### 1. RelayErrorHandler重构 (`service/error.go`)

- 删除预创建的不完整NewAPIError对象
- 统一所有返回路径，直接返回完整构造的错误对象
- 避免RelayError为nil时的类型断言风险
- 简化错误处理逻辑，提高代码健壮性

### 2. GLM-4.5模型支持 (`relay/channel/zhipu/constants.go`)

- 添加GLM-4.5系列模型到智谱渠道支持列表
- 包含多种精度版本: fp8, fp16, int4
- 支持大小写格式兼容: glm-4.5 和 GLM-4.5

## 技术改进

- 消除错误对象生命周期管理问题
- 统一错误处理流程，减少代码复杂度
- 增强New API对最新智谱模型的兼容性
- 提高错误处理的类型安全和健壮性

## 影响文件

- `service/error.go` - 错误处理架构重构
- `relay/channel/zhipu/constants.go` - GLM-4.5模型支持
- `types/error.go` - 之前已有的防护措施

## 测试验证

- ✅ 编译成功，无语法错误
- ✅ 后端服务启动正常
- ✅ 错误处理逻辑更加健壮
- ✅ GLM-4.5模型得到正确支持

## 提交记录

- `d0f2230d` - RelayErrorHandler错误处理重构
- `f89c0b6c` - GLM-4.5模型支持和完整修复
