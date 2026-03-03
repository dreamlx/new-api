# New API 二次开发记忆

## 项目概述
基于 New API 进行二次开发，集成自定义前端用户管理系统。保留 New API 的 LLM 网关和计费功能，使用外部用户系统替代原有的用户管理。

**这是Claude Code的核心导航文件，永远最先读取。**

---

## 🧠 强制人格激活（触发机制）
- 置信度评级: 对每个结论给出置信度(1-5分)

**每次开始时必须先激活专家人格：**

1. **读取人格文件**: `CLAUDE-PERSONA.md`
2. **激活思维模式**: 🧠 → 🎯 → ⚡
3. **建立行为偏好**: 系统性分析 > 直接行动

**用户触发词（有效机制）:**
- "专家模式" → 重读CLAUDE-PERSONA.md
- "深入思考" → 激活系统性分析，不立即修复
- "不对，有问题" → 重新评估当前任务
- "等一下" → 暂缓当前操作，反思操作并提问

---

## Epic索引（产品规划）

### 已完成Epic
- [Epic-001: 外部用户系统集成](docs/product/epics/epic-001-external-user/) ✅ 2025-08-21完成
  - 实现external_user_id映射机制
  - 7个核心API接口
  - 充值计费系统集成

- [Epic-002: 微信小程序集成](docs/product/epics/epic-002-wechat-miniprogram/) ✅ 2025-09-17完成
  - OpenID优先匹配策略
  - 多平台账号统一
  - UnionID扩展支持

### 进行中Epic
- 暂无

### 计划中Epic
- 暂无

**说明**: 新功能开发请先创建Epic规划，使用模板：`docs/templates/epic-template.md`

---

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
CREATE INDEX idx_users_external_user_id ON users(external_user_id);
```

#### 2. 身份验证替代方案
**替代 Session 认证**：
- 原系统：基于 session 的用户身份验证
- 新方案：基于 `external_user_id` 的 API 映射查询
- 实现：前端传递外部用户ID，后端通过映射关系获取 New API 用户信息

#### 3. 核心 API 接口

**V1 API（用户+Token模式）**：
```
POST /api/user/external/sync     - 用户同步
POST /api/user/external/topup    - 用户充值
POST /api/user/external/token    - Token创建（独立额度）
GET  /api/user/external/:id      - 用户信息查询
GET  /api/user/external/:id/stats - 用户统计
GET  /api/user/external/:id/logs  - 消费记录查询
```

**V2 API（平台集成模式）**：
```
POST /api/v2/:platform_id/authorize - Token授权（无限额度）
GET  /api/v2/:platform_id/logs       - 消费流水查询
GET  /api/v2/:platform_id/balance    - 余额查询
```

**完整文档**: [API总览](docs/external-api-overview.md) | [V1文档](docs/external-user-api.md) | [V2文档](docs/external-user-api-v2.md)

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

---

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

---

## 开发进度

### 已完成功能 ✅
- [x] 用户模型扩展 (model/user.go) - 添加 external_user_id 字段
- [x] **V1 API**（7个核心接口）- 用户+Token模式，独立额度
- [x] **V2 API**（3个核心接口）- 平台集成模式，无限额度
- [x] 路由配置 (router/api-router.go) - 外部用户相关路由
- [x] API文档完善 - 统一入口、V1文档、V2文档、测试资产索引
- [x] 单元测试和集成测试用例 - V1: 19/19, V2: 17/17 全部通过
- [x] **Token管理功能** (2025-11-05) - Token列表查询和验证接口
- [x] **Token消费回调功能** (2025-11-18) - CEC平台集成支持

### 待完成功能 📋
- [ ] 性能优化和安全加固
- [ ] 生产环境部署配置
- [ ] 监控和日志系统集成

### 技术栈
- **后端**：Go + Gin + GORM
- **数据库**：MySQL/PostgreSQL/SQLite
- **前端集成**：JavaScript SDK
- **认证方式**：External User ID Mapping

---

## 当前开发环境状态

### 运行模式
- **数据库服务**：使用 `docker-compose.db-only.yml` 启动 MySQL + Redis
- **后端服务**：使用 `make start` 本地运行 Go 服务
- **前端服务**：未启动（开发阶段专注后端API）

### 服务信息
- **Go 后端**：运行在 `localhost:3000`
- **MySQL 数据库**：Docker容器 `mysql-dev`，端口 `localhost:3307`
- **Redis 缓存**：Docker容器 `redis-dev`，端口 `localhost:6379`
- **环境配置**：使用 `.env.dev` 文件加载环境变量

### 数据库配置
- **连接信息**：`root:dev123456@tcp(localhost:3307)/new_api_dev`
- **渠道配置**：1个启用渠道(id=1, name="ds", type=43)
- **支持模型**：`deepseek-chat,deepseek-reasoner`
- **默认测试模型**：`deepseek-chat`

---

## 重要决策记录
1. **计费策略**：采用方案1 - 前端处理货币转换，后端只接收美元
2. **安全策略**：IP白名单由Nginx处理，不在代码中实现
3. **支付集成**：支持灵活的payment_id，不限制特定支付平台
4. **认证方式**：使用external_user_id映射替代session认证
5. **账号统一策略**：采用OpenID优先匹配方案，确保用户在多平台间账号统一（2025-09-17）

---

## Docker管理最佳实践 🐳

**重要提醒**: 避免使用 `killall` 强制终止 Docker Desktop，应优先使用优雅关闭方式。

### ✅ 正确做法

#### 1. 优雅停止容器
```bash
# 停止单个容器
docker stop <container_name>

# 停止所有运行中的容器
docker stop $(docker ps -q)

# 停止docker-compose管理的服务
docker compose down
docker compose -f docker-compose.db-only.yml down
```

#### 2. 优雅重启 Docker Desktop（macOS）
```bash
# 方法1：使用osascript（推荐）
osascript -e 'quit app "Docker"'
sleep 3
open -a Docker

# 方法2：通过菜单操作
# Docker Desktop → Quit Docker Desktop
# 然后从应用程序重新启动
```

#### 3. 检查Docker状态
```bash
# 检查Docker daemon是否就绪
docker ps

# 检查容器状态
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

# 检查特定容器
docker ps --filter "name=mysql-dev" --filter "name=redis-dev"
```

### 💡 开发环境启动流程

```bash
# 1. 确保Docker Desktop运行
docker ps > /dev/null 2>&1 || open -a Docker

# 2. 等待Docker daemon就绪
for i in {1..30}; do
    docker ps > /dev/null 2>&1 && break || sleep 2
done

# 3. 启动数据库服务
docker compose -f docker-compose.db-only.yml up -d

# 4. 检查容器健康状态
docker ps --filter "name=mysql-dev" --filter "name=redis-dev"

# 5. 启动后端服务
make start
# 或后台运行：make start-daemon
```

### 📊 容器健康检查

```bash
# 检查容器健康状态（healthy标志）
docker ps --format "table {{.Names}}\t{{.Status}}"

# 查看容器日志
docker logs mysql-dev --tail 50
docker logs redis-dev --tail 50

# 进入容器调试
docker exec -it mysql-dev mysql -u root -pdev123456
docker exec -it redis-dev redis-cli
```

---

## 最近3个月变更摘要

### 2026-03-03 - 上游选择性合并（Header Overrides） 🚀
**重大功能**: Header Overrides System
- 支持通配符(*)和正则表达式转发请求头
- 运行时动态修改请求头
- 13个核心修复，零破坏性改动
- 测试结果：36/36全部通过

[详细记录](docs/history/upstream-merges/2026-03-03-selective-cherry-pick.md)

---

### 2025-11-18 - Token消费回调功能 🔔
**新功能**: CEC平台集成支持
- Token创建支持callback_url配置
- 异步goroutine发送消费通知
- HMAC-SHA256签名验证
- 符合KISS/YAGNI原则

[详细记录](docs/history/features/2025-11-18-token-consume-callback.md)

---

### 2025-11-18 - API文档系统性整理 📚
**改进**: 用户体验大幅提升
- 创建统一入口文档（external-api-overview.md）
- V1/V2文档优化：快速开始章节、9个FAQ
- 整理测试资产到docs/testing/

[详细记录](docs/history/features/2025-11-18-api-documentation-reorganization.md)

---

### 2025-11-05 - Token管理功能 🔑
**新接口**: Token列表查询和验证
- GET /api/user/external/:external_user_id/tokens
- POST /api/user/external/token/verify
- 脱敏显示、详细状态、兼容多种格式

[详细记录](docs/history/features/2025-11-05-token-management.md)

---

### 2025-09-30 - 充值日志修复和环境优化 🔧
**修复**: 充值记录显示、Make命令优化
- 修复充值日志quota字段记录
- make stop完整停止、make start-daemon后台运行
- 开发指南v3.1和v3.2更新

[详细记录](docs/history/bug-fixes/2025-09-30-topup-log-makefile-optimization.md)

---

## 完整历史记录

查看完整的变更历史和详细记录：**[CHANGELOG.md](docs/CHANGELOG.md)**

**历史记录目录结构**:
```
docs/history/
├── bug-fixes/              # 6个Bug修复记录
├── features/               # 4个功能开发记录
└── upstream-merges/        # 4个上游合并记录
```

**测试资产**: [docs/testing/README.md](docs/testing/README.md)

---

*最后更新：2026-03-03*
*文档版本：v2.0 - 精简版（~300行）*
