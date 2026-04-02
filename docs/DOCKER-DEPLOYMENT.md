# New API Docker 开发部署指南

## 概述

本项目提供完整的 Docker 化开发和部署方案，支持：
- 本地开发环境快速启动
- 一键构建和推送镜像
- 自动化生产环境部署
- 数据备份和恢复

## 快速开始

### 1. 本地开发环境

```bash
# 启动开发环境（包含 MySQL + Redis + New API）
make dev

# 查看服务状态
make status

# 查看日志
make dev-logs

# 停止开发环境
make dev-stop
```

开发环境将在 `http://localhost:3000` 启动。

### 2. 查看所有可用命令

```bash
make help
```

## 开发流程

### 日常开发

1. **启动开发环境**：
   ```bash
   make dev
   ```

2. **修改代码后重新构建**：
   ```bash
   make dev-rebuild
   ```

3. **同步上游代码**：
   ```bash
   make sync
   ```

### 构建和推送

1. **登录 Docker Hub**：
   ```bash
   docker login
   ```

2. **构建并推送镜像**：
   ```bash
   make build
   ```

## 生产环境设置

### 服务器端准备

1. **在服务器上创建项目目录**：
   ```bash
   sudo mkdir -p /opt/new-api
   sudo chown $USER:$USER /opt/new-api
   cd /opt/new-api
   ```

2. **复制必要文件到服务器**：
   ```bash
   # 从开发机器执行
   scp docker-compose.prod.yml .env.prod.example scripts/deploy.sh your-server:/opt/new-api/
   ```

3. **配置生产环境变量**：
   ```bash
   # 在服务器上执行
   cp .env.prod.example .env.prod
   # 编辑 .env.prod 填入真实配置
   nano .env.prod
   ```

### 部署

```bash
# 从开发机器执行
make deploy-prod
```

## 目录结构

```
new-api/
├── docker-compose.yml          # 原始配置
├── docker-compose.dev.yml      # 开发环境配置
├── docker-compose.prod.yml     # 生产环境配置
├── .env.prod.example          # 生产环境变量模板
├── scripts/
│   ├── build-and-push.sh      # 构建推送脚本
│   └── deploy.sh              # 服务器部署脚本
├── Makefile                   # 便捷命令
└── DOCKER-DEPLOYMENT.md       # 本文档
```

## 环境变量说明

### 开发环境
- 数据库：`new_api_dev`
- 密码：`dev123456`
- 端口：MySQL(3306), Redis(6379), API(3000)

### 生产环境
需要在 `.env.prod` 中配置：
- `MYSQL_ROOT_PASSWORD`: 数据库密码
- `SESSION_SECRET`: 会话密钥（必须修改）

## 数据持久化

### 开发环境
- MySQL 数据：`mysql_dev_data` volume
- 应用数据：`./data` 目录
- 日志：`./logs` 目录

### 生产环境
- MySQL 数据：`mysql_data` volume
- Redis 数据：`redis_data` volume
- 应用数据：`./data` 目录
- 日志：`./logs` 目录

## 备份和恢复

### 数据库备份
```bash
# 开发环境
make db-backup

# 生产环境（在服务器上执行）
docker exec mysql-prod mysqldump -u root -p新密码 new_api > backup_$(date +%Y%m%d_%H%M%S).sql
```

### 应用数据备份
```bash
# 备份用户数据
tar -czf data-backup-$(date +%Y%m%d).tar.gz data/ logs/
```

## 监控和日志

### 查看服务状态
```bash
# 开发环境
make status

# 生产环境
docker-compose -f docker-compose.prod.yml ps
```

### 查看日志
```bash
# 开发环境
make dev-logs

# 生产环境
docker-compose -f docker-compose.prod.yml logs -f new-api
```

### 健康检查
生产环境配置了自动健康检查，每30秒检查一次 `/api/status` 端点。

## 故障排除

### 常见问题

1. **端口占用**：
   ```bash
   # 检查端口占用
   lsof -i :3000
   lsof -i :3306
   lsof -i :6379
   ```

2. **权限问题**：
   ```bash
   # 确保脚本有执行权限
   chmod +x scripts/*.sh
   ```

3. **镜像构建失败**：
   ```bash
   # 清理并重新构建
   make clean
   docker system prune -a
   make dev-rebuild
   ```

4. **数据库连接失败**：
   - 检查 Docker 网络连接
   - 确认数据库服务已启动
   - 验证数据库密码配置

### 日志位置
- 开发环境：`./logs/`
- 生产环境：`/opt/new-api/logs/`

## 安全建议

1. **生产环境必须修改**：
   - `MYSQL_ROOT_PASSWORD`
   - `SESSION_SECRET`

2. **网络安全**：
   - 使用 Nginx 反向代理
   - 配置 SSL/TLS
   - 设置防火墙规则

3. **定期备份**：
   - 数据库备份
   - 用户数据备份
   - 配置文件备份

## 版本管理

项目使用语义化版本控制：
- 镜像标签：`dreamlx/new-api:v1.2.3`
- Git 标签：同步版本号
- 最新版本：`latest` 标签

---

*最后更新：2025-01-30*