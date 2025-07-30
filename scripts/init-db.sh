#!/bin/bash

# 外部用户系统数据库初始化脚本
# 作者: Claude Code Assistant  
# 日期: 2025-01-30

set -e

# 配置变量
DB_HOST="127.0.0.1" 
DB_PORT="3307"
DB_USER="root"
DB_PASS="dev123456"
DB_NAME="new_api_dev"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🗄️  开始初始化外部用户系统数据库...${NC}"

# 检查MySQL连接
echo -e "${YELLOW}📡 检查数据库连接...${NC}"
if ! mysql -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -p"$DB_PASS" -e "SELECT 1;" > /dev/null 2>&1; then
    echo -e "${RED}❌ 无法连接到数据库，请检查数据库服务和连接参数${NC}"
    echo "连接参数: $DB_USER@$DB_HOST:$DB_PORT"
    exit 1
fi

echo -e "${GREEN}✅ 数据库连接成功${NC}"

# 检查数据库是否存在
echo -e "${YELLOW}📋 检查数据库 $DB_NAME...${NC}"
if ! mysql -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -p"$DB_PASS" -e "USE $DB_NAME;" > /dev/null 2>&1; then
    echo -e "${RED}❌ 数据库 $DB_NAME 不存在${NC}"
    exit 1
fi

echo -e "${GREEN}✅ 数据库 $DB_NAME 存在${NC}"

# 执行SQL脚本
echo -e "${YELLOW}⚡ 执行数据库扩展脚本...${NC}"
mysql -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -p"$DB_PASS" "$DB_NAME" < "$(dirname "$0")/init-external-user-db.sql"

if [ $? -eq 0 ]; then
    echo -e "${GREEN}🎉 外部用户系统数据库初始化完成！${NC}"
    echo ""
    echo -e "${BLUE}📊 数据库信息:${NC}"
    echo "  主机: $DB_HOST:$DB_PORT"
    echo "  数据库: $DB_NAME"
    echo "  用户: $DB_USER"
    echo ""
    echo -e "${BLUE}🔧 新增字段:${NC}"
    echo "  • external_user_id - 外部用户ID（唯一索引）"
    echo "  • phone - 手机号"
    echo "  • wechat_openid - 微信OpenID"  
    echo "  • wechat_unionid - 微信UnionID"
    echo "  • alipay_userid - 支付宝用户ID"
    echo "  • login_type - 登录类型"
    echo "  • is_external - 是否外部用户"
    echo "  • external_data - 外部用户扩展数据"
    echo ""
    echo -e "${GREEN}✅ 现在可以开始开发外部用户集成API了！${NC}"
else
    echo -e "${RED}❌ 数据库初始化失败${NC}"
    exit 1
fi