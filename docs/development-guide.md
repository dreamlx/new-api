# 外部用户系统集成 - 开发指南

## 项目概述

基于 New API 的外部用户系统集成方案，使用 **Make + Docker Compose** 进行开发和部署管理。

### 技术栈
- **后端**：Go 1.19+ + Gin + GORM
- **前端**：React + Vite (使用 Bun 或 NPM)
- **数据库**：MySQL 8.2 + Redis
- **开发工具**：Make + Docker Compose
- **部署**：Docker 容器化部署

## 环境要求

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

## 快速开始

### 1. 项目克隆和准备
```bash
# 克隆项目
git clone <your-repo-url>
cd new-api

# 查看所有可用命令
make help
```

### 2. 开发环境启动

#### 推荐方式：数据库容器 + 本地后端
```bash
# 1. 启动数据库服务（MySQL + Redis）
make dev-db

# 2. 初始化外部用户系统数据库
make db-init

# 3. 构建前端
make build-frontend

# 4. 启动后端开发服务器
make start-backend
```

#### 一键启动方式
```bash
# 快速启动完整开发环境
make dev-quick
```

#### 完整Docker方式
```bash
# 启动完整Docker开发环境
make dev

# 查看日志
make dev-logs

# 停止环境
make dev-stop
```

### 3. 访问应用
- **应用地址**：http://localhost:3000
- **管理界面**：http://localhost:3000/console
- **API文档**：参考 `docs/external-user-api.md`

## Make 命令详解

### 开发环境管理
```bash
make help              # 显示所有可用命令
make dev-db            # 启动数据库服务（推荐）
make dev               # 完整Docker开发环境
make dev-quick         # 一键启动：数据库+前端构建+后端
make dev-stop          # 停止开发环境
make dev-db-stop       # 停止数据库服务
make dev-logs          # 查看开发环境日志
make status            # 查看服务状态
```

### 构建和启动
```bash
make build-frontend    # 构建前端（自动选择bun或npm）
make start-backend     # 启动后端开发服务器
make start-backend-only # 仅启动后端（跳过前端构建）
make build             # 构建Docker镜像
```

### 数据库管理
```bash
make db-init          # 初始化外部用户系统数据库
make db-backup        # 备份开发数据库
```

### 测试和清理
```bash
make test             # 运行Go单元测试
make clean            # 清理Docker资源
```

## 环境配置

### 数据库连接信息
使用 `make dev-db` 启动的数据库服务：

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

### 环境变量文件
项目使用 `.env.dev` 文件管理开发环境变量：

```bash
# .env.dev
SQL_DSN=root:dev123456@tcp(localhost:3307)/new_api_dev
REDIS_CONN_STRING=redis://localhost:6379
GIN_MODE=debug
TZ=Asia/Shanghai
ERROR_LOG_ENABLED=true
```

## 数据库初始化

### 自动初始化
```bash
# 运行初始化脚本（推荐）
make db-init
```

### 手动初始化
如果需要手动操作：

```bash
# 连接数据库
mysql -h localhost -P 3307 -u root -pdev123456

# 执行SQL脚本
mysql -h localhost -P 3307 -u root -pdev123456 new_api_dev < scripts/init-external-user-db.sql
```

### 初始化内容
数据库初始化脚本会添加以下字段到 `users` 表：

- `external_user_id`：外部用户唯一标识（带唯一索引）
- `phone`：手机号码
- `wechat_openid`：微信OpenID
- `wechat_unionid`：微信UnionID
- `alipay_userid`：支付宝用户ID
- `login_type`：登录类型
- `is_external`：是否外部用户标识
- `external_data`：扩展数据字段

## 开发工作流

### 典型开发流程
```bash
# 1. 启动开发环境
make dev-db

# 2. 初始化数据库（首次）
make db-init

# 3. 开发循环
make build-frontend    # 修改前端后重新构建
make start-backend     # 启动后端（自动重启）

# 4. 运行测试
make test

# 5. 清理环境（可选）
make clean
```

### 前端开发
```bash
# 自动选择构建工具（优先bun）
make build-frontend

# 手动前端开发（另开终端）
cd web
npm run dev  # 或 bun run dev
```

### 后端开发
```bash
# 启动后端开发服务器（带热重载）
make start-backend

# 仅启动后端（跳过前端构建）
make start-backend-only
```

## 测试策略

### 单元测试
```bash
# 运行所有测试
make test

# 运行特定测试
go test ./controller -v
go test ./controller -run "TestExternalUser" -v

# 生成测试覆盖率
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### API测试
```bash
# 启动开发环境
make dev-db
make start-backend

# 使用curl测试API（参考文档）
curl -X GET "http://localhost:3000/api/user/external/test_user_001/stats"
```

### 完整测试流程
详细的API测试指南请参考：
- `docs/curl-testing-guide.md` - 完整的curl测试用例
- `docs/external-user-api.md` - API接口文档

## Docker 配置

### 开发环境文件
- `docker-compose.db-only.yml`：仅数据库服务（推荐）
- `docker-compose.dev.yml`：完整开发环境
- `docker-compose.prod.yml`：生产环境配置

### 容器管理
```bash
# 查看容器状态
docker compose -f docker-compose.db-only.yml ps

# 查看容器日志
docker logs mysql-dev
docker logs redis-dev

# 进入容器调试
docker exec -it mysql-dev mysql -u root -pdev123456
docker exec -it redis-dev redis-cli
```

## 故障排除

### 常见问题

#### 1. 数据库连接失败
```bash
# 检查容器状态
make status

# 重启数据库服务
make dev-db-stop
make dev-db
```

#### 2. 端口冲突
```bash
# 检查端口占用
lsof -i :3307  # MySQL
lsof -i :6379  # Redis
lsof -i :3000  # Web服务

# 修改端口配置
# 编辑 docker-compose.db-only.yml
```

#### 3. 前端构建失败
```bash
# 清理node_modules
cd web && rm -rf node_modules && npm install

# 或使用bun
cd web && rm -rf node_modules && bun install
```

#### 4. Go模块问题
```bash
# 清理Go模块缓存
go clean -modcache
go mod download
```

### 日志查看
```bash
# 开发环境日志
make dev-logs

# 特定容器日志
docker logs mysql-dev -f
docker logs redis-dev -f

# 后端应用日志（本地运行时）
tail -f logs/oneapi-*.log
```

## 生产部署

### 构建生产镜像
```bash
# 构建Docker镜像
make build

# 或手动构建
docker build -t new-api:latest .
```

### 生产环境部署
```bash
# 部署到生产环境
make deploy-prod

# 手动部署
docker-compose -f docker-compose.prod.yml up -d
```

## 代码规范

### Go代码规范
```bash
# 格式化代码
go fmt ./...

# 静态检查
go vet ./...

# 使用golangci-lint（推荐）
golangci-lint run
```

### Git工作流
```bash
# 同步上游代码
make sync

# 提交代码
git add .
git commit -m "feat: 添加消费记录查询API"
git push origin feature-branch
```

## 性能优化

### 开发环境优化
- 使用 `make dev-db` 而不是完整Docker环境
- 优先使用 bun 进行前端构建
- 启用Go模块代理：`export GOPROXY=https://goproxy.cn`

### 生产环境优化
- 使用多阶段Docker构建
- 启用Go编译优化：`go build -ldflags "-s -w"`
- 配置适当的容器资源限制

## 监控和日志

### 应用监控
```go
// 添加监控中间件示例
func MonitoringMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        c.Next()
        duration := time.Since(start)
        
        // 记录API调用指标
        log.Printf("API: %s %s - %d - %v", 
            c.Request.Method, c.Request.URL.Path, 
            c.Writer.Status(), duration)
    }
}
```

### 结构化日志
项目使用结构化日志记录，支持：
- 请求链路追踪
- 错误详情记录
- 性能指标监控
- 用户行为分析

---

## 快速命令参考

```bash
# 开发环境
make dev-db              # 启动数据库
make db-init             # 初始化数据库
make build-frontend      # 构建前端
make start-backend       # 启动后端
make test               # 运行测试

# 管理命令
make status             # 查看状态
make dev-logs           # 查看日志
make clean              # 清理环境
make help               # 显示帮助
```

---
*开发指南版本：v2.0*  
*最后更新：2025-01-31*
*基于 Make + Docker Compose 工作流*