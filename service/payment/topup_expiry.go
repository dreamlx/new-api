package payment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var expiryAlipayProvider = func() (AlipayService, error) {
	return NewAlipayServiceFromSettings()
}

var expiryWechatProvider = func() (WechatPayService, error) {
	return NewWechatPayServiceFromSettings()
}

// SetExpiryProvidersForTest temporarily overrides the provider factories used
// by the expiry sweep. It returns a restore function that must be deferred by
// the caller.
func SetExpiryProvidersForTest(alipay func() (AlipayService, error), wechat func() (WechatPayService, error)) func() {
	prevAlipay := expiryAlipayProvider
	prevWechat := expiryWechatProvider
	if alipay != nil {
		expiryAlipayProvider = alipay
	}
	if wechat != nil {
		expiryWechatProvider = wechat
	}
	return func() {
		expiryAlipayProvider = prevAlipay
		expiryWechatProvider = prevWechat
	}
}

func CloseExpiredPendingTopUps(ctx context.Context) (closedOk, closeFailed int, err error) {
	db := model.DB
	if db == nil {
		return 0, 0, errors.New("model.DB is not initialised")
	}

	now := time.Now().Unix()
	var rows []model.TopUp
	err = db.Where("status = ? AND expire_time > 0 AND expire_time < ?",
		common.TopUpStatusPending, now).
		Find(&rows).Error
	if err != nil {
		return 0, 0, fmt.Errorf("scan expired pending topups: %w", err)
	}

	for _, row := range rows {
		if ctx.Err() != nil {
			return closedOk, closeFailed, ctx.Err()
		}

		sdkErr := callProviderClose(ctx, row.PaymentMethod, row.TradeNo)
		if sdkErr != nil {
			closeFailed++
			common.SysError(fmt.Sprintf("expiry sweep: provider close failed tradeNo=%s method=%s err=%v",
				row.TradeNo, row.PaymentMethod, sdkErr))
		}

		result := db.Model(&model.TopUp{}).
			Where("trade_no = ? AND status = ?", row.TradeNo, common.TopUpStatusPending).
			Update("status", common.TopUpStatusExpired)
		if result.Error != nil {
			common.SysError(fmt.Sprintf("expiry sweep: local close UPDATE failed tradeNo=%s err=%v",
				row.TradeNo, result.Error))
			closeFailed++
			continue
		}
		if result.RowsAffected == 0 {
			common.SysLog(fmt.Sprintf("expiry sweep: tradeNo=%s already moved, skipping", row.TradeNo))
			continue
		}
		if sdkErr == nil {
			closedOk++
			common.SysLog(fmt.Sprintf("expiry sweep: closed tradeNo=%s method=%s", row.TradeNo, row.PaymentMethod))
		}
	}

	return closedOk, closeFailed, nil
}

func callProviderClose(ctx context.Context, paymentMethod string, tradeNo string) error {
	switch paymentMethod {
	case "alipay":
		svc, err := expiryAlipayProvider()
		if err != nil {
			return err
		}
		_, err = svc.TradeClose(ctx, tradeNo)
		return err
	case "wxpay":
		svc, err := expiryWechatProvider()
		if err != nil {
			return err
		}
		return svc.CloseOrder(ctx, tradeNo)
	default:
		return nil
	}
}
