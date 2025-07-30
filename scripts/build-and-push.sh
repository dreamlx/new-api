#!/bin/bash

# 自动化构建和推送Docker镜像脚本
set -e

# 配置变量
DOCKER_REGISTRY="dreamlx"  # 你的Docker Hub用户名
IMAGE_NAME="new-api"
VERSION=$(cat VERSION 2>/dev/null || echo "latest")
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT=$(git rev-parse --short HEAD)

# 镜像标签
FULL_IMAGE_NAME="${DOCKER_REGISTRY}/${IMAGE_NAME}"
TAGS=("${VERSION}" "latest" "${GIT_COMMIT}")

echo "🏗️  开始构建 New API Docker 镜像..."
echo "📝 版本: ${VERSION}"
echo "🔗 提交: ${GIT_COMMIT}"
echo "📅 构建时间: ${BUILD_DATE}"

# 构建镜像
echo "🔨 构建镜像..."
docker build \
  --build-arg VERSION="${VERSION}" \
  --build-arg BUILD_DATE="${BUILD_DATE}" \
  --build-arg GIT_COMMIT="${GIT_COMMIT}" \
  -t "${FULL_IMAGE_NAME}:${VERSION}" \
  -t "${FULL_IMAGE_NAME}:latest" \
  -t "${FULL_IMAGE_NAME}:${GIT_COMMIT}" \
  .

echo "✅ 镜像构建完成"

# 推送镜像
echo "📤 推送镜像到 Docker Hub..."
for tag in "${TAGS[@]}"; do
  echo "推送: ${FULL_IMAGE_NAME}:${tag}"
  docker push "${FULL_IMAGE_NAME}:${tag}"
done

echo "🎉 构建和推送完成！"
echo "📦 镜像: ${FULL_IMAGE_NAME}:${VERSION}"
echo ""
echo "🚀 部署命令:"
echo "   ssh your-server 'cd /path/to/new-api && docker-compose -f docker-compose.prod.yml pull && docker-compose -f docker-compose.prod.yml up -d'"