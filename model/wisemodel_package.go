package model

import (
	"time"

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
