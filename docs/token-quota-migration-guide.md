# Token独立额度数据迁移指南

## 概述

本指南说明如何使用 `migrate-token-quota.sh` 脚本将现有Token从**共享用户余额模式**迁移到**独立额度模式**。

## 迁移策略

### 额度分配规则

1. **单Token用户**：Token获得100%用户额度，最小保证 $20 (10,000,000 quota)
2. **多Token用户**：按比例平均分配，每个Token最小保证 $20
3. **V2平台Token**：自动识别并跳过（使用UnlimitedQuota=true）

### 示例

| 场景 | User余额 | Token数量 | 每Token分配额度 |
|------|----------|----------|----------------|
| 单Token用户（余额充足） | $100 | 1 | $100 |
| 单Token用户（余额不足） | $5 | 1 | $20（最小保证） |
| 多Token用户（余额充足） | $100 | 5 | $20（平均分配） |
| 多Token用户（余额不足） | $30 | 5 | $20（最小保证，总计$100） |

## 快速开始

### 1. 生产环境执行（推荐）

```bash
# MySQL数据库
cd /path/to/new-api
./scripts/migrate-token-quota.sh \
    -d mysql \
    -H localhost \
    -P 3306 \
    -u root \
    -D new_api_prod

# 会提示输入密码和确认
```

### 2. 模拟执行（查看影响）

```bash
# Dry-run模式，不修改数据
./scripts/migrate-token-quota.sh \
    -d mysql \
    -u root \
    -D new_api_dev \
    --dry-run
```

### 3. 仅生成SQL文件

```bash
# 生成SQL脚本供DBA审查
./scripts/migrate-token-quota.sh \
    -d mysql \
    -s /tmp/migration.sql
```

## 完整参数说明

```bash
选项:
    -h, --help              显示帮助信息
    -d, --database DB_TYPE  指定数据库类型 (mysql/postgres/sqlite)
    -H, --host HOST         数据库主机 (默认: localhost)
    -P, --port PORT         数据库端口
    -u, --user USER         数据库用户 (默认: root)
    -p, --password PASS     数据库密码（不推荐，建议交互式输入）
    -D, --dbname NAME       数据库名称 (默认: new_api_dev)
    -s, --sql-file FILE     仅生成SQL文件
    --dry-run               模拟执行，不修改数据
    --min-quota AMOUNT      最小保证额度 (默认: 10000000)
    --skip-backup           跳过数据库备份（不推荐）
    --backup-dir DIR        备份目录 (默认: ./backups)
```

## 执行流程

脚本执行的完整流程：

```
1. 参数验证
   ↓
2. 显示配置信息
   ↓
3. 用户确认
   ↓
4. 数据库备份（自动创建backup文件）
   ↓
5. 创建临时分配表（计算每个Token应分配的额度）
   ↓
6. 显示迁移影响（前20条记录预览）
   ↓
7. 执行Token额度更新
   ↓
8. 验证迁移结果
   ↓
9. 显示统计信息
```

## 安全措施

### 自动备份

脚本默认会在迁移前自动备份数据库：

```bash
# 备份文件命名格式
backups/new_api_backup_20251118_143052.sql

# 备份位置
默认：./backups/
自定义：--backup-dir /path/to/backups
```

### 回滚方法

如果迁移出现问题，可使用备份恢复：

```bash
# MySQL
mysql -u root -p new_api_prod < backups/new_api_backup_20251118_143052.sql

# PostgreSQL
psql -U postgres new_api_prod < backups/new_api_backup_20251118_143052.sql

# SQLite
cp backups/new_api_backup_20251118_143052.db new_api.db
```

## 使用示例

### 示例1：开发环境测试

```bash
# 1. 先dry-run查看影响
./scripts/migrate-token-quota.sh \
    -d mysql \
    -u root \
    -D new_api_dev \
    --dry-run

# 2. 确认无误后实际执行
./scripts/migrate-token-quota.sh \
    -d mysql \
    -u root \
    -D new_api_dev

# 输入密码并确认
```

### 示例2：生产环境（PostgreSQL）

```bash
# 使用PostgreSQL数据库
./scripts/migrate-token-quota.sh \
    -d postgres \
    -H db.example.com \
    -P 5432 \
    -u postgres \
    -D new_api_prod \
    --backup-dir /mnt/backups
```

### 示例3：Docker环境

```bash
# 如果数据库在Docker容器中
# MySQL示例
docker exec -i mysql-container sh -c "exec mysql -uroot -p'password' new_api" < /tmp/migration.sql

# 或直接在容器内执行
docker exec -it new-api-backend bash
cd /app
./scripts/migrate-token-quota.sh -d mysql -u root -D new_api
```

## 验证迁移结果

### 1. 检查Token数量

```sql
-- 查看迁移的Token总数
SELECT COUNT(*) as total_tokens
FROM tokens
WHERE unlimited_quota = 0;
```

### 2. 检查额度分配

```sql
-- 查看额度分配统计
SELECT
    MIN(remain_quota) as min_quota,
    MAX(remain_quota) as max_quota,
    AVG(remain_quota) as avg_quota,
    COUNT(*) as total_tokens
FROM tokens
WHERE unlimited_quota = 0;
```

### 3. 检查单Token用户

```sql
-- 验证单Token用户是否获得100%额度
SELECT
    t.id,
    t.user_id,
    t.remain_quota as token_quota,
    u.quota as user_quota
FROM tokens t
LEFT JOIN users u ON t.user_id = u.id
WHERE t.unlimited_quota = 0
AND t.user_id IN (
    SELECT user_id
    FROM tokens
    WHERE unlimited_quota = 0
    GROUP BY user_id
    HAVING COUNT(*) = 1
)
LIMIT 10;
```

## 故障排除

### 问题1：权限不足

**错误**：`ERROR 1045 (28000): Access denied`

**解决**：
```bash
# 确认数据库用户有足够权限
# MySQL
GRANT ALL PRIVILEGES ON new_api.* TO 'your_user'@'localhost';
FLUSH PRIVILEGES;
```

### 问题2：临时表创建失败

**错误**：`Can't create temporary table`

**解决**：
```bash
# MySQL检查tmp_table权限
SHOW VARIABLES LIKE 'tmp_table_size';

# 或使用更高权限的用户执行
```

### 问题3：备份空间不足

**错误**：磁盘空间不足

**解决**：
```bash
# 指定备份到其他磁盘
./scripts/migrate-token-quota.sh \
    --backup-dir /mnt/large-disk/backups \
    ...
```

## 注意事项

### ⚠️ 执行前必读

1. **测试环境验证**：生产前必须在测试环境完整执行一次
2. **业务低峰期**：建议在业务低峰期执行，避免影响用户
3. **停止相关服务**：迁移期间建议暂停API服务，避免并发冲突
4. **备份验证**：执行前确认备份文件完整且可恢复
5. **权限检查**：确保数据库用户有CREATE TEMPORARY TABLE权限

### 📊 预估影响

迁移脚本执行时间取决于Token数量：

| Token数量 | 预估时间 | 备份大小 |
|----------|---------|---------|
| < 1,000 | < 10秒 | < 10MB |
| 1,000 - 10,000 | < 1分钟 | 10-100MB |
| 10,000 - 100,000 | < 5分钟 | 100MB-1GB |
| > 100,000 | 视硬件而定 | > 1GB |

## 后续步骤

迁移完成后：

1. ✅ 验证关键用户的Token额度是否正确
2. ✅ 重启API服务，应用新的计费逻辑
3. ✅ 监控日志，确认无异常错误
4. ✅ 更新API文档，通知前端团队参数变更
5. ✅ 保留备份文件至少7天

## 技术支持

如遇问题，请提供以下信息：

- 数据库类型和版本
- 错误日志完整输出
- Token数量规模
- 备份文件是否创建成功

## 附录：手动SQL脚本

如果需要手动执行SQL（不使用脚本），可参考以下示例：

```sql
-- 手动迁移SQL脚本（MySQL）
-- 步骤1: 创建临时分配表
DROP TABLE IF EXISTS temp_token_allocation;
CREATE TEMPORARY TABLE temp_token_allocation AS
SELECT
    t.id as token_id,
    t.user_id,
    u.quota as user_quota,
    COUNT(*) OVER (PARTITION BY t.user_id) as token_count,
    CASE
        WHEN COUNT(*) OVER (PARTITION BY t.user_id) = 1 THEN
            GREATEST(u.quota, 10000000)
        ELSE
            GREATEST(
                FLOOR(u.quota / COUNT(*) OVER (PARTITION BY t.user_id)),
                10000000
            )
    END as allocated_quota
FROM tokens t
LEFT JOIN users u ON t.user_id = u.id
WHERE t.unlimited_quota = 0;

-- 步骤2: 更新Token额度
UPDATE tokens t
INNER JOIN temp_token_allocation a ON t.id = a.token_id
SET t.remain_quota = a.allocated_quota;

-- 步骤3: 验证结果
SELECT
    'Total Tokens' as metric,
    COUNT(*) as value
FROM temp_token_allocation
UNION ALL
SELECT 'Min Quota', MIN(allocated_quota)
FROM temp_token_allocation
UNION ALL
SELECT 'Max Quota', MAX(allocated_quota)
FROM temp_token_allocation;

-- 清理
DROP TABLE IF EXISTS temp_token_allocation;
```

---

**版本**: v1.0
**更新日期**: 2025-11-18
**维护者**: New API Team
