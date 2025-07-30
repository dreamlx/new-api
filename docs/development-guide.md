# 外部用户系统集成 - 开发指南

## 项目开发前准备

### 开发环境要求

#### 必需环境
- **Go 1.19+**：后端开发语言
- **Node.js 16+**：前端构建工具
- **MySQL 5.7+ / PostgreSQL 9.6+ / SQLite**：数据库
- **Redis**（可选）：缓存和会话存储
- **Git**：版本控制

#### 开发工具推荐
- **IDE**：GoLand / VS Code
- **数据库工具**：DBeaver / MySQL Workbench
- **API 测试**：Postman / Insomnia
- **版本管理**：GitHub / GitLab

### 开发环境搭建

#### 1. 克隆和构建项目
```bash
# 克隆项目
git clone https://github.com/Calcium-Ion/new-api.git
cd new-api

# 安装 Go 依赖
go mod download

# 构建前端
cd web
npm install
npm run build
cd ..

# 构建后端
go build -ldflags "-s -w" -o new-api
```

#### 2. 环境配置
```bash
# 复制配置文件
cp .env.example .env

# 编辑配置
vim .env
```

#### 3. 数据库初始化
```sql
-- 创建数据库
CREATE DATABASE new_api CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- 创建用户（可选）
CREATE USER 'newapi'@'localhost' IDENTIFIED BY 'password';
GRANT ALL PRIVILEGES ON new_api.* TO 'newapi'@'localhost';
FLUSH PRIVILEGES;
```

#### 4. 启动开发服务器
```bash
# 启动后端（开发模式）
export GIN_MODE=debug
export SQL_DSN="root:password@tcp(localhost:3306)/new_api"
./new-api

# 前端开发模式（另一个终端）
cd web
npm run dev
```

## 测试策略

### 1. 单元测试

#### 测试框架选择
- **后端**：Go 内置 testing 包 + testify
- **前端**：Jest + React Testing Library

#### Go 单元测试示例
```go
// controller/user_test.go
package controller

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

func TestSyncExternalUser(t *testing.T) {
    // 测试用户同步功能
    t.Run("创建新用户", func(t *testing.T) {
        // Arrange
        req := SyncExternalUserRequest{
            ExternalUserId: "test_user_123",
            Username:       "testuser",
            DisplayName:    "Test User",
            Email:          "test@example.com",
        }
        
        // Act
        result := syncExternalUser(req)
        
        // Assert
        assert.True(t, result.Success)
        assert.NotEmpty(t, result.Data.UserId)
    })
    
    t.Run("更新现有用户", func(t *testing.T) {
        // 测试用户更新逻辑
    })
}

func TestExternalUserTopUp(t *testing.T) {
    // 测试充值功能
    t.Run("正常充值", func(t *testing.T) {
        req := TopUpRequest{
            ExternalUserId: "test_user_123",
            AmountUSD:      10.00,
            PaymentId:      "test_payment_123",
        }
        
        result := externalUserTopUp(req)
        
        assert.True(t, result.Success)
        assert.Equal(t, 5000000, result.Data.QuotaAdded) // $10 * 500,000
    })
    
    t.Run("无效金额", func(t *testing.T) {
        // 测试金额验证
    })
}
```

#### 测试运行命令
```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./controller

# 生成测试覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### 2. 集成测试

#### 数据库集成测试
```go
// test/integration_test.go
package test

import (
    "testing"
    "database/sql"
    "github.com/stretchr/testify/suite"
)

type IntegrationTestSuite struct {
    suite.Suite
    db *sql.DB
}

func (suite *IntegrationTestSuite) SetupSuite() {
    // 设置测试数据库
    suite.db = setupTestDB()
    runMigrations(suite.db)
}

func (suite *IntegrationTestSuite) TearDownSuite() {
    // 清理测试数据库
    suite.db.Close()
}

func (suite *IntegrationTestSuite) TestExternalUserFlow() {
    // 测试完整的用户流程
    // 1. 同步用户
    // 2. 充值
    // 3. 创建Token
    // 4. 使用API
    // 5. 查看统计
}

func TestIntegrationSuite(t *testing.T) {
    suite.Run(t, new(IntegrationTestSuite))
}
```

#### API 端到端测试
```bash
# 使用 httpie 或 curl 进行 API 测试
#!/bin/bash

BASE_URL="http://localhost:3000"
ADMIN_TOKEN="your_admin_token"

# 测试用户同步
http POST $BASE_URL/api/user/external/sync \
  Authorization:"Bearer $ADMIN_TOKEN" \
  external_user_id="test_user_001" \
  username="testuser" \
  display_name="Test User"

# 测试充值
http POST $BASE_URL/api/user/external/topup \
  Authorization:"Bearer $ADMIN_TOKEN" \
  external_user_id="test_user_001" \
  amount_usd:=10.00 \
  payment_id="test_payment_001"

# 测试创建Token
http POST $BASE_URL/api/user/external/token \
  Authorization:"Bearer $ADMIN_TOKEN" \
  external_user_id="test_user_001" \
  token_name="Test Token" \
  expires_in_days:=30
```

### 3. 性能测试

#### 压力测试脚本
```bash
# 使用 hey 工具进行压力测试
#!/bin/bash

# 安装 hey
go install github.com/rakyll/hey@latest

# 测试用户同步API
hey -n 1000 -c 10 -m POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"external_user_id":"load_test_user","username":"loadtest","display_name":"Load Test"}' \
  http://localhost:3000/api/user/external/sync

# 测试充值API
hey -n 500 -c 5 -m POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"external_user_id":"load_test_user","amount_usd":1.00,"payment_id":"load_test_payment"}' \
  http://localhost:3000/api/user/external/topup
```

## 自动化部署

### 1. CI/CD 流水线

#### GitHub Actions 配置
```yaml
# .github/workflows/ci.yml
name: CI/CD Pipeline

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main ]

jobs:
  test:
    runs-on: ubuntu-latest
    
    services:
      mysql:
        image: mysql:8.0
        env:
          MYSQL_ROOT_PASSWORD: testpass
          MYSQL_DATABASE: new_api_test
        options: >-
          --health-cmd="mysqladmin ping"
          --health-interval=10s
          --health-timeout=5s
          --health-retries=3
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v3
      with:
        go-version: 1.19
    
    - name: Install dependencies
      run: go mod download
    
    - name: Run tests
      run: |
        export SQL_DSN="root:testpass@tcp(localhost:3306)/new_api_test"
        go test -v ./...
    
    - name: Build
      run: go build -v .

  deploy:
    needs: test
    runs-on: ubuntu-latest
    if: github.ref == 'refs/heads/main'
    
    steps:
    - name: Deploy to production
      run: |
        echo "部署到生产环境"
        # 实际部署脚本
```

### 2. Docker 部署

#### Dockerfile 优化
```dockerfile
# Dockerfile
FROM golang:1.19-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o new-api .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/new-api .
COPY --from=builder /app/web/build ./web/build

EXPOSE 3000

CMD ["./new-api"]
```

#### Docker Compose 开发环境
```yaml
# docker-compose.dev.yml
version: '3.8'

services:
  new-api:
    build: .
    ports:
      - "3000:3000"
    environment:
      - GIN_MODE=debug
      - SQL_DSN=root:password@tcp(mysql:3306)/new_api
    depends_on:
      - mysql
      - redis
    volumes:
      - ./:/app
      - /app/web/node_modules
  
  mysql:
    image: mysql:8.0
    environment:
      - MYSQL_ROOT_PASSWORD=password
      - MYSQL_DATABASE=new_api
    ports:
      - "3306:3306"
    volumes:
      - mysql_data:/var/lib/mysql
  
  redis:
    image: redis:alpine
    ports:
      - "6379:6379"

volumes:
  mysql_data:
```

### 3. 生产环境部署

#### 部署脚本
```bash
#!/bin/bash
# deploy.sh

set -e

echo "开始部署外部用户系统集成..."

# 拉取最新代码
git pull origin main

# 构建Docker镜像
docker build -t new-api:latest .

# 停止旧容器
docker-compose -f docker-compose.prod.yml down

# 数据库迁移
docker run --rm \
  --network new-api_default \
  -e SQL_DSN="root:password@tcp(mysql:3306)/new_api" \
  new-api:latest \
  ./new-api --migrate

# 启动新容器
docker-compose -f docker-compose.prod.yml up -d

# 健康检查
sleep 10
curl -f http://localhost:3000/api/status || exit 1

echo "部署完成！"
```

## 监控和日志

### 1. 应用监控
```go
// 添加监控中间件
func MonitoringMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        
        c.Next()
        
        // 记录请求指标
        duration := time.Since(start)
        statusCode := c.Writer.Status()
        
        log.Printf("API调用: %s %s - %d - %v", 
            c.Request.Method, c.Request.URL.Path, statusCode, duration)
    }
}
```

### 2. 日志配置
```go
// 结构化日志
import "github.com/sirupsen/logrus"

func setupLogging() {
    logrus.SetFormatter(&logrus.JSONFormatter{})
    logrus.SetLevel(logrus.InfoLevel)
    
    if gin.Mode() == gin.DebugMode {
        logrus.SetLevel(logrus.DebugLevel)
    }
}
```

## 开发最佳实践

### 1. 代码规范
- 使用 `gofmt` 格式化代码
- 遵循 Go 官方编码规范
- 添加必要的注释和文档
- 使用有意义的变量和函数名

### 2. 错误处理
```go
// 统一错误处理
func handleError(c *gin.Context, err error, message string) {
    logrus.WithError(err).Error(message)
    c.JSON(500, gin.H{
        "success": false,
        "message": message,
    })
}
```

### 3. 数据验证
```go
// 使用 validator 进行数据验证
type SyncUserRequest struct {
    ExternalUserId string `json:"external_user_id" binding:"required,min=1,max=100"`
    Username       string `json:"username" binding:"required,min=1,max=50"`
    DisplayName    string `json:"display_name" binding:"max=100"`
    Email          string `json:"email" binding:"omitempty,email,max=100"`
}
```

---
*开发指南版本：v1.0*  
*最后更新：2025-01-29*