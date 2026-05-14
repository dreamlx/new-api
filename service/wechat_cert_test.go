package service

import (
	"context"
	"strings"
	"testing"
)

// TestCheckWechatCertHealthRejectsEmptyMchId guards against the trivial misuse
// of asking about the empty merchant id, which the SDK silently treats as
// "any merchant" and could mask real configuration errors.
func TestCheckWechatCertHealthRejectsEmptyMchId(t *testing.T) {
	serial, err := CheckWechatCertHealth(context.Background(), "")
	if err == nil {
		t.Fatalf("expected error for empty mchId, got serial=%q nil err", serial)
	}
	if serial != "" {
		t.Fatalf("expected empty serial on error, got %q", serial)
	}
}

// TestCheckWechatCertHealthUnregisteredMerchant exercises the negative path:
// a merchant id that was never registered with the SDK downloader must surface
// an error. This is the operator-facing signal that the auto-downloader has
// not run / has stalled.
//
// Note: this is an integration-style negative test. Standing up a real
// downloader requires network access to api.mch.weixin.qq.com plus a valid
// merchant key, so we only verify the no-cert path here.
func TestCheckWechatCertHealthUnregisteredMerchant(t *testing.T) {
	const fakeMchId = "0000000000-not-registered-test-only"

	serial, err := CheckWechatCertHealth(context.Background(), fakeMchId)
	if err == nil {
		t.Fatalf("expected error for unregistered mchId, got serial=%q nil err", serial)
	}
	if serial != "" {
		t.Fatalf("expected empty serial on error, got %q", serial)
	}
	if !strings.Contains(err.Error(), fakeMchId) {
		t.Logf("error message did not mention mchId=%q (got %v); not a hard requirement but helps operators triage", fakeMchId, err)
	}
}
