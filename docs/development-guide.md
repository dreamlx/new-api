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
# 方式1：推荐 - 分步启动
make dev-db      # 启动数据库（MySQL + Redis）
make start       # 启动后端服务（前台运行，Ctrl+C 停止）

# 方式2：一键启动
make dev         # 启动数据库 + 后端
```

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
make dev          # 一键启动开发环境
make start        # 启动后端服务
make stop         # 停止所有服务
make status       # 查看服务状态
```

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
make db-init      # 初始化外部用户系统数据库
make db-backup    # 备份开发数据库
make db-reset     # 重置数据库（危险操作，需确认）
```

### 其他命令

```bash
make sync         # 同步上游代码
make clean        # 清理Docker资源
```

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

#### 1. 端口被占用

**症状：** `bind: address already in use`

**解决：**
```bash
# 停止所有服务
make stop

# 如果还有问题，手动查找占用进程
lsof -i :3000   # 后端
lsof -i :3307   # MySQL
lsof -i :6379   # Redis

# 强制停止
pkill -f "go run main.go"
```

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

#### 4. 前端构建失败

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

**开发指南版本：v3.0**
**最后更新：2025-09-30**
**基于 Make + Docker Compose 工作流**