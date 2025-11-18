#!/usr/bin/env bash
#######################################
# Token独立额度数据迁移脚本
# 用途：将现有Token从共享用户余额模式迁移到独立额度模式
# 作者：New API Team
# 日期：2025-11-18
#######################################

set -euo pipefail  # 错误时退出，未定义变量报错，管道任一失败即退出

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*"
}

# 显示帮助信息
show_help() {
    cat <<EOF
Token独立额度数据迁移脚本

用法:
    $0 [选项]

选项:
    -h, --help              显示此帮助信息
    -d, --database DB_TYPE  指定数据库类型 (mysql/postgres/sqlite)
    -H, --host HOST         数据库主机 (默认: localhost)
    -P, --port PORT         数据库端口 (MySQL默认:3306, PostgreSQL默认:5432)
    -u, --user USER         数据库用户 (默认: root)
    -p, --password PASS     数据库密码
    -D, --dbname NAME       数据库名称 (默认: new_api_dev)
    -s, --sql-file FILE     仅生成SQL文件，不执行迁移
    --dry-run               模拟执行，不实际修改数据
    --min-quota AMOUNT      最小保证额度 (默认: 10000000, 即$20)
    --skip-backup           跳过数据库备份（不推荐）
    --backup-dir DIR        备份目录 (默认: ./backups)

示例:
    # MySQL迁移（交互式输入密码）
    $0 -d mysql -u root -D new_api_dev

    # PostgreSQL迁移（指定密码）
    $0 -d postgres -u postgres -p mypassword -D new_api

    # 仅生成SQL文件
    $0 -d mysql -s /tmp/migration.sql

    # 模拟执行（查看影响）
    $0 -d mysql --dry-run

EOF
}

# 默认配置
DB_TYPE="mysql"
DB_HOST="localhost"
DB_PORT=""
DB_USER="root"
DB_PASSWORD=""
DB_NAME="new_api_dev"
SQL_FILE=""
DRY_RUN=0
MIN_QUOTA=10000000  # $20
SKIP_BACKUP=0
BACKUP_DIR="./backups"

# 解析命令行参数
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_help
            exit 0
            ;;
        -d|--database)
            DB_TYPE="$2"
            shift 2
            ;;
        -H|--host)
            DB_HOST="$2"
            shift 2
            ;;
        -P|--port)
            DB_PORT="$2"
            shift 2
            ;;
        -u|--user)
            DB_USER="$2"
            shift 2
            ;;
        -p|--password)
            DB_PASSWORD="$2"
            shift 2
            ;;
        -D|--dbname)
            DB_NAME="$2"
            shift 2
            ;;
        -s|--sql-file)
            SQL_FILE="$2"
            shift 2
            ;;
        --dry-run)
            DRY_RUN=1
            shift
            ;;
        --min-quota)
            MIN_QUOTA="$2"
            shift 2
            ;;
        --skip-backup)
            SKIP_BACKUP=1
            shift
            ;;
        --backup-dir)
            BACKUP_DIR="$2"
            shift 2
            ;;
        *)
            log_error "未知参数: $1"
            show_help
            exit 1
            ;;
    esac
done

# 设置默认端口
if [ -z "$DB_PORT" ]; then
    case $DB_TYPE in
        mysql)
            DB_PORT=3306
            ;;
        postgres)
            DB_PORT=5432
            ;;
        sqlite)
            DB_PORT=""
            ;;
        *)
            log_error "不支持的数据库类型: $DB_TYPE"
            exit 1
            ;;
    esac
fi

# 生成SQL脚本
generate_sql() {
    local sql_content

    case $DB_TYPE in
        mysql|postgres)
            sql_content=$(cat <<'EOSQL'
-- Token独立额度数据迁移SQL脚本
-- 生成时间: $(date '+%Y-%m-%d %H:%M:%S')
-- 最小保证额度: $MIN_QUOTA (约 $$(echo "scale=2; $MIN_QUOTA/500000" | bc) USD)

-- 步骤1: 创建临时分配表
DROP TABLE IF EXISTS temp_token_allocation;
CREATE TEMPORARY TABLE temp_token_allocation AS
SELECT
    t.id as token_id,
    t.user_id,
    u.quota as user_quota,
    COUNT(*) OVER (PARTITION BY t.user_id) as token_count,
    CASE
        -- 单Token用户：100%用户额度，最小$MIN_QUOTA
        WHEN COUNT(*) OVER (PARTITION BY t.user_id) = 1 THEN
            GREATEST(u.quota, $MIN_QUOTA)
        -- 多Token用户：按比例分配，最小$MIN_QUOTA
        ELSE
            GREATEST(
                FLOOR(u.quota / COUNT(*) OVER (PARTITION BY t.user_id)),
                $MIN_QUOTA
            )
    END as allocated_quota
FROM tokens t
LEFT JOIN users u ON t.user_id = u.id
WHERE t.unlimited_quota = 0;  -- 只处理非无限额度的Token

-- 步骤2: 查看迁移影响（仅展示前20条）
SELECT
    token_id,
    user_id,
    user_quota,
    token_count,
    allocated_quota,
    CONCAT('$', ROUND(allocated_quota/500000, 2)) as allocated_usd
FROM temp_token_allocation
LIMIT 20;

-- 步骤3: 更新Token的RemainQuota
UPDATE tokens t
INNER JOIN temp_token_allocation a ON t.id = a.token_id
SET t.remain_quota = a.allocated_quota;

-- 步骤4: 验证迁移结果
SELECT
    'Total Tokens Migrated' as metric,
    COUNT(*) as value
FROM temp_token_allocation
UNION ALL
SELECT
    'Min Allocated Quota' as metric,
    MIN(allocated_quota) as value
FROM temp_token_allocation
UNION ALL
SELECT
    'Max Allocated Quota' as metric,
    MAX(allocated_quota) as value
FROM temp_token_allocation
UNION ALL
SELECT
    'Avg Allocated Quota' as metric,
    ROUND(AVG(allocated_quota)) as value
FROM temp_token_allocation;

-- 清理临时表
DROP TABLE IF EXISTS temp_token_allocation;
EOSQL
            )
            ;;
        sqlite)
            sql_content=$(cat <<'EOSQL'
-- SQLite版本的迁移脚本
-- 注意：SQLite不支持WINDOW函数和临时表，需要分步执行

-- 步骤1: 查看现有Token分布
SELECT
    user_id,
    COUNT(*) as token_count,
    SUM(remain_quota) as total_remain_quota
FROM tokens
WHERE unlimited_quota = 0
GROUP BY user_id;

-- 步骤2: 单Token用户更新（100%用户额度或最小$MIN_QUOTA）
UPDATE tokens
SET remain_quota = (
    SELECT CASE
        WHEN u.quota > $MIN_QUOTA THEN u.quota
        ELSE $MIN_QUOTA
    END
    FROM users u
    WHERE u.id = tokens.user_id
)
WHERE unlimited_quota = 0
AND user_id IN (
    SELECT user_id
    FROM tokens
    WHERE unlimited_quota = 0
    GROUP BY user_id
    HAVING COUNT(*) = 1
);

-- 步骤3: 多Token用户更新（按比例分配）
-- 注意：SQLite需要手动处理，建议使用MySQL或PostgreSQL
EOSQL
            )
            ;;
    esac

    # 替换变量
    echo "$sql_content" | sed "s/\$MIN_QUOTA/$MIN_QUOTA/g"
}

# 执行MySQL数据库备份
backup_mysql() {
    local backup_file="$BACKUP_DIR/new_api_backup_$(date +%Y%m%d_%H%M%S).sql"

    log_info "开始备份MySQL数据库..."
    mkdir -p "$BACKUP_DIR"

    local mysql_cmd="mysqldump -h $DB_HOST -P $DB_PORT -u $DB_USER"
    if [ -n "$DB_PASSWORD" ]; then
        mysql_cmd="$mysql_cmd -p$DB_PASSWORD"
    fi
    mysql_cmd="$mysql_cmd $DB_NAME"

    if $mysql_cmd > "$backup_file" 2>/dev/null; then
        log_success "备份完成: $backup_file"
        echo "$backup_file"
    else
        log_error "备份失败！"
        return 1
    fi
}

# 执行PostgreSQL数据库备份
backup_postgres() {
    local backup_file="$BACKUP_DIR/new_api_backup_$(date +%Y%m%d_%H%M%S).sql"

    log_info "开始备份PostgreSQL数据库..."
    mkdir -p "$BACKUP_DIR"

    export PGPASSWORD="$DB_PASSWORD"
    if pg_dump -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" "$DB_NAME" > "$backup_file" 2>/dev/null; then
        log_success "备份完成: $backup_file"
        echo "$backup_file"
    else
        log_error "备份失败！"
        return 1
    fi
}

# 执行SQL迁移
execute_migration() {
    local sql_script="$1"

    case $DB_TYPE in
        mysql)
            local mysql_cmd="mysql -h $DB_HOST -P $DB_PORT -u $DB_USER"
            if [ -n "$DB_PASSWORD" ]; then
                mysql_cmd="$mysql_cmd -p$DB_PASSWORD"
            fi
            mysql_cmd="$mysql_cmd $DB_NAME"

            echo "$sql_script" | $mysql_cmd
            ;;
        postgres)
            export PGPASSWORD="$DB_PASSWORD"
            echo "$sql_script" | psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" "$DB_NAME"
            ;;
        sqlite)
            echo "$sql_script" | sqlite3 "$DB_NAME"
            ;;
    esac
}

# 验证迁移结果
verify_migration() {
    log_info "验证迁移结果..."

    local verify_sql="SELECT COUNT(*) as total_tokens FROM tokens WHERE unlimited_quota = 0;"
    local result=$(execute_migration "$verify_sql")

    log_info "迁移的Token总数: $result"
}

# 主程序
main() {
    log_info "==========================================="
    log_info "Token独立额度数据迁移工具"
    log_info "==========================================="
    log_info "数据库类型: $DB_TYPE"
    log_info "数据库地址: $DB_HOST:$DB_PORT"
    log_info "数据库名称: $DB_NAME"
    log_info "最小额度: $MIN_QUOTA (约 \$$(echo "scale=2; $MIN_QUOTA/500000" | bc) USD)"
    log_info "==========================================="

    # 生成SQL脚本
    SQL_SCRIPT=$(generate_sql)

    # 如果只生成SQL文件
    if [ -n "$SQL_FILE" ]; then
        echo "$SQL_SCRIPT" > "$SQL_FILE"
        log_success "SQL脚本已生成: $SQL_FILE"
        exit 0
    fi

    # Dry-run模式
    if [ $DRY_RUN -eq 1 ]; then
        log_warning "=== DRY-RUN 模式 ==="
        echo "$SQL_SCRIPT"
        log_warning "=== 以上为将要执行的SQL，未实际执行 ==="
        exit 0
    fi

    # 确认执行
    log_warning "此操作将修改数据库中的Token额度数据！"
    read -p "是否继续？(y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "取消迁移"
        exit 0
    fi

    # 数据库备份
    if [ $SKIP_BACKUP -eq 0 ]; then
        case $DB_TYPE in
            mysql)
                BACKUP_FILE=$(backup_mysql) || exit 1
                ;;
            postgres)
                BACKUP_FILE=$(backup_postgres) || exit 1
                ;;
            sqlite)
                BACKUP_FILE="$BACKUP_DIR/new_api_backup_$(date +%Y%m%d_%H%M%S).db"
                cp "$DB_NAME" "$BACKUP_FILE"
                log_success "备份完成: $BACKUP_FILE"
                ;;
        esac
    else
        log_warning "跳过数据库备份（不推荐）"
    fi

    # 执行迁移
    log_info "开始执行数据迁移..."
    if execute_migration "$SQL_SCRIPT"; then
        log_success "数据迁移完成！"
        verify_migration
    else
        log_error "迁移失败！"
        if [ -n "$BACKUP_FILE" ]; then
            log_warning "可使用备份文件恢复: $BACKUP_FILE"
        fi
        exit 1
    fi

    log_success "==========================================="
    log_success "迁移成功完成！"
    log_success "备份文件: ${BACKUP_FILE:-无}"
    log_success "==========================================="
}

# 执行主程序
main
