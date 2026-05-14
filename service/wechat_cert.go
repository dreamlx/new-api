package service

import (
	"context"
	"fmt"

	"github.com/wechatpay-apiv3/wechatpay-go/core/downloader"
)

// CheckWechatCertHealth reports whether the WeChat Pay SDK's auto-downloader
// has at least one valid platform certificate cached for the given merchant.
// It returns the serial of the newest cached cert, or an error if no certs
// are available.
//
// Why this exists: the SDK option WithWechatPayAutoAuthCipher silently
// registers a background goroutine that periodically downloads platform
// certs. If that goroutine has never run (client not yet built), has been
// rate-limited by WeChat, or had its goroutine stopped, signature
// verification of notify callbacks will silently start failing. Operators
// (and the later monitoring cron) need a synchronous probe that exposes the
// state of that downloader without waiting for a failed callback.
//
// We rely on downloader.MgrInstance(), the process-wide singleton the SDK
// itself uses; querying it is read-only and safe to call concurrently with
// the downloader's own writes (the SDK guards its internal map with a mutex).
func CheckWechatCertHealth(ctx context.Context, mchId string) (string, error) {
	if mchId == "" {
		return "", fmt.Errorf("wechat cert health: empty mchId")
	}

	mgr := downloader.MgrInstance()
	if mgr == nil {
		return "", fmt.Errorf("wechat cert health: downloader manager not initialised (client never built) mchId=%s", mchId)
	}

	if !mgr.HasDownloader(ctx, mchId) {
		return "", fmt.Errorf("wechat cert health: no downloader registered for mchId=%s (auto-cipher option not applied)", mchId)
	}

	serial := mgr.GetNewestCertificateSerial(ctx, mchId)
	if serial == "" {
		return "", fmt.Errorf("wechat cert health: no platform certificates cached for mchId=%s (downloader has not completed initial fetch)", mchId)
	}
	return serial, nil
}
