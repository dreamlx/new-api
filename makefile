FRONTEND_DIR = ./web
BACKEND_DIR = .

.PHONY: help all build-frontend start-backend dev build push deploy clean sync

# 默认目标
help: ## 显示帮助信息
	@echo "New API 开发和部署命令:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

# 原有构建命令
all: build-frontend start-backend ## 构建前端并启动后端 (原有命令)

build-frontend: ## 构建前端
	@echo "🏗️  构建前端..."
	@export PATH="$$HOME/.bun/bin:$$PATH"; \
	if command -v bun >/dev/null 2>&1; then \
		echo "✅ 使用 bun 构建"; \
		cd $(FRONTEND_DIR) && bun install && DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$$(cat ../VERSION) bun run build; \
	else \
		echo "⚠️  未找到 bun，使用 npm 构建"; \
		cd $(FRONTEND_DIR) && npm install --legacy-peer-deps && DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$$(cat ../VERSION) npm run build; \
	fi

start-backend: ## 启动后端开发服务器（需要先构建前端）
	@echo "🚀 启动后端开发服务器..."
	@echo "📋 加载环境变量: .env.dev"
	@cd $(BACKEND_DIR) && export $$(cat .env.dev | xargs) && go run main.go

start-backend-only: ## 仅启动后端（跳过前端构建）
	@echo "🚀 启动后端开发服务器（跳过前端构建）..."
	@echo "📋 加载环境变量: .env.dev"
	@cd $(BACKEND_DIR) && \
	export SQL_DSN=root:dev123456@tcp\(localhost:3307\)/new_api_dev && \
	export REDIS_CONN_STRING=redis://localhost:6379 && \
	export GIN_MODE=debug && \
	export TZ=Asia/Shanghai && \
	export ERROR_LOG_ENABLED=true && \
	go run main.go

dev-quick: dev-db build-frontend start-backend ## 快速启动完整开发环境

# Docker 开发环境
dev: ## 启动本地开发环境（完整Docker）
	@echo "🏗️  启动完整开发环境..."
	docker compose -f docker-compose.dev.yml up -d
	@echo "✅ 开发环境已启动: http://localhost:3000"

dev-db: ## 仅启动数据库服务（推荐）
	@echo "🗄️  启动数据库服务..."
	docker compose -f docker-compose.db-only.yml up -d
	@echo "✅ 数据库服务已启动"
	@echo "📝 连接信息:"
	@echo "   MySQL: localhost:3307, 用户: root, 密码: dev123456, 数据库: new_api_dev"
	@echo "   Redis: localhost:6379"
	@echo "🚀 现在可以运行: make start-backend"

dev-logs: ## 查看开发环境日志
	docker compose -f docker-compose.dev.yml logs -f

dev-stop: ## 停止开发环境
	docker compose -f docker-compose.dev.yml down

dev-db-stop: ## 停止数据库服务
	docker compose -f docker-compose.db-only.yml down

dev-rebuild: ## 重新构建并启动开发环境
	docker compose -f docker-compose.dev.yml down
	docker compose -f docker-compose.dev.yml build --no-cache
	docker compose -f docker-compose.dev.yml up -d

# 构建和推送
build: ## 构建 Docker 镜像
	@echo "🔨 构建镜像..."
	./scripts/build-and-push.sh

# 同步上游代码
sync: ## 同步上游官方代码
	@echo "🔄 同步上游代码..."
	git fetch upstream
	git checkout main
	git merge upstream/main
	git push origin main
	@echo "✅ 代码同步完成"

# 部署相关
deploy-staging: ## 部署到测试环境
	@echo "🚀 部署到测试环境..."
	# 这里添加测试环境部署命令

deploy-prod: ## 部署到生产环境 (需要服务器访问权限)
	@echo "🚀 部署到生产环境..."
	@read -p "确认部署到生产环境? [y/N]: " confirm && [ "$$confirm" = "y" ]
	ssh your-server 'cd /opt/new-api && ./scripts/deploy.sh'

# 清理
clean: ## 清理本地Docker资源
	@echo "🧹 清理Docker资源..."
	docker compose -f docker-compose.dev.yml down --volumes --remove-orphans
	docker system prune -f
	@echo "✅ 清理完成"

# 测试
test: ## 运行单元测试
	@echo "🧪 运行单元测试..."
	go test ./...

test-api: ## 运行外部用户API自动化测试
	@echo "🧪 运行外部用户API自动化测试..."
	@chmod +x scripts/test-external-user-api.sh
	@chmod +x test-api-rollback.sh
	@./scripts/test-external-user-api.sh

test-api-rollback: ## 运行API回滚验证测试
	@echo "🧪 运行API回滚验证测试..."
	@chmod +x test-api-rollback.sh
	@./test-api-rollback.sh

test-api-quick: ## 快速检查API服务状态
	@echo "⚡ 快速API健康检查..."
	@curl -sf http://localhost:3000/api/status > /dev/null && echo "✅ API服务正常" || echo "❌ API服务异常"

# 数据库相关
db-init: ## 初始化外部用户系统数据库
	@echo "🗄️  初始化外部用户系统数据库..."
	./scripts/init-db.sh

db-backup: ## 备份开发数据库
	@echo "💾 备份数据库..."
	docker exec mysql-dev mysqldump -u root -pdev123456 new_api_dev > backup_$$(date +%Y%m%d_%H%M%S).sql

# 状态检查
status: ## 查看服务状态
	@echo "📊 开发环境状态:"
	docker compose -f docker-compose.dev.yml ps
