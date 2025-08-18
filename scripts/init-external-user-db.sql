-- 外部用户系统集成数据库初始化脚本
-- 作者: Claude Code Assistant
-- 日期: 2025-01-30

USE new_api_dev;

-- 检查并添加 external_user_id 字段
SET @sql = (
    SELECT IF(
        COUNT(*) = 0,
        'ALTER TABLE users ADD COLUMN external_user_id VARCHAR(100) DEFAULT '''' AFTER id;',
        'SELECT ''external_user_id 字段已存在'' as message;'
    )
    FROM information_schema.COLUMNS 
    WHERE TABLE_SCHEMA = 'new_api_dev' 
    AND TABLE_NAME = 'users' 
    AND COLUMN_NAME = 'external_user_id'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 检查并创建普通索引（非唯一索引，避免空值冲突）
SET @sql = (
    SELECT IF(
        COUNT(*) = 0,
        'CREATE INDEX idx_users_external_user_id ON users(external_user_id);',
        'SELECT ''索引 idx_users_external_user_id 已存在'' as message;'
    )
    FROM information_schema.STATISTICS 
    WHERE TABLE_SCHEMA = 'new_api_dev' 
    AND TABLE_NAME = 'users' 
    AND INDEX_NAME = 'idx_users_external_user_id'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加其他可能需要的字段（如果不存在）
-- 手机号字段
SET @sql = (
    SELECT IF(
        COUNT(*) = 0,
        'ALTER TABLE users ADD COLUMN phone VARCHAR(20) DEFAULT '''' AFTER email;',
        'SELECT ''phone 字段已存在'' as message;'
    )
    FROM information_schema.COLUMNS 
    WHERE TABLE_SCHEMA = 'new_api_dev' 
    AND TABLE_NAME = 'users' 
    AND COLUMN_NAME = 'phone'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 微信OpenID字段
SET @sql = (
    SELECT IF(
        COUNT(*) = 0,
        'ALTER TABLE users ADD COLUMN wechat_openid VARCHAR(100) DEFAULT '''' AFTER phone;',
        'SELECT ''wechat_openid 字段已存在'' as message;'
    )
    FROM information_schema.COLUMNS 
    WHERE TABLE_SCHEMA = 'new_api_dev' 
    AND TABLE_NAME = 'users' 
    AND COLUMN_NAME = 'wechat_openid'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 微信UnionID字段
SET @sql = (
    SELECT IF(
        COUNT(*) = 0,
        'ALTER TABLE users ADD COLUMN wechat_unionid VARCHAR(100) DEFAULT '''' AFTER wechat_openid;',
        'SELECT ''wechat_unionid 字段已存在'' as message;'
    )
    FROM information_schema.COLUMNS 
    WHERE TABLE_SCHEMA = 'new_api_dev' 
    AND TABLE_NAME = 'users' 
    AND COLUMN_NAME = 'wechat_unionid'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 支付宝用户ID字段
SET @sql = (
    SELECT IF(
        COUNT(*) = 0,
        'ALTER TABLE users ADD COLUMN alipay_userid VARCHAR(100) DEFAULT '''' AFTER wechat_unionid;',
        'SELECT ''alipay_userid 字段已存在'' as message;'
    )
    FROM information_schema.COLUMNS 
    WHERE TABLE_SCHEMA = 'new_api_dev' 
    AND TABLE_NAME = 'users' 
    AND COLUMN_NAME = 'alipay_userid'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 登录类型字段
SET @sql = (
    SELECT IF(
        COUNT(*) = 0,
        'ALTER TABLE users ADD COLUMN login_type VARCHAR(20) DEFAULT ''email'' AFTER alipay_userid;',
        'SELECT ''login_type 字段已存在'' as message;'
    )
    FROM information_schema.COLUMNS 
    WHERE TABLE_SCHEMA = 'new_api_dev' 
    AND TABLE_NAME = 'users' 
    AND COLUMN_NAME = 'login_type'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 是否外部用户标识
SET @sql = (
    SELECT IF(
        COUNT(*) = 0,
        'ALTER TABLE users ADD COLUMN is_external BOOLEAN DEFAULT false AFTER login_type;',
        'SELECT ''is_external 字段已存在'' as message;'
    )
    FROM information_schema.COLUMNS 
    WHERE TABLE_SCHEMA = 'new_api_dev' 
    AND TABLE_NAME = 'users' 
    AND COLUMN_NAME = 'is_external'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 外部用户扩展数据字段
SET @sql = (
    SELECT IF(
        COUNT(*) = 0,
        'ALTER TABLE users ADD COLUMN external_data TEXT AFTER is_external;',
        'SELECT ''external_data 字段已存在'' as message;'
    )
    FROM information_schema.COLUMNS 
    WHERE TABLE_SCHEMA = 'new_api_dev' 
    AND TABLE_NAME = 'users' 
    AND COLUMN_NAME = 'external_data'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 显示最终的表结构
SHOW CREATE TABLE users;

-- 显示用户统计信息
SELECT 
    COUNT(*) as total_users,
    COUNT(CASE WHEN is_external = 1 THEN 1 END) as external_users,
    COUNT(CASE WHEN external_user_id != '' THEN 1 END) as users_with_external_id
FROM users;

SELECT '✅ 外部用户系统数据库初始化完成！' as status;