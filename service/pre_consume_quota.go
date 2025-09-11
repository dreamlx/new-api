package service

import (
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/logger"
	"one-api/model"
	relaycommon "one-api/relay/common"
	"one-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

func ReturnPreConsumedQuota(c *gin.Context, relayInfo *relaycommon.RelayInfo, preConsumedQuota int) {
	if preConsumedQuota != 0 {
		logger.LogInfo(c, fmt.Sprintf("用户 %d 请求失败, 返还预扣费额度 %s", relayInfo.UserId, logger.FormatQuota(preConsumedQuota)))
		gopool.Go(func() {
			relayInfoCopy := *relayInfo

			err := PostConsumeQuota(&relayInfoCopy, -preConsumedQuota, 0, false)
			if err != nil {
				common.SysLog("error return pre-consumed quota: " + err.Error())
			}
		})
	}
}

// PreConsumeQuota checks if the user has enough quota to pre-consume.
// It returns the pre-consumed quota if successful, or an error if not.
func PreConsumeQuota(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) (int, *types.NewAPIError) {
	userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
	if err != nil {
		return 0, types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
	}

	// ========== Token额度同步逻辑开始 ==========
	// 检查是否需要同步Token额度与用户额度
	if !relayInfo.TokenUnlimited {
		tokenQuota := c.GetInt("token_quota")
		if tokenQuota < userQuota {
			// 发现Token额度落后于用户额度，执行同步
			err := model.UpdateTokenQuota(relayInfo.TokenId, userQuota)
			if err != nil {
				// 记录日志，但不中断主流程，避免影响正常请求
				logger.LogError(c, fmt.Sprintf("同步Token额度失败: %v", err))
			} else {
				// 同步成功后，更新当前请求上下文中的额度，确保本次请求可用
				c.Set("token_quota", userQuota)
				logger.LogInfo(c, fmt.Sprintf("Token %d 额度已从 %d 同步至 %d", relayInfo.TokenId, tokenQuota, userQuota))
			}
		}
	}
	// ========== Token额度同步逻辑结束 ==========

	if userQuota <= 0 {
		return 0, types.NewErrorWithStatusCode(fmt.Errorf("用户额度不足, 剩余额度: %s", logger.FormatQuota(userQuota)), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	if userQuota-preConsumedQuota < 0 {
		return 0, types.NewErrorWithStatusCode(fmt.Errorf("预扣费额度失败, 用户剩余额度: %s, 需要预扣费额度: %s", logger.FormatQuota(userQuota), logger.FormatQuota(preConsumedQuota)), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}

	trustQuota := common.GetTrustQuota()

	relayInfo.UserQuota = userQuota
	if userQuota > trustQuota {
		// 用户额度充足，判断令牌额度是否充足
		if !relayInfo.TokenUnlimited {
			// 非无限令牌，判断令牌额度是否充足
			tokenQuota := c.GetInt("token_quota")
			if tokenQuota > trustQuota {
				// 令牌额度充足，信任令牌
				preConsumedQuota = 0
				logger.LogInfo(c, fmt.Sprintf("用户 %d 剩余额度 %s 且令牌 %d 额度 %d 充足, 信任且不需要预扣费", relayInfo.UserId, logger.FormatQuota(userQuota), relayInfo.TokenId, tokenQuota))
			}
		} else {
			// in this case, we do not pre-consume quota
			// because the user has enough quota
			preConsumedQuota = 0
			logger.LogInfo(c, fmt.Sprintf("用户 %d 额度充足且为无限额度令牌, 信任且不需要预扣费", relayInfo.UserId))
		}
	}

	if preConsumedQuota > 0 {
		err := PreConsumeTokenQuota(relayInfo, preConsumedQuota)
		if err != nil {
			return 0, types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		err = model.DecreaseUserQuota(relayInfo.UserId, preConsumedQuota)
		if err != nil {
			return 0, types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
		logger.LogInfo(c, fmt.Sprintf("用户 %d 预扣费 %s, 预扣费后剩余额度: %s", relayInfo.UserId, logger.FormatQuota(preConsumedQuota), logger.FormatQuota(userQuota-preConsumedQuota)))
	}
	relayInfo.FinalPreConsumedQuota = preConsumedQuota
	return preConsumedQuota, nil
}
