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
- [x] 外部用户消费记录 API (controller/external_user.go) - GET /api/user/external/{id}/logs
- [x] 路由配置 (router/api-router.go) - 外部用户相关路由
- [x] 数据库迁移脚本整合到代码中 (scripts/init-db.sh, scripts/init-external-user-db.sql)
- [x] API文档完善 (docs/external-user-api.md) - 包含消费记录查询接口
- [x] 开发指南文档 (docs/development-guide.md) - 基于 Make + Docker Compose 工作流
- [x] curl测试指南 (docs/curl-testing-guide.md) - 包含消费记录测试用例
- [x] 单元测试和集成测试用例 (controller/external_user_test.go) - 覆盖所有API
- [x] 错误处理和边界情况优化 - 详细错误信息和参数验证
- [x] API接口功能测试和验证 - 全部通过
- [x] **BUG修复**: external_user_id 唯一索引冲突问题 - 修复普通用户注册失败问题

### 待完成功能 📋
- [ ] 性能优化和安全加固
- [ ] 生产环境部署配置
- [ ] 监控和日志系统集成

### 技术栈
- **后端**：Go + Gin + GORM
- **数据库**：MySQL/PostgreSQL/SQLite
- **前端集成**：JavaScript SDK
- **认证方式**：External User ID Mapping

## 当前开发环境状态

### 运行模式
- **数据库服务**：使用 `docker-compose.db-only.yml` 启动 MySQL + Redis
- **后端服务**：使用 `make start-backend` 本地运行 Go 服务
- **前端服务**：未启动（开发阶段专注后端API）

### 服务信息
- **Go 后端**：运行在 `localhost:3000`，进程ID: 40357
- **MySQL 数据库**：Docker容器 `mysql-dev`，端口 `localhost:3307`
- **Redis 缓存**：Docker容器 `redis-dev`，端口 `localhost:6379`
- **环境配置**：使用 `.env.dev` 文件加载环境变量

### 数据库配置
- **连接信息**：`root:dev123456@tcp(localhost:3307)/new_api_dev`
- **渠道配置**：1个启用渠道(id=1, name="ds", type=43)
- **支持模型**：`deepseek-chat,deepseek-reasoner`
- **默认测试模型**：`deepseek-chat`

### 当前问题
- balance_capacity API 返回 models_available=0，未显示具体模型信息
- 需要调试为什么模型倍率检查失败

### 下一步任务
- 修复 balance_capacity 中模型显示问题
- 确保 deepseek-chat 优先显示
- 完成所有功能测试

## 重要决策记录
1. **计费策略**：采用方案1 - 前端处理货币转换，后端只接收美元
2. **安全策略**：IP白名单由Nginx处理，不在代码中实现
3. **支付集成**：支持灵活的payment_id，不限制特定支付平台
4. **认证方式**：使用external_user_id映射替代session认证

## Bug修复记录

### 2025-08-20: GLM模型调用Panic错误修复 🐛➜✅

**问题描述**:
调用 GLM-4.5 等智谱模型时出现 `interface conversion: interface {} is nil, not types.OpenAIError` 错误，导致 500 panic。切换到 OpenRouter 的 GLM-4.5 能正常工作。

**根本原因**:
- `service/error.go:108-109` 中错误处理逻辑有严重缺陷
- 先用 `NewErrorWithStatusCode()` 创建错误对象（`RelayError = nil`）
- 然后强制设置 `ErrorType = ErrorTypeOpenAIError`，造成类型不一致
- 调用 `ToOpenAIError()` 时尝试访问 `nil` 的 `RelayError` 导致 panic

**修复方案**:
1. **修复根本问题** (`service/error.go:108-113`):
   - 删除错误的 `ErrorType` 强制设置
   - 正确构造 `OpenAIError` 对象并使用 `WithOpenAIError()` 创建错误

2. **添加防护措施** (`types/error.go:108-116`):
   - 在 `ToOpenAIError()` 中添加 `nil` 检查
   - 当 `RelayError` 为 `nil` 时返回通用错误格式，避免 panic

**影响文件**:
- `service/error.go` - 修复错误处理逻辑
- `types/error.go` - 添加防护措施

**测试验证**:
- ✅ 编译成功，无语法错误
- ✅ 后端服务启动正常
- ✅ 所有API路由正确注册
- ✅ 修复GLM等模型调用的panic问题

---

### 2025-08-18: external_user_id 唯一索引冲突问题 🐛➜✅

**问题描述**:
普通用户注册时出现 `Error 1062: Duplicate entry '' for key 'users.idx_users_external_user_id'` 错误，导致多个用户无法同时注册。

**根本原因**:
- `external_user_id` 字段设置了唯一索引约束
- 普通用户注册时该字段为空字符串，导致多个空值违反唯一性约束

**修复方案**:
1. **代码层修复**:
   - 将 `external_user_id` 从 `uniqueIndex` 改为普通 `index` (model/user.go:32)
   - 新增 `IsExternalUserIdAlreadyTaken()` 函数处理应用层唯一性检查 (model/user.go:825-832)
   - 优化外部用户同步逻辑，增强错误处理 (controller/external_user.go)

2. **数据库层修复**:
   - 删除唯一索引：`DROP INDEX idx_users_external_user_id ON users`
   - 重建普通索引：`CREATE INDEX idx_users_external_user_id ON users(external_user_id)`

3. **数据库迁移更新**:
   - 更新 `scripts/init-external-user-db.sql` 以创建普通索引而非唯一索引

**测试验证**:
- ✅ 多个普通用户可同时注册（external_user_id 为空）
- ✅ 外部用户同步正常工作（external_user_id 有值且唯一）
- ✅ 无重复键值冲突错误
- ✅ API响应正常返回JSON格式

**影响文件**:
- `model/user.go`
- `controller/external_user.go`  
- `scripts/init-external-user-db.sql`

---
*最后更新：2025-08-18*