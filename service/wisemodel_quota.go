package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const wisemodelPkgRemainKeyPrefix = "wm:pkg:remain:"
const wisemodelPkgCandidatesContextKey = "wisemodel_package_candidates"

func buildWisemodelPackageCandidates(packages []*model.WisemodelPackage, attribution map[string]int64, requiredQuota int64) []string {
	candidates := make([]string, 0, len(packages))
	for _, pkg := range packages {
		if requiredQuota > 0 && pkg.QuotaGranted-attribution[pkg.PackageId] < requiredQuota {
			continue
		}
		candidates = append(candidates, pkg.PackageId)
	}
	return candidates
}

func initWisemodelPkgRemainKey(ctx context.Context, pkgID string) error {
	key := wisemodelPkgRemainKeyPrefix + pkgID
	if _, err := common.RDB.Get(ctx, key).Result(); errors.Is(err, redis.Nil) {
		remain, initErr := model.ComputeWisemodelPackageRemain(pkgID)
		if initErr != nil {
			common.SysError(fmt.Sprintf("PreConsumeWisemodelPkg: init failed for pkg %s: %v", pkgID, initErr))
			return initErr
		}
		common.RDB.SetNX(ctx, key, remain, 0)
	}
	return nil
}

func PrepareWisemodelPackageForPreConsume(c *gin.Context, requestedModel string, estimatedQuota int) error {
	tokenName := c.GetString("token_name")
	tokenKey := c.GetString("token_key")
	if tokenName != "wisemodel-token" && !strings.HasPrefix(tokenKey, "wisemodel-") {
		return nil
	}

	userID := c.GetInt("id")
	if userID == 0 {
		return nil
	}

	packages, err := model.GetActiveWisemodelPackages(userID)
	if err != nil {
		return err
	}
	if len(packages) == 0 {
		c.Set("wisemodel_package_id", "")
		return fmt.Errorf("wisemodel package quota exhausted")
	}

	attribution, err := model.CalculatePackageAttribution(userID, packages)
	if err != nil {
		return err
	}

	eligiblePackages := packages
	if requestedModel != "" {
		eligiblePackages = model.FilterPackagesByModel(packages, requestedModel)
		if len(eligiblePackages) == 0 {
			c.Set(wisemodelPkgCandidatesContextKey, []string{})
			c.Set("wisemodel_package_id", "")
			return fmt.Errorf("wisemodel package does not support model %s", requestedModel)
		}
	}

	candidates := buildWisemodelPackageCandidates(eligiblePackages, attribution, int64(estimatedQuota))
	c.Set(wisemodelPkgCandidatesContextKey, candidates)
	if len(candidates) == 0 {
		c.Set("wisemodel_package_id", "")
		return fmt.Errorf("wisemodel package quota exhausted")
	}

	c.Set("wisemodel_package_id", candidates[0])
	return nil
}

// PreConsumeWisemodelPkg 在上游调用前原子预扣资源包配额。
// 仅在 context 有 wisemodel_package_id 时生效；Redis 故障 fail-open。
// 成功时将预扣量写入 context key "wisemodel_pre_consumed_quota"。
func PreConsumeWisemodelPkg(c *gin.Context, estimatedQuota int) error {
	pkgId := c.GetString("wisemodel_package_id")
	if pkgId == "" || estimatedQuota <= 0 {
		return nil
	}
	if !common.RedisEnabled {
		return nil
	}

	ctx := context.Background()
	candidates := []string{pkgId}
	if rawCandidates, ok := c.Get(wisemodelPkgCandidatesContextKey); ok {
		if ids, ok := rawCandidates.([]string); ok && len(ids) > 0 {
			candidates = ids
		}
	}

	for _, candidateID := range candidates {
		if candidateID == "" {
			continue
		}

		if err := initWisemodelPkgRemainKey(ctx, candidateID); err != nil {
			return nil // Redis/DB 初始化异常 → fail-open
		}

		key := wisemodelPkgRemainKeyPrefix + candidateID
		newVal, err := common.RDB.DecrBy(ctx, key, int64(estimatedQuota)).Result()
		if err != nil {
			common.SysError(fmt.Sprintf("PreConsumeWisemodelPkg: Redis DecrBy error for pkg %s: %v", candidateID, err))
			return nil // Redis 故障 → fail-open
		}
		if newVal < 0 {
			common.RDB.IncrBy(ctx, key, int64(estimatedQuota)) // 回滚预扣
			continue
		}

		c.Set("wisemodel_package_id", candidateID)
		c.Set("wisemodel_pre_consumed_quota", estimatedQuota)
		return nil
	}

	return fmt.Errorf("wisemodel package quota exhausted")
}

// SettleWisemodelPkg 结算实际用量与预扣量的差额。
// actualQuota=0 表示上游失败，全额退还预扣。
func SettleWisemodelPkg(c *gin.Context, actualQuota int) {
	pkgId := c.GetString("wisemodel_package_id")
	preConsumed := c.GetInt("wisemodel_pre_consumed_quota")
	if pkgId == "" || preConsumed == 0 {
		return
	}
	if !common.RedisEnabled {
		return
	}

	delta := int64(preConsumed - actualQuota) // 正=释放过估，负=补扣低估
	if delta == 0 {
		return
	}
	ctx := context.Background()
	if err := common.RDB.IncrBy(ctx, wisemodelPkgRemainKeyPrefix+pkgId, delta).Err(); err != nil {
		common.SysError(fmt.Sprintf("SettleWisemodelPkg: Redis IncrBy error for pkg %s: %v", pkgId, err))
	}
}
