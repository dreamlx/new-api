package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const wisemodelPkgCandidatesContextKey = "wisemodel_package_candidates"

// ErrWisemodelServiceUnavailable 包裹账本不可用(DB 故障)类错误，供 relay 层映射为
// 可重试的 503，而非把瞬时故障误报成永久的 403「资源包额度耗尽」。
var ErrWisemodelServiceUnavailable = errors.New("wisemodel service temporarily unavailable")

// isWisemodelToken reports whether the request is authenticated with a wisemodel token.
func isWisemodelToken(c *gin.Context) bool {
	if c.GetString("token_name") == "wisemodel-token" {
		return true
	}
	return strings.HasPrefix(c.GetString("token_key"), "wisemodel-")
}

// PrepareWisemodelPackageForPreConsume 选出可承载该请求的 active 资源包候选(按到期 FIFO 序),
// 存入 context 供 PreConsumeWisemodelPkg 原子扣减。本步骤不读取/比较任何额度数字——
// 额度门控由 PreConsumeWisemodelPkg 的原子扣减唯一负责,避免双层判定背离。
func PrepareWisemodelPackageForPreConsume(c *gin.Context, requestedModel string, estimatedQuota int) error {
	if !isWisemodelToken(c) {
		return nil
	}
	userID := c.GetInt("id")
	if userID == 0 {
		return nil
	}

	packages, err := model.GetActiveWisemodelPackages(userID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWisemodelServiceUnavailable, err)
	}
	if len(packages) == 0 {
		c.Set("wisemodel_package_id", "")
		return fmt.Errorf("wisemodel package quota exhausted")
	}

	// 按到期先后排序(ValidUntil ASC, nil 永久包最后),优先消耗最早到期的包。
	model.SortPackagesByValidUntil(packages)

	eligible := packages
	if requestedModel != "" {
		eligible = model.FilterPackagesByModel(packages, requestedModel)
		if len(eligible) == 0 {
			c.Set(wisemodelPkgCandidatesContextKey, []string{})
			c.Set("wisemodel_package_id", "")
			return fmt.Errorf("wisemodel package does not support model %s", requestedModel)
		}
	}

	candidates := make([]string, len(eligible))
	for i, pkg := range eligible {
		candidates[i] = pkg.PackageId
	}
	c.Set(wisemodelPkgCandidatesContextKey, candidates)
	c.Set("wisemodel_package_id", candidates[0])
	return nil
}

// PreConsumeWisemodelPkg 在 active 候选包中按 FIFO 序原子预扣 estimatedQuota。
// 首个扣减成功的包被选中并记入 context;全部不足则报耗尽。DB 故障 fail-closed
// (权威账本不可用时拒绝,避免旁路门控)。
// estimatedQuota <= 0(按次/免费模型,单次成本四舍五入为 0)时不预扣,保留 Prepare 选中的
// 首个候选,由结算按实际量扣减,杜绝按量门控被旁路。
func PreConsumeWisemodelPkg(c *gin.Context, estimatedQuota int) error {
	if !isWisemodelToken(c) {
		return nil
	}

	ids := wisemodelCandidates(c)
	if len(ids) == 0 {
		return nil
	}

	if estimatedQuota <= 0 {
		// 按次/免费模型不预扣，但仍须挑选一个尚有额度的候选包(按 FIFO 序)，
		// 由结算按实际量扣减；全部耗尽则拒绝，杜绝从已耗尽包旁路按量门控。
		for _, pkgId := range ids {
			if pkgId == "" {
				continue
			}
			positive, err := model.PackageRemainPositive(pkgId)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrWisemodelServiceUnavailable, err)
			}
			if !positive {
				continue
			}
			c.Set("wisemodel_package_id", pkgId)
			c.Set("wisemodel_pre_consumed_quota", 0)
			return nil
		}
		return fmt.Errorf("wisemodel package quota exhausted")
	}

	for _, pkgId := range ids {
		if pkgId == "" {
			continue
		}
		ok, err := model.TryDeductPackageRemain(pkgId, int64(estimatedQuota))
		if err != nil {
			return fmt.Errorf("%w: %v", ErrWisemodelServiceUnavailable, err)
		}
		if !ok {
			continue
		}
		c.Set("wisemodel_package_id", pkgId)
		c.Set("wisemodel_pre_consumed_quota", estimatedQuota)
		return nil
	}
	return fmt.Errorf("wisemodel package quota exhausted")
}

// wisemodelCandidates returns the FIFO candidate package ids set by Prepare,
// falling back to a single already-selected package id when Prepare was skipped.
func wisemodelCandidates(c *gin.Context) []string {
	if raw, ok := c.Get(wisemodelPkgCandidatesContextKey); ok {
		if ids, ok := raw.([]string); ok && len(ids) > 0 {
			return ids
		}
	}
	if pkgId := c.GetString("wisemodel_package_id"); pkgId != "" {
		return []string{pkgId}
	}
	return nil
}

// SettleWisemodelPkg 结算实际用量与预扣量的差额到选中包的 remain_quota。
// delta = 预扣 - 实扣:正值退还过估,负值补扣低估;预扣为 0(按次/免费)时按实际量扣减。
// actualQuota=0(上游失败)→ 全额退还预扣。结算幂等:重复调用不会重复增减。
func SettleWisemodelPkg(c *gin.Context, actualQuota int) {
	if !isWisemodelToken(c) {
		return
	}
	if c.GetBool("wisemodel_settled") {
		return
	}
	pkgId := c.GetString("wisemodel_package_id")
	if pkgId == "" {
		return
	}
	preConsumed := c.GetInt("wisemodel_pre_consumed_quota")
	delta := int64(preConsumed - actualQuota)
	c.Set("wisemodel_settled", true)
	if delta == 0 {
		return
	}
	if err := model.AdjustPackageRemain(pkgId, delta); err != nil {
		common.SysError(fmt.Sprintf("SettleWisemodelPkg: adjust remain error for pkg %s: %v", pkgId, err))
	}
}
