# external_user_id 唯一索引冲突问题

**日期**: 2025-08-18
**状态**: ✅ 已完成

## 问题描述

普通用户注册时出现 `Error 1062: Duplicate entry '' for key 'users.idx_users_external_user_id'` 错误，导致多个用户无法同时注册。

## 根本原因

- `external_user_id` 字段设置了唯一索引约束
- 普通用户注册时该字段为空字符串，导致多个空值违反唯一性约束

## 修复方案

### 1. 代码层修复

- 将 `external_user_id` 从 `uniqueIndex` 改为普通 `index` (model/user.go:32)
- 新增 `IsExternalUserIdAlreadyTaken()` 函数处理应用层唯一性检查 (model/user.go:825-832)
- 优化外部用户同步逻辑，增强错误处理 (controller/external_user.go)

### 2. 数据库层修复

- 删除唯一索引：`DROP INDEX idx_users_external_user_id ON users`
- 重建普通索引：`CREATE INDEX idx_users_external_user_id ON users(external_user_id)`

### 3. 数据库迁移更新

- 更新 `scripts/init-external-user-db.sql` 以创建普通索引而非唯一索引

## 测试验证

- ✅ 多个普通用户可同时注册（external_user_id 为空）
- ✅ 外部用户同步正常工作（external_user_id 有值且唯一）
- ✅ 无重复键值冲突错误
- ✅ API响应正常返回JSON格式

## 影响文件

- `model/user.go`
- `controller/external_user.go`
- `scripts/init-external-user-db.sql`
