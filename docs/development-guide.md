# 外部用户系统集成 - 开发指南

## 📖 项目概述

基于 New API 的外部用户系统集成方案，使用 **Make + Docker Compose** 进行开发和部署管理。

### 核心功能
- ✅ 外部用户系统集成（微信、支付宝、邮箱、短信登录）
- ✅ 用户充值和计费管理（$1 = 500,000 quota）
- ✅ API Token 管理和权限控制
- ✅ 消费记录和余额查询
- ✅ 完整的 REST API 接口

### 技术栈
- **后端**：Go 1.19+ + Gin + GORM
- **前端**：React + Vite (使用 Bun 或 NPM)
- **数据库**：MySQL 8.2 + Redis
- **开发工具**：Make + Docker Compose
- **部署**：Docker 容器化部署

---

## 🛠️ 环境要求

### 必需软件
- **Docker** 和 **Docker Compose**：容器化开发环境
- **Go 1.19+**：后端开发
- **Make**：构建和任务管理
- **Git**：版本控制

### 可选软件
- **Bun**：前端构建工具（优先，更快）
- **Node.js 16+**：前端构建备选方案
- **MySQL 客户端**：数据库管理
- **VS Code / GoLand**：开发IDE

### 端口要求
确保以下端口可用：
- `3000` - 后端服务
- `3307` - MySQL
- `6379` - Redis

---

## 🚀 快速开始

### 1. 克隆项目

```bash
git clone <your-repo-url>
cd new-api

# 查看所有可用命令
make help
```

### 2. 启动开发环境

```bash
# 方式1：一键启动（推荐）
make dev         # 启动数据库 + 后端（自动使用 .env.dev 配置）

# 方式2：分步启动
make dev-db      # 1. 启动数据库（MySQL + Redis）
make start       # 2. 启动后端服务（前台运行，Ctrl+C 停止）
```

**提示**：
- 首次启动会自动从 `.env.dev` 加载配置
- 可以通过修改 `.env.dev` 自定义数据库连接等配置
- 使用 `Ctrl+C` 停止前台运行的后端服务

### 3. 初始化数据库（首次运行）

```bash
make db-init     # 初始化外部用户系统数据库
```

### 4. 访问应用

- **应用地址**：http://localhost:3000
- **管理界面**：http://localhost:3000/console
- **API文档**：参考 `docs/external-user-api.md`

---

## 📋 Make 命令参考

### 核心命令

```bash
make              # 显示帮助信息
make dev          # 一键启动开发环境（推荐，前台运行）
make start        # 启动后端服务（前台运行，使用 .env.dev 配置）
make start-backend # 启动后端服务（兼容旧版本，等同于 make start）
make start-daemon # 后台启动后端服务（服务器部署用）
make stop         # 停止所有服务（后端进程 + 数据库容器）
make status       # 查看服务状态
```

**命令说明**：
- `make dev`：最便捷的方式，自动启动数据库和后端（前台运行）
- `make start`：只启动后端，需要先运行 `make dev-db` 启动数据库（前台运行）
- `make start-backend`：旧版本命令别名，与 `make start` 完全相同
- `make start-daemon`：后台启动后端，适合服务器部署，日志保存到 `logs/app.log`
- `make stop`：会停止后端Go进程和数据库Docker容器，释放端口3000

### 开发环境管理

```bash
make dev-db       # 启动数据库服务（MySQL + Redis）
make logs         # 查看数据库日志
make stop         # 停止所有服务（后端 + 数据库）
make status       # 查看服务运行状态
```

### 构建相关

```bash
make build-frontend    # 构建前端（生产部署用）
make build-docker      # 构建 Docker 镜像
```

### 测试命令

```bash
make test              # 运行Go单元测试
make test-api          # 测试外部用户API
make test-user-story   # 测试完整业务流程
```

### 数据库管理

```bash
make db-init      # 初始化外部用户系统数据库（首次部署）
make db-backup    # 备份开发数据库
make db-reset     # 重置数据库（危险操作，需确认）
```

**重要说明**：
- ✅ **新增字段无需手动执行SQL**：项目使用 GORM 自动迁移
- ✅ **首次启动自动创建**：后端服务启动时会自动检测并创建新字段
- ✅ **增量更新安全**：已存在的字段不会重复创建
- ⚠️ `make db-init` 仅用于首次部署或需要确保外部用户字段存在时

### 其他命令

```bash
make sync         # 同步上游代码
make clean        # 清理Docker资源
```

---

## ⚙️ 环境变量配置

### 配置文件 `.env.dev`

开发环境使用 `.env.dev` 文件管理环境变量。`make start` 和 `make dev` 会自动加载此文件。

**默认配置**（`.env.dev`）：
```bash
SQL_DSN=root:dev123456@tcp(localhost:3307)/new_api_dev
REDIS_CONN_STRING=redis://localhost:6379
GIN_MODE=debug
TZ=Asia/Shanghai
ERROR_LOG_ENABLED=true
```

### 配置加载优先级

```bash
make start
└─> 检查 .env.dev 是否存在
    ├─> 存在：使用 .env.dev 配置 ✅
    └─> 不存在：使用 Makefile 硬编码默认值 ⚠️
```

### 自定义配置

如果需要修改配置（如数据库端口、Redis地址等）：

```bash
# 方式1：直接编辑 .env.dev（推荐）
vim .env.dev

# 方式2：创建本地副本（避免提交）
cp .env.dev .env.local
vim .env.local
# 注意：需要修改 Makefile 支持 .env.local
```

### 配置项说明

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `SQL_DSN` | MySQL连接字符串 | `root:dev123456@tcp(localhost:3307)/new_api_dev` |
| `REDIS_CONN_STRING` | Redis连接地址 | `redis://localhost:6379` |
| `GIN_MODE` | Gin运行模式 | `debug` (开发环境) |
| `TZ` | 时区设置 | `Asia/Shanghai` |
| `ERROR_LOG_ENABLED` | 是否启用错误日志 | `true` |

---

## 🗄️ 数据库迁移机制

### GORM 自动迁移

项目使用 **GORM AutoMigrate** 自动管理数据库结构变更，**无需手动执行SQL脚本**。

#### 工作原理

```
启动后端服务
  ↓
GORM 读取模型定义（model/*.go）
  ↓
检测数据库当前结构
  ↓
自动创建缺失的表和字段
  ↓
服务正常启动
```

#### 支持的操作

✅ **自动创建新表**：首次启动时创建所有表
✅ **自动添加新字段**：模型新增字段时自动创建
✅ **幂等性保证**：重复执行安全，不会重复创建
✅ **多数据库支持**：MySQL、PostgreSQL、SQLite

#### 涉及的模型

```go
// model/main.go
DB.AutoMigrate(
    &Channel{},
    &Token{},             // ✅ 包含 callback_url, callback_enabled, callback_secret
    &User{},              // ✅ 包含 external_user_id 等外部用户字段
    &Option{},
    &Redemption{},
    // ...
)
```

#### 最新字段变更（Callback功能）

**Token表新增字段**：
- `callback_url` - varchar(500) - 回调URL
- `callback_enabled` - boolean - 是否启用回调
- `callback_secret` - varchar(64) - 回调签名密钥

**迁移方式**：
```bash
# 无需任何操作，直接启动服务即可
make start

# 首次启动会自动创建这些字段
# 已存在的字段不会重复创建
```

#### 验证字段是否创建

```bash
# 方式1：通过MySQL客户端
mysql -h 127.0.0.1 -P 3307 -u root -pdev123456 new_api_dev \
  -e "DESCRIBE tokens;" | grep callback

# 方式2：查看后端启动日志
# 应该看到类似输出：
# [GORM] migrating schema for Token
```

#### 何时需要手动SQL脚本？

**几乎不需要**，除非：
- ⚠️ 首次部署需要确保外部用户字段（可选）
- ⚠️ 需要创建特殊索引（GORM自动创建常规索引）
- ⚠️ 需要修改已存在字段的类型（需谨慎操作）

---

## 🔄 Token独立额度迁移

### 重要说明

**Token独立额度机制**是一个重大架构变更，涉及：
1. **代码逻辑变更**：从共享用户余额改为Token独立额度
2. **数据迁移**：需要将现有Token的余额从用户迁移到Token

### 迁移前后对比

**迁移前（共享用户余额）**：
```
User.Quota = $100
  ↓
多个Token共享这$100
  ↓
任何Token消费都扣User.Quota
```

**迁移后（Token独立额度）**：
```
User.Quota = $100 → 分配给Token
  ↓
Token1.RemainQuota = $20
Token2.RemainQuota = $30
Token3.RemainQuota = $50
  ↓
每个Token消费扣自己的RemainQuota
```

### 何时需要迁移？

**新项目**：
- ✅ 无需迁移，直接使用最新代码
- ✅ 创建Token时分配`allocated_quota`即可

**现有项目升级**：
- ⚠️ **必须执行迁移脚本**
- ⚠️ 否则会出现"Token余额不足"错误
- ⚠️ 现有Token的`RemainQuota`为0，需要从用户余额分配

### 迁移步骤

#### 1. 开发环境迁移

```bash
# 进入项目目录
cd /Users/dreamlinx/Dropbox/Projects/NetBeansProjects/new-api

# 执行迁移脚本（会提示确认）
./scripts/migrate-token-quota.sh \
    -d mysql \
    -H localhost \
    -P 3307 \
    -u root \
    -D new_api_dev

# 输入数据库密码：dev123456
# 确认迁移：输入 yes
```

#### 2. 生产环境迁移（推荐步骤）

```bash
# 步骤1：Dry-run模拟执行，查看影响
./scripts/migrate-token-quota.sh \
    -d mysql \
    -H your-db-host \
    -P 3306 \
    -u root \
    -D new_api_prod \
    --dry-run

# 步骤2：生成SQL文件供审查
./scripts/migrate-token-quota.sh \
    -d mysql \
    -s /tmp/migration.sql

# 步骤3：备份数据库（自动备份到./backups）
# 或手动备份：
mysqldump -h host -u root -p new_api_prod > backup_$(date +%Y%m%d).sql

# 步骤4：执行迁移
./scripts/migrate-token-quota.sh \
    -d mysql \
    -H your-db-host \
    -P 3306 \
    -u root \
    -D new_api_prod

# 步骤5：验证迁移结果
mysql -h your-db-host -u root -p new_api_prod \
  -e "SELECT id, name, remain_quota FROM tokens LIMIT 5;"
```

### 迁移脚本功能

**自动处理**：
- ✅ 单Token用户：获得100%用户余额（最少$20）
- ✅ 多Token用户：按比例平均分配（每个最少$20）
- ✅ V2平台Token：自动跳过（UnlimitedQuota=true）
- ✅ 数据库备份：自动备份到`./backups`目录
- ✅ 验证输出：显示迁移统计信息

**详细文档**：
- 完整迁移指南：`docs/token-quota-migration-guide.md`
- 实施总结：`docs/token-independent-quota-implementation-summary.md`
- 测试脚本：`scripts/test-v1-token-quota.sh`

### 迁移后验证

```bash
# 1. 检查Token余额
mysql -h 127.0.0.1 -P 3307 -u root -pdev123456 new_api_dev \
  -e "SELECT id, name, remain_quota,
      CONCAT('$', ROUND(remain_quota/500000, 2)) as balance_usd
      FROM tokens WHERE unlimited_quota = 0 LIMIT 10;"

# 2. 运行自动化测试
bash scripts/test-v1-token-quota.sh

# 3. 测试完整业务流程
bash scripts/test-user-story.sh
```

### 常见问题

**Q: 新创建的Token需要迁移吗？**
- A: 不需要。使用最新代码创建Token时会自动设置`RemainQuota`

**Q: 迁移脚本是幂等的吗？**
- A: 不是。重复执行会再次分配额度，不推荐

**Q: 迁移失败怎么办？**
- A: 使用自动备份恢复：`mysql -u root -p db_name < backups/backup_xxx.sql`

**Q: V2 API的Token需要迁移吗？**
- A: 不需要。迁移脚本会自动跳过`UnlimitedQuota=true`的Token

---

## 💻 开发工作流

### 标准开发流程

```bash
# 1. 启动数据库
make dev-db

# 2. 初始化数据库（首次）
make db-init

# 3. 启动后端（前台运行，便于查看日志）
make start

# 4. 开发和测试
#    - 修改代码
#    - Ctrl+C 停止后端
#    - make start 重新启动

# 5. 运行测试
make test-user-story

# 6. 停止服务
make stop
```

### 完整开发循环示例

```bash
# 早上开始开发
make dev-db              # 启动数据库
make start               # 启动后端

# 开发过程中
# ... 编辑代码 ...
Ctrl+C                   # 停止后端
make start               # 重新启动

# 运行测试
make test-user-story     # 完整业务测试

# 下班前清理
Ctrl+C                   # 停止后端
make stop                # 停止数据库
```

### 前端开发

```bash
# 方式1：生产构建（后端嵌入）
make build-frontend

# 方式2：前端开发服务器（热重载）
cd web
npm run dev              # 或 bun run dev
# 前端：http://localhost:5173
# 后端API代理：http://localhost:3000
```

---

## 🗄️ 数据库配置

### 连接信息

使用 `make dev-db` 启动的数据库：

```
MySQL:
  主机: localhost
  端口: 3307
  用户: root
  密码: dev123456
  数据库: new_api_dev

Redis:
  主机: localhost
  端口: 6379
```

### 直接连接数据库

```bash
# MySQL
docker exec -it mysql-dev mysql -uroot -pdev123456 new_api_dev

# Redis
docker exec -it redis-dev redis-cli
```

### 数据库初始化内容

`make db-init` 会添加以下字段到 `users` 表：

- `external_user_id`：外部用户唯一标识（带索引）
- `phone`：手机号码
- `wechat_openid`：微信OpenID
- `wechat_unionid`：微信UnionID
- `alipay_userid`：支付宝用户ID
- `login_type`：登录类型
- `is_external`：是否外部用户标识
- `external_data`：扩展数据字段

---

## 🧪 测试指南

### 运行测试

```bash
# 单元测试
make test

# API集成测试
make test-api

# 完整业务流程测试
make test-user-story
```

### 手动API测试

```bash
# 1. 运行用户故事测试获取Token
make test-user-story

# 2. 使用返回的Access Key测试
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "Hello"}]
  }'

# 3. 查看消费记录
curl http://localhost:3000/api/user/external/alice_xxx/logs
```

### 健康检查

```bash
# 检查后端服务
curl http://localhost:3000/api/status

# 检查数据库连接
docker exec mysql-dev mysql -uroot -pdev123456 -e "SELECT VERSION();"
```

---

## 🐛 故障排除

### 常见问题

#### 1. 端口被占用（最常见）

**症状：** `bind: address already in use` 或 `Error starting userland proxy: listen tcp4 0.0.0.0:3000: bind: address already in use`

**原因：** `make stop` 没有正确停止后端 Go 进程

**解决：**
```bash
# 使用增强的 make stop（推荐）
make stop

# 如果还有问题，手动查找并停止占用进程
lsof -i :3000   # 查找占用3000端口的进程
lsof -i :3307   # 查找占用3307端口的进程（MySQL）
lsof -i :6379   # 查找占用6379端口的进程（Redis）

# 强制停止后端进程
pkill -f "go run main.go"

# 或强制停止指定端口
lsof -ti:3000 | xargs kill -9
```

**预防：**
- 使用 `Ctrl+C` 停止前台运行的后端
- 使用 `make stop` 而不是直接关闭终端

#### 2. 数据库连接失败

**症状：** `Error 2003: Can't connect to MySQL server`

**解决：**
```bash
# 检查容器状态
make status

# 重启数据库
make stop
make dev-db

# 查看数据库日志
make logs
```

#### 3. Go模块下载慢

**解决：**
```bash
# 使用国内镜像
export GOPROXY=https://goproxy.cn,direct
go mod download
```

#### 4. 环境变量未生效

**症状：** 修改了 `.env.dev` 但配置没有生效

**原因：** 需要重启后端服务才能加载新配置

**解决：**
```bash
# 停止后端（Ctrl+C 或 make stop）
Ctrl+C

# 重新启动
make start  # 会重新加载 .env.dev
```

**验证配置：**
```bash
# 查看 .env.dev 内容
cat .env.dev

# 启动时会显示
# 📋 使用 .env.dev 配置文件  ← 确认使用了配置文件
# ⚠️  .env.dev 不存在，使用默认配置  ← 提示文件不存在
```

#### 5. 充值记录显示金额为0

**症状：** 在 `/console/log` 页面充值记录金额显示为 $0

**原因：** 旧版本代码 bug，已在最新版本修复

**解决：**
```bash
# 确保使用最新代码
git pull origin main

# 重启服务
make stop
make dev

# 重新充值测试
bash scripts/test-user-story.sh
```

#### 6. 新字段未创建（数据库迁移问题）

**症状：**
- API报错 `Error 1054: Unknown column 'callback_url' in 'field list'`
- 或其他新字段不存在的错误

**原因：** GORM自动迁移未正常执行

**诊断：**
```bash
# 检查tokens表是否有callback字段
mysql -h 127.0.0.1 -P 3307 -u root -pdev123456 new_api_dev \
  -e "DESCRIBE tokens;" | grep callback

# 应该看到3个字段：
# callback_url
# callback_enabled
# callback_secret
```

**解决方式1（推荐）：重启后端**
```bash
# 停止服务
make stop

# 重新启动，GORM会自动创建缺失字段
make start

# 查看启动日志，确认迁移执行
# 应该看到类似：[GORM] migrating schema...
```

**解决方式2（手动创建，不推荐）：**
```bash
# 仅在自动迁移失败时使用
mysql -h 127.0.0.1 -P 3307 -u root -pdev123456 new_api_dev

# 手动添加callback字段
ALTER TABLE tokens ADD COLUMN callback_url VARCHAR(500) DEFAULT '';
ALTER TABLE tokens ADD COLUMN callback_enabled BOOLEAN DEFAULT false;
ALTER TABLE tokens ADD COLUMN callback_secret VARCHAR(64) DEFAULT '';
CREATE INDEX idx_tokens_callback_enabled ON tokens(callback_enabled);
```

**预防：**
- ✅ 拉取最新代码后，重启后端服务
- ✅ 不要手动修改数据库结构（让GORM管理）
- ✅ 首次部署可运行 `make db-init` 确保基础字段存在

**验证修复：**
- 充值日志应该显示正确的 `spend` 金额（负数表示充值）
- 例如：充值 $20 应显示为 `spend: -20`

#### 7. Token余额不足错误（Token独立额度迁移）

**症状：**
- API调用报错：`Token余额不足，当前余额: $0.00，需要: $0.01`
- 或报错：`Token余额不足 (insufficient token quota)`
- 用户明明充值过，但Token无法使用

**根本原因：**
- 项目从**共享用户余额模式**升级到**Token独立额度模式**
- 旧Token的`remain_quota`为0，需要执行数据迁移

**诊断：**
```bash
# 检查Token余额
mysql -h 127.0.0.1 -P 3307 -u root -pdev123456 new_api_dev \
  -e "SELECT id, name, remain_quota, unlimited_quota FROM tokens LIMIT 5;"

# 如果remain_quota全部为0，且unlimited_quota为0，需要迁移
```

**解决：执行Token独立额度迁移**
```bash
# 查看详细迁移指南
cat docs/token-quota-migration-guide.md

# 执行迁移脚本
./scripts/migrate-token-quota.sh \
    -d mysql \
    -H localhost \
    -P 3307 \
    -u root \
    -D new_api_dev

# 验证迁移成功
mysql -h 127.0.0.1 -P 3307 -u root -pdev123456 new_api_dev \
  -e "SELECT id, name, remain_quota,
      CONCAT('$', ROUND(remain_quota/500000, 2)) as balance_usd
      FROM tokens WHERE unlimited_quota = 0 LIMIT 5;"
```

**完整说明：** 参见 [Token独立额度迁移](#-token独立额度迁移) 章节

**预防：**
- ✅ 升级项目前阅读迁移指南
- ✅ 使用 `--dry-run` 模式预览影响
- ✅ 生产环境执行前先备份数据库

#### 6. 前端构建失败

**解决：**
```bash
# 清理并重新安装
cd web
rm -rf node_modules
npm install          # 或 bun install

# 重新构建
make build-frontend
```

### 日志查看

```bash
# 数据库日志
make logs

# 后端日志（前台运行时直接显示）
make start

# Docker容器日志
docker logs mysql-dev -f
docker logs redis-dev -f
```

---

## 📊 服务管理

### 查看服务状态

```bash
make status
```

**输出示例：**
```
📊 服务状态:
NAME        IMAGE          COMMAND                  SERVICE     STATUS         PORTS
mysql-dev   mysql:8.2      "docker-entrypoint.s…"   mysql-dev   Up 5 minutes   0.0.0.0:3307->3306/tcp
redis-dev   redis:latest   "docker-entrypoint.s…"   redis-dev   Up 5 minutes   0.0.0.0:6379->6379/tcp

后端服务: 运行中 (PID: 12345)
```

### 完全重置环境

```bash
# 停止所有服务
make stop

# 删除数据卷（清空所有数据）
docker compose -f docker-compose.db-only.yml down -v

# 重新启动
make dev-db
make db-init
make start
```

---

## 🔄 版本管理

### 同步上游代码

```bash
# 使用Make命令
make sync

# 或手动同步
git fetch upstream
git merge upstream/main
git push origin main
```

### Git工作流

```bash
# 创建功能分支
git checkout -b feature/new-api-endpoint

# 提交代码
git add .
git commit -m "feat: 添加新的API端点"

# 推送到远程
git push origin feature/new-api-endpoint

# 创建Pull Request
```

---

## 📚 相关文档

### 核心文档
- [外部用户API文档](./external-user-api.md) - 完整的API接口说明
- [curl测试指南](./curl-testing-guide.md) - API测试用例和示例
- [微信小程序集成](./wechat-miniprogram-integration.md) - 微信小程序接入指南

### 参考文档
- [上游开发指南](./upstream-dev-guide-reference.md) - New API 原生开发参考
- [路线图](./roadmap.md) - 项目开发规划

---

## 🖥️ 服务器后台运行

在服务器环境，`make dev` 和 `make start` 会前台运行，关闭终端会导致服务停止。以下是两种推荐的后台运行方案。

### 方案1：使用 make start-daemon（推荐）

**适用场景**：快速部署、开发测试服务器

```bash
# 1. 启动数据库
make dev-db

# 2. 后台启动后端
make start-daemon

# 输出：
# 🚀 后台启动服务...
# 📋 使用 .env.dev 配置文件
# ✅ 服务已在后台启动，PID: 12345
# 📋 查看日志: tail -f logs/app.log
# 🛑 停止服务: make stop

# 3. 查看日志
tail -f logs/app.log

# 4. 停止服务
make stop
```

**特点**：
- ✅ 自动使用 `.env.dev` 配置
- ✅ 日志输出到 `logs/app.log`
- ✅ PID 保存到 `.pid` 文件
- ✅ 使用 `make stop` 可正确停止

**查看进程状态**：
```bash
# 查看 PID
cat .pid

# 检查进程是否运行
ps -p $(cat .pid)

# 查看服务状态
make status
```

---

### 方案2：使用 tmux

**适用场景**：需要随时查看日志、临时部署

#### 安装 tmux

```bash
# Ubuntu/Debian
sudo apt install tmux

# CentOS/RHEL
sudo yum install tmux

# macOS
brew install tmux
```

#### 使用步骤

```bash
# 1. 创建命名会话
tmux new -s new-api

# 2. 在会话中启动服务
make dev-db
make start

# 3. 断开会话（服务继续运行）
# 按键：Ctrl+B，然后按 D

# 4. 重新连接会话
tmux attach -t new-api

# 或简写
tmux a -t new-api

# 5. 查看所有会话
tmux ls

# 6. 停止服务并退出会话
# 在会话中按 Ctrl+C 停止服务
# 然后输入 exit 退出

# 7. 强制删除会话
tmux kill-session -t new-api
```

#### tmux 快捷键

| 快捷键 | 功能 |
|--------|------|
| `Ctrl+B` `D` | 断开会话 |
| `Ctrl+B` `C` | 创建新窗口 |
| `Ctrl+B` `N` | 下一个窗口 |
| `Ctrl+B` `P` | 上一个窗口 |
| `Ctrl+B` `%` | 垂直分屏 |
| `Ctrl+B` `"` | 水平分屏 |
| `Ctrl+B` `方向键` | 切换分屏 |

**常用工作流**：
```bash
# 启动多个窗口管理不同服务
tmux new -s new-api

# 窗口0：数据库
make dev-db
# 按 Ctrl+B C 创建新窗口

# 窗口1：后端
make start
# 按 Ctrl+B C 创建新窗口

# 窗口2：查看日志
tail -f logs/app.log

# 切换窗口：Ctrl+B N (下一个) 或 Ctrl+B P (上一个)
```

---

### 方案对比

| 对比项 | make start-daemon | tmux |
|--------|-------------------|------|
| **安装要求** | 无需额外安装 | 需要安装 tmux |
| **易用性** | ⭐⭐⭐⭐⭐ 最简单 | ⭐⭐⭐ 需要学习快捷键 |
| **日志查看** | 文件日志，需要 tail | 实时日志，随时查看 |
| **重启服务** | make stop && make start-daemon | Ctrl+C 后重新启动 |
| **进程管理** | 通过 .pid 文件 | 通过 tmux 会话 |
| **适用场景** | 生产/测试服务器 | 开发/临时部署 |

---

### 其他方案参考

#### screen（类似 tmux）

```bash
# 创建会话
screen -S new-api

# 启动服务
make dev

# 断开：Ctrl+A D
# 重连：screen -r new-api
# 查看：screen -ls
```

#### systemd（生产环境推荐）

参考项目根目录的 `one-api.service` 文件，需要先编译程序：

```bash
# 1. 编译
go build -o one-api main.go

# 2. 配置 systemd 服务
sudo cp one-api.service /etc/systemd/system/
sudo nano /etc/systemd/system/one-api.service
# 修改路径和用户名

# 3. 启动
sudo systemctl daemon-reload
sudo systemctl start one-api
sudo systemctl enable one-api
sudo systemctl status one-api
```

---

## 🚢 生产部署

### 构建生产镜像

```bash
# 使用Make命令
make build-docker

# 或手动构建
docker build -t new-api:latest .
```

### Docker Compose 部署

```bash
# 编辑生产配置
vi docker-compose.prod.yml

# 启动生产环境
docker-compose -f docker-compose.prod.yml up -d

# 查看日志
docker-compose -f docker-compose.prod.yml logs -f
```

---

## 🎯 开发最佳实践

### 代码规范

```bash
# Go代码格式化
go fmt ./...

# 静态检查
go vet ./...

# 使用golangci-lint（推荐）
golangci-lint run
```

### 性能优化

**开发环境：**
- 使用 `make dev-db` 而不是完整Docker环境
- 优先使用 bun 进行前端构建
- 启用Go模块代理：`export GOPROXY=https://goproxy.cn`

**生产环境：**
- 使用多阶段Docker构建
- 启用Go编译优化：`go build -ldflags "-s -w"`
- 配置适当的容器资源限制

### 安全建议

- ⚠️ 修改默认数据库密码
- ⚠️ 生产环境禁用debug模式
- ⚠️ 配置防火墙规则
- ⚠️ 定期备份数据库

---

## 🔗 快速链接

### 本地服务
- 后端服务: http://localhost:3000
- 管理界面: http://localhost:3000/console
- API健康检查: http://localhost:3000/api/status

### 数据库连接
- MySQL: `mysql -h localhost -P 3307 -u root -pdev123456 new_api_dev`
- Redis: `redis-cli -h localhost -p 6379`

### 项目资源
- GitHub仓库: https://github.com/Calcium-Ion/new-api
- 上游文档: https://docs.new-api.com

---

## 📞 获取帮助

遇到问题？

1. 查看 [故障排除](#-故障排除) 章节
2. 运行 `make status` 检查服务状态
3. 查看日志：`make logs` 或 `make start`（前台运行查看实时日志）
4. 查阅 [相关文档](#-相关文档)

---

**开发指南版本：v3.4**
**最后更新：2025-11-18**
**基于 Make + Docker Compose 工作流**

**v3.4 更新内容**（2025-11-18）：
- ✅ 新增"Token独立额度迁移"完整章节（140行）
- ✅ 说明Token独立额度的架构变更和迁移必要性
- ✅ 提供开发环境和生产环境的详细迁移步骤
- ✅ 新增故障排除：Token余额不足错误（迁移问题）
- ✅ 补充迁移常见问题FAQ

**v3.3 更新内容**（2025-11-18）：
- ✅ 新增"数据库迁移机制"章节（GORM自动迁移说明）
- ✅ 说明Callback功能的数据库字段自动创建
- ✅ 更新数据库管理命令说明（强调自动迁移）
- ✅ 新增故障排除：数据库字段未创建问题
- ✅ 明确无需手动执行SQL脚本的场景

**v3.2 更新内容**（2025-09-30）：
- ✅ 新增"服务器后台运行"章节
- ✅ 添加 `make start-daemon` 后台启动命令
- ✅ 详细说明 tmux 使用方法和快捷键
- ✅ 提供方案对比表格（make daemon vs tmux）
- ✅ 补充 screen 和 systemd 参考方案

**v3.1 更新内容**（2025-09-30）：
- ✅ 添加环境变量配置说明（`.env.dev`）
- ✅ 更新 `make start` 命令说明（优先使用 `.env.dev`）
- ✅ 添加 `make start-backend` 兼容性说明
- ✅ 增强故障排除指南（端口占用、配置未生效、充值日志问题）
- ✅ 更新 `make stop` 命令说明（正确停止所有进程）