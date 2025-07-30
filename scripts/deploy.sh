#!/bin/bash

# 服务器端自动部署脚本
set -e

# 配置变量
PROJECT_DIR="/opt/new-api"  # 服务器上的项目目录
BACKUP_DIR="/opt/new-api-backups"
SERVICE_NAME="new-api-prod"

echo "🚀 开始部署 New API..."

# 创建备份
echo "📦 创建备份..."
BACKUP_FILE="${BACKUP_DIR}/backup-$(date +%Y%m%d-%H%M%S).tar.gz"
mkdir -p "${BACKUP_DIR}"
tar -czf "${BACKUP_FILE}" -C "${PROJECT_DIR}" data logs .env.prod 2>/dev/null || echo "⚠️  部分文件备份失败，继续部署..."

# 拉取最新镜像
echo "📥 拉取最新镜像..."
cd "${PROJECT_DIR}"
docker compose -f docker-compose.prod.yml pull

# 重启服务
echo "🔄 重启服务..."
docker compose -f docker-compose.prod.yml down
docker compose -f docker-compose.prod.yml up -d

# 等待服务启动
echo "⏳ 等待服务启动..."
sleep 10

# 健康检查
echo "🏥 执行健康检查..."
MAX_ATTEMPTS=30
ATTEMPT=1

while [ $ATTEMPT -le $MAX_ATTEMPTS ]; do
  if curl -f -s http://localhost:3000/api/status > /dev/null; then
    echo "✅ 服务启动成功！"
    break
  fi
  
  if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
    echo "❌ 服务启动失败，正在恢复备份..."
    docker compose -f docker-compose.prod.yml down
    # 这里可以添加恢复备份的逻辑
    exit 1
  fi
  
  echo "⏳ 尝试 ${ATTEMPT}/${MAX_ATTEMPTS}，等待服务响应..."
  sleep 2
  ATTEMPT=$((ATTEMPT + 1))
done

# 清理旧备份（保留最近7个）
echo "🧹 清理旧备份..."
ls -t "${BACKUP_DIR}"/backup-*.tar.gz 2>/dev/null | tail -n +8 | xargs rm -f 2>/dev/null || true

# 清理未使用的Docker镜像
echo "🗑️  清理未使用的Docker镜像..."
docker image prune -f

echo "🎉 部署完成！"
echo "📊 服务状态:"
docker compose -f docker-compose.prod.yml ps