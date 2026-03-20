package model

import (
	"fmt"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// WisemodelPackage Wisemodel资源包模型
type WisemodelPackage struct {
	Id              int        `json:"id" gorm:"primaryKey"`
	UserId          int        `json:"user_id" gorm:"type:int;not null;index"`
	PackageId       string     `json:"package_id" gorm:"type:varchar(50);not null;uniqueIndex"`
	OrderId         string     `json:"order_id" gorm:"type:varchar(50);not null;index"`

	// 原始值（Wisemodel传来的，仅用于显示）
	OriginalPoints  int        `json:"original_points" gorm:"type:int;default:0"`
	OriginalTokens  int        `json:"original_tokens" gorm:"type:int;default:0"`

	// 转换后的quota
	QuotaGranted    int64      `json:"quota_granted" gorm:"type:bigint;not null"`

	// 可用模型列表（逗号分隔，仅供展示）
	AvailableModels string     `json:"available_models" gorm:"type:text"`

	Amount          float64    `json:"amount" gorm:"type:decimal(10,2);not null"`
	IsFree          bool       `json:"is_free" gorm:"type:boolean;default:false"`
	ValidUntil      *time.Time `json:"valid_until" gorm:"type:datetime"`
	CreatedAt       time.Time  `json:"created_at" gorm:"type:datetime;default:CURRENT_TIMESTAMP"`
	ReclaimedAt     *time.Time `json:"reclaimed_at" gorm:"type:datetime"`
}

// TableName 指定表名
func (WisemodelPackage) TableName() string {
	return "wisemodel_packages"
}

// GetWisemodelPackageByPackageId 根据PackageId查询资源包
func GetWisemodelPackageByPackageId(packageId string) (*WisemodelPackage, error) {
	var pkg WisemodelPackage
	err := DB.Where("package_id = ?", packageId).First(&pkg).Error
	return &pkg, err
}

// GetWisemodelPackagesByUserId 查询用户的所有资源包
func GetWisemodelPackagesByUserId(userId int) ([]*WisemodelPackage, error) {
	var packages []*WisemodelPackage
	err := DB.Where("user_id = ?", userId).Order("created_at DESC").Find(&packages).Error
	return packages, err
}

// GetActiveWisemodelPackages 查询用户的有效资源包（未过期）
func GetActiveWisemodelPackages(userId int) ([]*WisemodelPackage, error) {
	var packages []*WisemodelPackage
	now := time.Now()
	err := DB.Where("user_id = ? AND (valid_until IS NULL OR valid_until > ?)", userId, now).
		Order("created_at DESC").
		Find(&packages).Error
	return packages, err
}

// HasPaidPackages 检查用户是否有付费资源包
func HasPaidPackages(userId int) (bool, error) {
	var count int64
	err := DB.Model(&WisemodelPackage{}).
		Where("user_id = ? AND is_free = ? AND amount > ?", userId, false, 0).
		Count(&count).Error
	return count > 0, err
}

// CreateWisemodelPackage 创建资源包记录
func CreateWisemodelPackage(pkg *WisemodelPackage) error {
	return DB.Create(pkg).Error
}

// DeleteWisemodelPackagesByUserId 删除用户的所有资源包
func DeleteWisemodelPackagesByUserId(userId int) error {
	return DB.Where("user_id = ?", userId).Delete(&WisemodelPackage{}).Error
}

// MigrateWisemodelPackage 数据库迁移
func MigrateWisemodelPackage(db *gorm.DB) error {
	return db.AutoMigrate(&WisemodelPackage{})
}

// SortPackagesByValidUntil 按 ValidUntil ASC 排序，nil（永久包）排最后
func SortPackagesByValidUntil(packages []*WisemodelPackage) {
	sort.Slice(packages, func(i, j int) bool {
		if packages[i].ValidUntil == nil {
			return false
		}
		if packages[j].ValidUntil == nil {
			return true
		}
		return packages[i].ValidUntil.Before(*packages[j].ValidUntil)
	})
}

// ReclaimExpiredPackages 回收指定用户所有已过期但未回收包的剩余 quota。
// 使用乐观锁（UPDATE WHERE reclaimed_at IS NULL）防止并发重复回收。
func ReclaimExpiredPackages(userId int) error {
	now := time.Now()
	var packages []*WisemodelPackage
	if err := DB.Where("user_id = ? AND valid_until < ? AND reclaimed_at IS NULL", userId, now).
		Find(&packages).Error; err != nil {
		return err
	}
	for _, pkg := range packages {
		// 1. 估算该包时间窗口内的消费（近似值）
		var consumed int64
		if err := DB.Model(&Log{}).
			Where("user_id = ? AND type = ? AND created_at >= ? AND created_at < ?",
				userId, LogTypeConsume, pkg.CreatedAt.Unix(), pkg.ValidUntil.Unix()).
			Select("COALESCE(SUM(quota), 0)").Scan(&consumed).Error; err != nil {
			return err
		}

		refund := pkg.QuotaGranted - consumed
		if refund < 0 {
			refund = 0
		}

		// 2. 事务内：先原子抢占 reclaimed_at，再扣 quota
		err := DB.Transaction(func(tx *gorm.DB) error {
			result := tx.Model(&WisemodelPackage{}).
				Where("id = ? AND reclaimed_at IS NULL", pkg.Id).
				Update("reclaimed_at", now)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				// 已被其他 goroutine 处理，跳过
				return nil
			}
			if refund > 0 {
				return tx.Model(&User{}).Where("id = ?", userId).
					UpdateColumn("quota", gorm.Expr("quota - ?", refund)).Error
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// AttributeLogsToPackages 按 FIFO 规则将 logs 归属到 packages。
// packages 必须已按 ValidUntil ASC（nil 排最后）排序。
// logs 必须已按 CreatedAt ASC 排序。
// 返回 map[packageId] -> 归属消费 quota 之和。
func AttributeLogsToPackages(packages []*WisemodelPackage, logs []Log) map[string]int64 {
	result := make(map[string]int64, len(packages))
	for _, pkg := range packages {
		result[pkg.PackageId] = 0
	}
	for _, log := range logs {
		logTime := log.CreatedAt // Unix 秒
		for _, pkg := range packages {
			if logTime < pkg.CreatedAt.Unix() {
				continue // log 早于此包创建时间
			}
			if pkg.ValidUntil != nil && logTime >= pkg.ValidUntil.Unix() {
				continue // log 晚于此包到期时间
			}
			// 此包在 log 时刻有效，FIFO：归属第一个匹配包
			result[pkg.PackageId] += int64(log.Quota)
			break
		}
		// 若无匹配包，此 log 忽略
	}
	return result
}

// ReclaimAllExpiredPackages 全局扫描所有用户的过期未回收包，供后台定时任务调用。
func ReclaimAllExpiredPackages() error {
	var userIds []int
	if err := DB.Model(&WisemodelPackage{}).
		Where("valid_until < ? AND reclaimed_at IS NULL", time.Now()).
		Distinct("user_id").Pluck("user_id", &userIds).Error; err != nil {
		return err
	}
	var lastErr error
	for _, userId := range userIds {
		if err := ReclaimExpiredPackages(userId); err != nil {
			common.SysError(fmt.Sprintf("ReclaimAllExpiredPackages: userId %d failed: %v", userId, err))
			lastErr = err
		}
	}
	return lastErr
}
