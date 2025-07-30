# New API 二次开发记忆

## 项目概述
基于 New API 进行二次开发，集成自定义前端用户管理系统。保留 New API 的 LLM 网关和计费功能，使用外部用户系统替代原有的用户管理。

## 核心设计方案

### 用户系统集成策略
- **前端用户系统**：支持微信登录、支付宝登录、短信登录、邮箱登录
- **New API 后端**：作为 LLM 网关和计费系统
- **映射机制**：通过 `external_user_id` 字段建立前端用户与 New API 用户的关联关系

### 关键技术方案

#### 1. 数据库扩展
扩展 `users` 表，新增字段：
```sql
ALTER TABLE users ADD COLUMN phone VARCHAR(20) DEFAULT '';
ALTER TABLE users ADD COLUMN wechat_openid VARCHAR(100) DEFAULT '';
ALTER TABLE users ADD COLUMN wechat_unionid VARCHAR(100) DEFAULT '';
ALTER TABLE users ADD COLUMN alipay_userid VARCHAR(100) DEFAULT '';
ALTER TABLE users ADD COLUMN external_user_id VARCHAR(100) DEFAULT '';
ALTER TABLE users ADD COLUMN login_type VARCHAR(20) DEFAULT 'email';
ALTER TABLE users ADD COLUMN is_external BOOLEAN DEFAULT false;
ALTER TABLE users ADD COLUMN external_data TEXT;

-- 创建索引
CREATE UNIQUE INDEX idx_users_external_user_id ON users(external_user_id);
```

#### 2. 身份验证替代方案
**替代 Session 认证**：
- 原系统：基于 session 的用户身份验证
- 新方案：基于 `external_user_id` 的 API 映射查询
- 实现：前端传递外部用户ID，后端通过映射关系获取 New API 用户信息

#### 3. 核心 API 接口

**用户同步接口**：
```
POST /api/user/external/sync
- 同步外部用户到 New API 系统
- 支持创建新用户和更新现有用户信息
```

**用户充值接口**：
```
POST /api/user/external/topup
- 基于 external_user_id 的充值接口
- 美元金额自动转换为 quota (1 USD = 500,000 quota)
```

**用户信息查询**：
```
GET /api/user/external/{external_user_id}
- 根据外部用户ID获取 New API 用户信息
- 返回 quota、使用统计等信息
```

#### 4. 计费系统兼容性
- **New API 计费机制**：`QuotaPerUnit = 500,000` ($0.002 / 1K tokens)
- **美元充值集成**：前端美元充值 → quota 转换 → New API 计费
- **完全兼容**：无需修改现有计费逻辑

### 集成流程

#### 用户注册/登录流程
1. 用户在前端系统完成注册/登录（微信/支付宝/短信/邮箱）
2. 前端调用 `/api/user/external/sync` 同步用户到 New API
3. New API 创建或更新用户记录，建立 `external_user_id` 映射

#### 充值流程
1. 用户在前端完成美元充值
2. 前端调用 `/api/user/external/topup` 
3. New API 自动转换美元为 quota 并记录充值日志

#### API 访问流程
1. 前端为用户生成 API Token（通过 external_user_id 映射）
2. 用户使用 Token 访问 LLM API
3. New API 按现有逻辑进行计费和使用统计

### 优势
- ✅ 保留完善的前端用户系统
- ✅ 复用 New API 成熟的 LLM 网关功能
- ✅ 数据同步简单可靠
- ✅ 计费系统完全兼容
- ✅ 支持所有现有登录方式
- ✅ 无需大幅修改后端架构

## Demo阶段核心功能清单

### 计费策略确认（方案1）
- **货币统一**：前端收款任意货币 → Stripe等支付网关转换 → 后端只接收美元
- **汇率处理**：完全由前端网站和支付网关负责，New API不处理汇率转换
- **计费逻辑**：$1 USD = 500,000 quota（使用 common.QuotaPerUnit）
- **优势**：简化架构、避免汇率同步、利用支付网关的实时汇率

### Access Key 管理简化  
- **默认权限**：Token创建时默认1年有效期
- **多Token支持**：用户可创建多个不同时间周期的Token
- **权限控制**：Demo阶段使用默认权限，不做复杂限制

## 开发进度

### 已完成功能 ✅
- [x] 用户模型扩展 (model/user.go) - 添加 external_user_id 字段
- [x] 外部用户同步 API (controller/external_user.go) - POST /api/user/external/sync
- [x] 外部用户充值 API (controller/external_user.go) - POST /api/user/external/topup  
- [x] 外部用户Token管理 API (controller/external_user.go) - POST /api/user/external/token
- [x] 外部用户统计 API (controller/external_user.go) - GET /api/user/external/{id}/stats
- [x] 路由配置 (router/api-router.go) - 外部用户相关路由
- [x] 数据库迁移脚本整合到代码中
- [x] API文档完善 (docs/external-user-api.md)
- [x] 开发指南文档 (docs/development-guide.md)

### 当前进行中 🔄
- [x] API接口功能测试和验证

### 待完成功能 📋
- [ ] 单元测试和集成测试用例
- [ ] 错误处理和边界情况优化
- [ ] 性能优化和安全加固
- [ ] 生产环境部署配置
- [ ] 监控和日志系统集成

### 技术栈
- **后端**：Go + Gin + GORM
- **数据库**：MySQL/PostgreSQL/SQLite
- **前端集成**：JavaScript SDK
- **认证方式**：External User ID Mapping

## 当前状态
- **开发阶段**：Demo阶段核心功能开发完成
- **测试状态**：API接口功能验证中
- **下一步**：单元测试和集成测试用例开发

## 重要决策记录
1. **计费策略**：采用方案1 - 前端处理货币转换，后端只接收美元
2. **安全策略**：IP白名单由Nginx处理，不在代码中实现
3. **支付集成**：支持灵活的payment_id，不限制特定支付平台
4. **认证方式**：使用external_user_id映射替代session认证

---
*最后更新：2025-01-30*