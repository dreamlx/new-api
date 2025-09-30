FRONTEND_DIR = ./web
BACKEND_DIR = .

.PHONY: help dev dev-db stop start start-backend start-daemon status logs \
	build build-backend build-frontend build-docker prod \
	test test-api test-user-story \
	db-init db-backup db-reset \
	sync clean

# 默认目标：显示帮助
.DEFAULT_GOAL := help

help: ## 显示所有可用命令
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  New API 开发和部署命令"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 🚀 开发环境
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

dev: dev-db start ## 快速启动开发环境（推荐）

start: ## 启动后端服务（前台运行，需先执行 make dev-db）
	@echo "🚀 启动后端服务..."
	@if [ -f .env.dev ]; then \
		echo "📋 使用 .env.dev 配置文件"; \
		cd $(BACKEND_DIR) && export $$(cat .env.dev | xargs) && go run main.go; \
	else \
		echo "⚠️  .env.dev 不存在，使用默认配置"; \
		cd $(BACKEND_DIR) && \
		export SQL_DSN=root:dev123456@tcp\(localhost:3307\)/new_api_dev && \
		export REDIS_CONN_STRING=redis://localhost:6379 && \
		export GIN_MODE=debug && \
		export TZ=Asia/Shanghai && \
		export ERROR_LOG_ENABLED=true && \
		go run main.go; \
	fi

start-backend: start ## 启动后端服务（兼容旧版本命令）

start-daemon: ## 后台启动后端服务（服务器部署用）
	@echo "🚀 后台启动服务..."
	@mkdir -p logs
	@if [ -f .env.dev ]; then \
		echo "📋 使用 .env.dev 配置文件"; \
		cd $(BACKEND_DIR) && export $$(cat .env.dev | xargs) && \
		nohup go run main.go > logs/app.log 2>&1 & echo $$! > .pid && \
		echo "✅ 服务已在后台启动，PID: $$(cat .pid)"; \
	else \
		echo "⚠️  .env.dev 不存在，使用默认配置"; \
		cd $(BACKEND_DIR) && \
		nohup sh -c 'export SQL_DSN=root:dev123456@tcp\(localhost:3307\)/new_api_dev && \
		export REDIS_CONN_STRING=redis://localhost:6379 && \
		export GIN_MODE=debug && \
		export TZ=Asia/Shanghai && \
		export ERROR_LOG_ENABLED=true && \
		go run main.go' > logs/app.log 2>&1 & echo $$! > .pid && \
		echo "✅ 服务已在后台启动，PID: $$(cat .pid)"; \
	fi
	@echo "📋 查看日志: tail -f logs/app.log"
	@echo "🛑 停止服务: make stop"

dev-db: ## 启动数据库服务（MySQL + Redis）
	@echo "🗄️  启动数据库服务..."
	@docker compose -f docker-compose.db-only.yml up -d
	@echo "✅ 数据库服务已启动"
	@echo ""
	@echo "📝 连接信息:"
	@echo "   MySQL: localhost:3307"
	@echo "   Redis: localhost:6379"
	@echo ""

stop: ## 停止所有服务
	@echo "🛑 停止所有服务..."
	@pkill -f "go run main.go" 2>/dev/null || true
	@lsof -ti:3000 2>/dev/null | xargs kill -9 2>/dev/null || true
	@docker compose -f docker-compose.db-only.yml down
	@echo "✅ 所有服务已停止"

status: ## 查看服务运行状态
	@echo "📊 服务状态:"
	@docker compose -f docker-compose.db-only.yml ps
	@echo ""
	@ps aux | grep "go run main.go" | grep -v grep || echo "后端服务: 未运行"

logs: ## 查看数据库日志
	@docker compose -f docker-compose.db-only.yml logs -f

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 🏗️  构建相关
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

build: build-backend build-frontend ## 完整构建（后端+前端）

build-backend: ## 编译后端二进制文件（生产部署用）
	@echo "🔨 编译后端..."
	@go build -ldflags="-s -w" -o one-api main.go
	@echo "✅ 编译完成: ./one-api"
	@ls -lh one-api

prod: build-backend ## 生产环境启动（使用编译后的二进制）
	@echo "🚀 启动生产服务..."
	@if [ ! -f one-api ]; then \
		echo "❌ 二进制文件不存在，请先运行: make build-backend"; \
		exit 1; \
	fi
	@if [ -f .env.prod ]; then \
		echo "📋 使用 .env.prod 配置文件"; \
		export $$(cat .env.prod | xargs) && ./one-api; \
	else \
		echo "⚠️  .env.prod 不存在，使用环境变量"; \
		./one-api; \
	fi

build-frontend: ## 构建前端（生产部署用）
	@echo "🏗️  构建前端..."
	@export PATH="$$HOME/.bun/bin:$$PATH"; \
	if command -v bun >/dev/null 2>&1; then \
		echo "✅ 使用 bun 构建"; \
		cd $(FRONTEND_DIR) && bun install && DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$$(cat ../VERSION) bun run build; \
	else \
		echo "⚠️  使用 npm 构建"; \
		cd $(FRONTEND_DIR) && npm install --legacy-peer-deps && DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$$(cat ../VERSION) npm run build; \
	fi

build-docker: ## 构建 Docker 镜像
	@echo "🔨 构建 Docker 镜像..."
	@./scripts/build-and-push.sh

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 🧪 测试相关
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

test: ## 运行Go单元测试
	@echo "🧪 运行单元测试..."
	@go test ./... -v

test-api: ## 测试外部用户API
	@echo "🧪 测试外部用户API..."
	@chmod +x scripts/test-external-user-api.sh
	@./scripts/test-external-user-api.sh

test-user-story: ## 测试完整业务流程
	@echo "📖 测试用户故事..."
	@chmod +x scripts/test-user-story.sh
	@./scripts/test-user-story.sh

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 🗄️  数据库相关
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

db-init: ## 初始化数据库
	@echo "🗄️  初始化数据库..."
	@./scripts/init-db.sh

db-backup: ## 备份数据库
	@echo "💾 备份数据库..."
	@docker exec mysql-dev mysqldump -u root -pdev123456 new_api_dev > backup_$$(date +%Y%m%d_%H%M%S).sql
	@echo "✅ 备份完成: backup_$$(date +%Y%m%d_%H%M%S).sql"

db-reset: stop ## 重置数据库（危险操作）
	@echo "⚠️  重置数据库..."
	@read -p "确认删除所有数据? [y/N]: " confirm && [ "$$confirm" = "y" ]
	@docker compose -f docker-compose.db-only.yml down -v
	@docker compose -f docker-compose.db-only.yml up -d
	@echo "✅ 数据库已重置"

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 🔄 版本管理
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

sync: ## 同步上游代码
	@echo "🔄 同步上游代码..."
	@git fetch upstream
	@git merge upstream/main
	@echo "✅ 同步完成"

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 🧹 清理相关
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

clean: ## 清理Docker资源
	@echo "🧹 清理资源..."
	@docker compose -f docker-compose.db-only.yml down --volumes --remove-orphans
	@docker system prune -f
	@echo "✅ 清理完成"
