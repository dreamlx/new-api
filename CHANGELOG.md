# 更新日志 / Changelog

本文档记录了 New API 二次开发项目的所有重要变更。

项目遵循 [语义化版本](https://semver.org/lang/zh-CN/) 规范。

---

## [未发布] - 开发中

### 新增功能 ✨
- 外部用户系统集成
  - 外部用户同步 API (`POST /api/user/external/sync`)
  - 外部用户充值 API (`POST /api/user/external/topup`)
  - 外部用户 Token 管理 API (`POST /api/user/external/token`, `DELETE /api/user/external/token`)
  - 外部用户统计查询 (`GET /api/user/external/{id}/stats`)
  - 外部用户消费记录查询 (`GET /api/user/external/{id}/logs`)
  - 外部用户模型列表查询 (`GET /api/user/external/models`)

### 数据库变更 💾
- 扩展 `users` 表，新增外部用户系统字段：
  - `external_user_id`: 外部用户ID
  - `phone`: 手机号
  - `wechat_openid`: 微信OpenID
  - `wechat_unionid`: 微信UnionID  
  - `alipay_userid`: 支付宝用户ID
  - `login_type`: 登录类型
  - `is_external`: 是否外部用户
  - `external_data`: 外部用户扩展数据

### 开发工具 🛠️
- 新增基于 Docker Compose 的开发环境
- 新增 Makefile 开发工作流
- 完善的 API 文档和测试指南
- 单元测试和集成测试覆盖

---

## [2025-08-18] - Bug 修复

### 修复问题 🐛
- **修复 external_user_id 唯一索引冲突问题**
  - **问题**: 普通用户注册时出现 `Error 1062: Duplicate entry '' for key 'users.idx_users_external_user_id'` 错误
  - **根因**: `external_user_id` 字段的唯一索引约束导致多个空值冲突
  - **解决**: 将唯一索引改为普通索引，在应用层处理唯一性验证
  - **影响**: 修复了多用户注册失败问题，确保普通用户和外部用户都能正常注册

### 变更内容 📝
- `model/user.go:32`: 将 `external_user_id` 字段索引从 `uniqueIndex` 改为 `index`
- `model/user.go:825-832`: 新增 `IsExternalUserIdAlreadyTaken()` 函数
- `controller/external_user.go`: 优化外部用户同步逻辑
- `scripts/init-external-user-db.sql`: 更新数据库迁移脚本

### 测试验证 ✅
- 多个普通用户可同时注册
- 外部用户同步功能正常
- API 正常返回 JSON 响应
- 无数据库约束冲突错误

---

## 版本说明

- **[未发布]**: 当前开发分支的新功能
- **[日期]**: 已发布版本或重要修复的日期

## 贡献指南

更新此文档时，请遵循以下格式：
- 使用反向时间顺序（最新在顶部）
- 按类型分组变更：新增功能、修复问题、性能优化、安全更新等
- 包含影响的文件路径和代码行号
- 简洁描述变更内容和影响范围

---

*最后更新：2025-08-18*