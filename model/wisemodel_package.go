package model

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Wisemodel Points/Tokens 换算常量
// 定价: 100,000,000 points = $100 USD → 1,000,000 points = $1 = QuotaPerUnit quota
const (
	WisemodelPointsPerUnit = 1_000_000
	WisemodelTokensPerUnit = 1_000_000
)

// WisemodelPackage Wisemodel资源包模型
type WisemodelPackage struct {
	Id                int    `json:"id" gorm:"primaryKey"`
	UserId            int    `json:"user_id" gorm:"type:int;not null;index"`
	PackageId         string `json:"package_id" gorm:"type:varchar(50);not null;uniqueIndex"`
	OriginalPackageId string `json:"original_package_id" gorm:"type:varchar(50);default:'';index"`
	OrderId           string `json:"order_id" gorm:"type:varchar(50);not null;index"`

	// 原始值（Wisemodel传来的，仅用于显示）
	OriginalPoints int `json:"original_points" gorm:"type:int;default:0"`
	OriginalTokens int `json:"original_tokens" gorm:"type:int;default:0"`

	// 转换后的quota
	QuotaGranted int64 `json:"quota_granted" gorm:"type:bigint;not null"`

	// 可用模型列表（逗号分隔，仅供展示）
	AvailableModels string `json:"available_models" gorm:"type:text"`

	Amount      float64    `json:"amount" gorm:"type:decimal(10,2);not null"`
	IsFree      bool       `json:"is_free" gorm:"type:boolean;default:false"`
	ValidUntil  *time.Time `json:"valid_until" gorm:"type:datetime"`
	CreatedAt   time.Time  `json:"created_at" gorm:"type:datetime;default:CURRENT_TIMESTAMP"`
	ReclaimedAt *time.Time `json:"reclaimed_at" gorm:"type:datetime"`
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

// CreateWisemodelPackage 创建资源包记录，并初始化 Redis 配额计数器。
func CreateWisemodelPackage(pkg *WisemodelPackage) error {
	if err := DB.Create(pkg).Error; err != nil {
		return err
	}
	// 新包消费为零，计数器 = 总配额；供 Layer-2 原子预扣使用
	common.RDB.Set(context.Background(), "wm:pkg:remain:"+pkg.PackageId, pkg.QuotaGranted, 0)
	return nil
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
		if packages[i].ValidUntil.Equal(*packages[j].ValidUntil) {
			if !packages[i].CreatedAt.Equal(packages[j].CreatedAt) {
				return packages[i].CreatedAt.Before(packages[j].CreatedAt)
			}
			return packages[i].PackageId < packages[j].PackageId
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
		if err := LOG_DB.Model(&Log{}).
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
		// 清理 Redis 计数器，防止过期包的残留计数影响统计
		common.RDB.Del(context.Background(), "wm:pkg:remain:"+pkg.PackageId)
	}
	return nil
}

// RestoreOriginalPackageId recovers the upstream package ID from the internal record ID.
// Legacy rows may not have OriginalPackageId populated yet.
func RestoreOriginalPackageId(packageId string) string {
	idx := strings.LastIndex(packageId, "_")
	if idx <= 0 || idx == len(packageId)-1 {
		return packageId
	}
	suffix := packageId[idx+1:]
	for _, ch := range suffix {
		if ch < '0' || ch > '9' {
			return packageId
		}
	}
	return packageId[:idx]
}

func (pkg *WisemodelPackage) DisplayPackageId() string {
	if pkg == nil {
		return ""
	}
	if pkg.OriginalPackageId != "" {
		return pkg.OriginalPackageId
	}
	return RestoreOriginalPackageId(pkg.PackageId)
}

// AttributeLogsToPackages 按 FIFO 规则将 logs 归属到 packages。
// packages 必须已按 ValidUntil ASC（nil 排最后）排序。
// logs 必须已按 CreatedAt ASC 排序。
// 返回 map[packageId] -> 归属消费 quota 之和。
func AttributeLogsToPackages(packages []*WisemodelPackage, logs []Log) map[string]int64 {
	return AttributeLogsToPackagesWithBaseline(packages, logs, nil)
}

// AttributeLogsToPackagesWithBaseline attributes old logs by FIFO while respecting
// each package's remaining capacity after baseline consumption has already been
// precisely attributed.
func AttributeLogsToPackagesWithBaseline(packages []*WisemodelPackage, logs []Log, baseline map[string]int64) map[string]int64 {
	result := make(map[string]int64, len(packages))
	consumed := make(map[string]int64, len(packages))
	for _, pkg := range packages {
		result[pkg.PackageId] = 0
		if baseline != nil {
			consumed[pkg.PackageId] = baseline[pkg.PackageId]
		}
	}
	for _, log := range logs {
		logTime := log.CreatedAt // Unix 秒
		remainingQuota := int64(log.Quota)
		if remainingQuota <= 0 {
			continue
		}
		for _, pkg := range packages {
			if logTime < pkg.CreatedAt.Unix() {
				continue // log 早于此包创建时间
			}
			if pkg.ValidUntil != nil && logTime >= pkg.ValidUntil.Unix() {
				continue // log 晚于此包到期时间
			}
			available := pkg.QuotaGranted - consumed[pkg.PackageId]
			if available <= 0 {
				continue
			}
			take := remainingQuota
			if take > available {
				take = available
			}
			result[pkg.PackageId] += take
			consumed[pkg.PackageId] += take
			remainingQuota -= take
			if remainingQuota == 0 {
				break
			}
		}
		// 若无匹配包，此 log 忽略
	}
	return result
}

// ModelUsageRow 用于 GetModelUsageByPackages 的数据库扫描结果。
// 导出供 controller 包引用。
type ModelUsageRow struct {
	WisemodelPackageId string `gorm:"column:wisemodel_package_id"`
	ModelName          string `gorm:"column:model_name"`
	UsedQuota          int64  `gorm:"column:used_quota"`
}

// GetModelUsageByPackages 批量查询多个资源包的 per-model quota 消费。
// 返回 map[pkgId] → 按 used_quota DESC 排序的消费明细列表。
// 只统计 wisemodel_package_id 精确匹配的日志（精确归因），FIFO 历史日志不纳入。
func GetModelUsageByPackages(pkgIds []string) (map[string][]ModelUsageRow, error) {
	if len(pkgIds) == 0 {
		return map[string][]ModelUsageRow{}, nil
	}
	var rows []ModelUsageRow
	err := LOG_DB.Model(&Log{}).
		Where("wisemodel_package_id IN ? AND type = ?", pkgIds, LogTypeConsume).
		Group("wisemodel_package_id, model_name").
		Select("wisemodel_package_id, model_name, COALESCE(SUM(quota), 0) AS used_quota").
		Order("used_quota DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string][]ModelUsageRow, len(pkgIds))
	for _, r := range rows {
		result[r.WisemodelPackageId] = append(result[r.WisemodelPackageId], r)
	}
	return result, nil
}

func BuildPackageUsageRows(packages []*WisemodelPackage, attribution map[string]int64, modelMap map[string][]ModelUsageRow) []map[string]interface{} {
	rows := make([]map[string]interface{}, 0, len(packages))
	for _, pkg := range packages {
		consumed := attribution[pkg.PackageId]

		availableModels := []string{}
		if pkg.AvailableModels != "" {
			for _, modelName := range strings.Split(pkg.AvailableModels, ",") {
				if modelName = strings.TrimSpace(modelName); modelName != "" {
					availableModels = append(availableModels, modelName)
				}
			}
		}

		// 构建 per-model details
		details := []interface{}{}
		for _, u := range modelMap[pkg.PackageId] {
			if pkg.OriginalPoints > 0 {
				usedAmount := u.UsedQuota * WisemodelPointsPerUnit / int64(common.QuotaPerUnit)
				details = append(details, map[string]interface{}{
					"model_name":  u.ModelName,
					"used_amount": usedAmount,
				})
			} else {
				usedAmount := u.UsedQuota * WisemodelTokensPerUnit / int64(common.QuotaPerUnit)
				details = append(details, map[string]interface{}{
					"model_name":         u.ModelName,
					"used_amount_tokens": usedAmount,
				})
			}
		}

		row := map[string]interface{}{
			"package_id":        pkg.DisplayPackageId(),
			"package_record_id": pkg.PackageId,
			"available_models":  availableModels,
			"details":           details,
		}

		if pkg.OriginalPoints > 0 {
			remainPoints := (pkg.QuotaGranted - consumed) * WisemodelPointsPerUnit / int64(common.QuotaPerUnit)
			if remainPoints < 0 {
				remainPoints = 0
			}
			row["points"] = pkg.OriginalPoints
			row["remain_points"] = remainPoints
			row["amount"] = int64(pkg.OriginalPoints) - remainPoints
		} else {
			remainTokens := (pkg.QuotaGranted - consumed) * WisemodelTokensPerUnit / int64(common.QuotaPerUnit)
			if remainTokens < 0 {
				remainTokens = 0
			}
			row["tokens"] = pkg.OriginalTokens
			row["remain_tokens"] = remainTokens
			row["amount_tokens"] = int64(pkg.OriginalTokens) - remainTokens
		}

		rows = append(rows, row)
	}
	return rows
}

func CalculatePackageAttribution(userId int, packages []*WisemodelPackage) (map[string]int64, error) {
	attribution := make(map[string]int64, len(packages))
	if len(packages) == 0 {
		return attribution, nil
	}

	SortPackagesByValidUntil(packages)

	minTime := packages[0].CreatedAt.Unix()
	maxTime := time.Now().Unix()
	for _, pkg := range packages {
		if createdAt := pkg.CreatedAt.Unix(); createdAt < minTime {
			minTime = createdAt
		}
		if pkg.ValidUntil != nil && pkg.ValidUntil.Unix() > maxTime {
			maxTime = pkg.ValidUntil.Unix()
		}
	}

	pkgIds := make([]string, len(packages))
	for i, pkg := range packages {
		pkgIds[i] = pkg.PackageId
	}
	preciseAttribution, err := getPreciseAttributionByPackages(pkgIds)
	if err != nil {
		return nil, err
	}

	var oldLogs []Log
	if err := LOG_DB.
		Where("user_id = ? AND type = ? AND (wisemodel_package_id IS NULL OR wisemodel_package_id = '') AND created_at >= ? AND created_at <= ?",
			userId, LogTypeConsume, minTime, maxTime).
		Order("created_at ASC").
		Find(&oldLogs).Error; err != nil {
		return nil, err
	}

	fifoAttribution := AttributeLogsToPackagesWithBaseline(packages, oldLogs, preciseAttribution)
	for _, pkg := range packages {
		attribution[pkg.PackageId] = preciseAttribution[pkg.PackageId] + fifoAttribution[pkg.PackageId]
	}

	return attribution, nil
}

func SelectPackageWithRemainingQuota(packages []*WisemodelPackage, attribution map[string]int64) *WisemodelPackage {
	for _, pkg := range packages {
		if pkg.QuotaGranted-attribution[pkg.PackageId] > 0 {
			return pkg
		}
	}
	return nil
}

func SelectPackageWithSufficientQuota(packages []*WisemodelPackage, attribution map[string]int64, requiredQuota int64) *WisemodelPackage {
	if requiredQuota <= 0 {
		return SelectPackageWithRemainingQuota(packages, attribution)
	}
	for _, pkg := range packages {
		if pkg.QuotaGranted-attribution[pkg.PackageId] >= requiredQuota {
			return pkg
		}
	}
	return nil
}

// FilterPackagesByModel 返回能承载指定模型请求的资源包子集。
// 规则（按包逐个判断）：
//   - 包的 AvailableModels 为空 → 通用包，能承载任意模型
//   - 包的 AvailableModels 非空 → 仅当请求模型在列表中（大小写不敏感）才能承载
//
// requestedModel 为空时直接返回全部包（无法确定模型，不做限制）。
func FilterPackagesByModel(packages []*WisemodelPackage, requestedModel string) []*WisemodelPackage {
	if requestedModel == "" {
		return packages
	}
	requestedModelLower := strings.ToLower(strings.TrimSpace(requestedModel))
	eligible := make([]*WisemodelPackage, 0, len(packages))
	for _, pkg := range packages {
		if pkg.AvailableModels == "" {
			eligible = append(eligible, pkg) // 通用包
			continue
		}
		for _, m := range strings.Split(pkg.AvailableModels, ",") {
			if strings.ToLower(strings.TrimSpace(m)) == requestedModelLower {
				eligible = append(eligible, pkg)
				break
			}
		}
	}
	return eligible
}

// getPreciseAttributionByPackages 批量查询多个资源包的精确消费总和（一次 GROUP BY 替代 N 次单包查询）。
func getPreciseAttributionByPackages(pkgIds []string) (map[string]int64, error) {
	if len(pkgIds) == 0 {
		return map[string]int64{}, nil
	}
	type row struct {
		WisemodelPackageId string `gorm:"column:wisemodel_package_id"`
		UsedQuota          int64  `gorm:"column:used_quota"`
	}
	var rows []row
	err := LOG_DB.Model(&Log{}).
		Where("wisemodel_package_id IN ? AND type = ?", pkgIds, LogTypeConsume).
		Group("wisemodel_package_id").
		Select("wisemodel_package_id, COALESCE(SUM(quota), 0) AS used_quota").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(pkgIds))
	for _, r := range rows {
		result[r.WisemodelPackageId] = r.UsedQuota
	}
	return result, nil
}

// ComputeWisemodelPackageRemain 计算单个资源包的剩余配额（精确归因），供 Redis 计数器懒初始化使用。
func ComputeWisemodelPackageRemain(pkgId string) (int64, error) {
	pkg, err := GetWisemodelPackageByPackageId(pkgId)
	if err != nil {
		return 0, err
	}
	var used int64
	if err := LOG_DB.Model(&Log{}).
		Where("wisemodel_package_id = ? AND type = ?", pkgId, LogTypeConsume).
		Select("COALESCE(SUM(quota), 0)").Scan(&used).Error; err != nil {
		return 0, err
	}
	remain := pkg.QuotaGranted - used
	if remain < 0 {
		remain = 0
	}
	return remain, nil
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
