package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const wisemodelPkgRemainKeyPrefix = "wm:pkg:remain:"

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

	key := wisemodelPkgRemainKeyPrefix + pkgId
	ctx := context.Background()

	// 懒加载初始化：SETNX 竞争安全，首个 goroutine 胜出写入
	if _, err := common.RDB.Get(ctx, key).Result(); errors.Is(err, redis.Nil) {
		remain, initErr := model.ComputeWisemodelPackageRemain(pkgId)
		if initErr != nil {
			common.SysError(fmt.Sprintf("PreConsumeWisemodelPkg: init failed for pkg %s: %v", pkgId, initErr))
			return nil // fail-open
		}
		common.RDB.SetNX(ctx, key, remain, 0)
	}

	newVal, err := common.RDB.DecrBy(ctx, key, int64(estimatedQuota)).Result()
	if err != nil {
		common.SysError(fmt.Sprintf("PreConsumeWisemodelPkg: Redis DecrBy error for pkg %s: %v", pkgId, err))
		return nil // Redis 故障 → fail-open
	}
	if newVal < 0 {
		common.RDB.IncrBy(ctx, key, int64(estimatedQuota)) // 回滚预扣
		return fmt.Errorf("wisemodel package quota exhausted")
	}
	c.Set("wisemodel_pre_consumed_quota", estimatedQuota)
	return nil
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
