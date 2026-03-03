# 变更日志

本文档按时间倒序记录项目的所有重大变更。详细记录存储在 `docs/history/` 目录下。

---

## 2026年

### 2026-03-03 - 上游选择性合并（13个提交）🚀
**类型**: 上游合并
**影响**: Header Overrides System + 12项核心修复
**状态**: ✅ 已完成，36/36测试通过

[详细记录](history/upstream-merges/2026-03-03-selective-cherry-pick.md)

**亮点**:
- Header Overrides System：支持通配符、正则表达式、运行时动态修改
- 13个核心修复涵盖安全、性能、兼容性
- 零破坏性改动，完美向后兼容
- 跳过8个有冲突的提交（保守策略）

---

## 2025年

### 2025-11-18 - Token独立额度测试完整性修复
**类型**: Bug修复
**影响**: V1+V2 API测试全部通过（36/36）
**状态**: ✅ 已完成

[详细记录](history/bug-fixes/2025-11-18-token-quota-test-completeness.md)

**修复内容**:
- V1 API测试修复：3处JSON路径错误
- V2 API测试修复：首次授权逻辑、logs数组null问题
- API Bug修复：logs数组初始化为空数组而非null

---

### 2025-11-18 - Token消费回调功能 🔔
**类型**: 功能开发
**影响**: CEC平台集成支持
**状态**: ✅ 已完成

[详细记录](history/features/2025-11-18-token-consume-callback.md)

**核心特性**:
- 扩展V1 API，Token创建支持callback配置
- 异步goroutine发送回调，3秒超时
- HMAC-SHA256签名验证
- 符合KISS/YAGNI原则（仅记录日志，不重试）

---

### 2025-11-18 - API文档系统性整理 📚
**类型**: 功能开发（文档）
**影响**: 用户体验大幅改善
**状态**: ✅ 已完成

[详细记录](history/features/2025-11-18-api-documentation-reorganization.md)

**成果**:
- 创建统一入口文档（external-api-overview.md）
- V1/V2文档优化：新增快速开始章节、9个FAQ
- 整理测试资产到docs/testing/
- 文档导航链接、决策矩阵、测试索引

---

### 2025-11-05 - V2消费流水Token显示修复
**类型**: Bug修复
**影响**: V2 API消费流水接口
**状态**: ✅ 已完成

[详细记录](history/bug-fixes/2025-11-05-v2-token-key-display.md)

**修复内容**:
- 修复token_key显示优先级（实际密钥 > Token名称）
- 返回完整Token密钥而非名称
- 便于对方平台匹配识别

---

### 2025-11-05 - Token管理功能 🔑
**类型**: 功能开发
**影响**: 新增Token列表查询和验证接口
**状态**: ✅ 已完成

[详细记录](history/features/2025-11-05-token-management.md)

**核心接口**:
- GET /api/user/external/:external_user_id/tokens - Token列表查询
- POST /api/user/external/token/verify - Token有效性验证
- 脱敏显示、详细状态信息、兼容多种Token格式

---

### 2025-11-05 - 上游选择性集成
**类型**: 上游合并
**影响**: OpenAI音频计费修复
**状态**: ✅ 1/3完成（保守策略）

[详细记录](history/upstream-merges/2025-11-05-selective-integration.md)

**策略**:
- 成功集成：OpenAI音频计费修复
- 跳过：Claude 1h缓存（7文件冲突）、渠道权重逻辑（代码冲突）
- 集成率：33.3%（稳定性优先）

---

### 2025-09-30 - v0.9.1.4版本合并
**类型**: 上游合并
**影响**: Passkey无密码登录 + Claude Sonnet 4.5 + 41项改进
**状态**: ✅ 已完成，零冲突

[详细记录](history/upstream-merges/2025-09-30-v0.9.1.4-merge.md)

**亮点**:
- WebAuthn支持、2FA验证系统
- Claude Sonnet 4.5支持
- 41个提交，净增长+3,667行

---

### 2025-09-30 - 充值日志修复和开发环境优化 🔧
**类型**: Bug修复
**影响**: 充值记录显示、Make命令、开发环境
**状态**: ✅ 已完成

[详细记录](history/bug-fixes/2025-09-30-topup-log-makefile-optimization.md)

**修复内容**:
- 充值日志显示金额（修复quota字段记录）
- Makefile优化（make stop、make start-daemon、.env.dev）
- 开发指南v3.1和v3.2更新（350+行新增）

---

### 2025-09-26 - Amazon Nova模型支持合并
**类型**: 上游合并
**影响**: Amazon Nova + 50+改进
**状态**: ✅ 已完成，Fast-forward合并

[详细记录](history/upstream-merges/2025-09-26-amazon-nova-merge.md)

**改进**:
- Amazon Nova模型支持
- 2FA双因子认证系统
- 支付系统优化、错误处理重构
- SSRF防护、UI/UX增强

---

### 2025-09-17 - 微信小程序集成方案
**类型**: 功能开发
**影响**: OpenID优先匹配策略、账号统一
**状态**: ✅ 已完成

[详细记录](history/features/2025-09-17-wechat-miniprogram-integration.md)

**核心逻辑**:
- OpenID唯一性：同一OpenID只对应一个账号
- 首次优先：谁先注册谁是主账号
- 平台透明：各平台独立external_user_id，但指向相同用户数据

---

### 2025-08-21 - GLM模型错误处理和模型支持修复
**类型**: Bug修复
**影响**: RelayErrorHandler重构、GLM-4.5支持
**状态**: ✅ 已完成

[详细记录](history/bug-fixes/2025-08-21-glm-error-handling.md)

**修复内容**:
- RelayErrorHandler架构重构（消除nil RelayError风险）
- 添加GLM-4.5系列模型支持（fp8, fp16, int4）
- 增强错误处理类型安全和健壮性

---

### 2025-08-20 - GLM模型调用Panic错误修复
**类型**: Bug修复
**影响**: 修复500 panic错误
**状态**: ✅ 已完成

[详细记录](history/bug-fixes/2025-08-20-glm-panic-fix.md)

**根本原因**:
- ErrorType强制设置导致类型不一致
- ToOpenAIError()访问nil RelayError导致panic

**修复方案**:
- 修复错误对象创建逻辑
- 添加nil检查防护措施

---

### 2025-08-18 - external_user_id唯一索引冲突问题
**类型**: Bug修复
**影响**: 修复普通用户注册失败问题
**状态**: ✅ 已完成

[详细记录](history/bug-fixes/2025-08-18-external-user-id-unique-index.md)

**根本原因**:
- external_user_id唯一索引导致多个空值冲突

**修复方案**:
- 改为普通索引
- 应用层唯一性检查（IsExternalUserIdAlreadyTaken）

---

## 历史记录目录结构

```
docs/history/
├── bug-fixes/              # Bug修复记录
│   ├── 2025-08-18-external-user-id-unique-index.md
│   ├── 2025-08-20-glm-panic-fix.md
│   ├── 2025-08-21-glm-error-handling.md
│   ├── 2025-09-30-topup-log-makefile-optimization.md
│   ├── 2025-11-05-v2-token-key-display.md
│   └── 2025-11-18-token-quota-test-completeness.md
├── features/               # 功能开发记录
│   ├── 2025-09-17-wechat-miniprogram-integration.md
│   ├── 2025-11-05-token-management.md
│   ├── 2025-11-18-api-documentation-reorganization.md
│   └── 2025-11-18-token-consume-callback.md
└── upstream-merges/        # 上游合并记录
    ├── 2025-09-26-amazon-nova-merge.md
    ├── 2025-09-30-v0.9.1.4-merge.md
    ├── 2025-11-05-selective-integration.md
    └── 2026-03-03-selective-cherry-pick.md
```

---

## 统计数据

**总计**:
- Bug修复：6个
- 功能开发：4个
- 上游合并：4个

**最近3个月活跃度**（2025-11-18起算）:
- 2025-11月：5个重大变更
- 2025-09-10月：5个重大变更
- 2025-08月：3个重大变更

---

*最后更新：2026-03-03*
