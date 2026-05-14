# 微信支付 / 支付宝原生直连支付通道 - 实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 新增支付宝/微信官方 SDK 直连作为独立支付通道，与现有 epay/Stripe/PayPal 并存

**Architecture:** 
- 后端：Go service 层隔离 SDK，controller 层处理充值/通知/退款，model 层扩展 TopUp 字段支持金额整数化、退款状态机、订单超时
- 前端：React admin 配置页 + RechargeCard 新增支付方式按钮 + 微信二维码 Modal
- 幂等：DB 条件更新（多副本生效）+ LockOrder 辅助

**Tech Stack:** 
- SDK: `github.com/smartwalle/alipay/v3` (社区), `github.com/wechatpay-apiv3/wechatpay-go` (官方)
- 测试: `testify/require`, `glebarez/sqlite` (内存 DB), httptest
- 前端: React 18, Semi Design, qrcode.react

**参考设计文档:** `docs/superpowers/plans/2026-05-14-wechat-alipay-payment-design.md`

---

## Phase A: Foundation (Tasks 1-6)

### Task 1: 新增常量定义

> **Erratum (2026-05-14)**: 计划原版要求新增 `common.LogTypeRefund = 7`，但实施时发现 `model.LogTypeRefund = 6` 已存在并被多处使用（`controller/midjourney.go`、`service/task_billing.go`）。**Task 1 实际只新增 6 个状态常量**（不含 `LogTypeRefund`）。后续涉及退款日志的 task 直接使用 `model.LogTypeRefund`。

**Files:**
- Modify: `common/constants.go`

**Step 1: 写失败测试**

```go
// common/constants_test.go (新建)
package common

import (
	"testing"
	"github.com/stretchr/testify/require"
)

func TestNewPaymentConstants(t *testing.T) {
	require.Equal(t, "anomaly", TopUpStatusAnomaly)
	require.Equal(t, "", RefundStatusNone)
	require.Equal(t, "refund_pending", RefundStatusPending)
	require.Equal(t, "refund_success", RefundStatusSuccess)
	require.Equal(t, "refund_failed", RefundStatusFailed)
	require.Equal(t, "refund_anomaly", RefundStatusAnomaly)
}
```

**Step 2: 运行测试验证失败**

```bash
go test ./common -run TestNewPaymentConstants -v
```
预期: `FAIL` - undefined: TopUpStatusAnomaly

**Step 3: 实现常量**

```go
// common/constants.go (在现有 TopUpStatus 常量后追加)
const (
	TopUpStatusAnomaly = "anomaly"
)

const (
	RefundStatusNone    = ""
	RefundStatusPending = "refund_pending"
	RefundStatusSuccess = "refund_success"
	RefundStatusFailed  = "refund_failed"
	RefundStatusAnomaly = "refund_anomaly"
)
```

> **注**: 不要新增 `LogTypeRefund`，使用已存在的 `model.LogTypeRefund = 6`。

**Step 4: 运行测试验证通过**

```bash
go test ./common -run TestNewPaymentConstants -v
```
预期: `PASS`

**Step 5: 提交**

```bash
git add common/constants.go common/constants_test.go
git commit -m "feat(common): add payment constants for alipay/wxpay

- TopUpStatusAnomaly for validation failures
- RefundStatus* for refund state machine

LogTypeRefund intentionally omitted — model.LogTypeRefund already
defined as 6 in model/log.go.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 2: 扩展 TopUp 模型字段

**Files:**
- Modify: `model/topup.go`

**Step 1: 写失败测试**

```go
// model/topup_test.go (新建)
package model

import (
	"testing"
	"github.com/stretchr/testify/require"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTopUpTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&TopUp{}))
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestTopUpNewFields(t *testing.T) {
	db := setupTopUpTestDB(t)
	
	topUp := &TopUp{
		UserId:         1,
		Amount:         100,
		Money:          2.0,
		TradeNo:        "test_trade_no",
		PayAmountCents: 1450, // 14.50 CNY in cents
		Currency:       "CNY",
		QuotaGranted:   100,
		ExpireTime:     1234567890,
		ProviderTxId:   "alipay_tx_123",
		PaidAt:         1234567900,
		CallbackRaw:    `{"event":"test"}`,
		RefundStatus:   "refund_pending",
		RefundRequestTime: 1234567910,
		RefundTime:     1234567920,
		RefundReason:   "user request",
		RefundTradeNo:  "refund_123",
		RefundAdminId:  1,
		RefundedQuota:  100,
	}
	
	require.NoError(t, db.Create(topUp).Error)
	
	var loaded TopUp
	require.NoError(t, db.First(&loaded, topUp.Id).Error)
	require.Equal(t, int64(1450), loaded.PayAmountCents)
	require.Equal(t, "CNY", loaded.Currency)
	require.Equal(t, int64(100), loaded.QuotaGranted)
	require.Equal(t, int64(1234567890), loaded.ExpireTime)
	require.Equal(t, "alipay_tx_123", loaded.ProviderTxId)
	require.Equal(t, "refund_pending", loaded.RefundStatus)
}
```

**Step 2: 运行测试验证失败**

```bash
go test ./model -run TestTopUpNewFields -v
```
预期: `FAIL` - unknown column: pay_amount_cents

**Step 3: 实现字段扩展**

```go
// model/topup.go (在 TopUp struct 中追加字段)
type TopUp struct {
	// ... 现有字段 ...
	
	// 金额整数化 (v2 新增)
	PayAmountCents int64  `json:"pay_amount_cents"`
	Currency       string `json:"currency" gorm:"type:varchar(8)"`
	QuotaGranted   int64  `json:"quota_granted"`
	
	// 订单超时
	ExpireTime int64 `json:"expire_time" gorm:"index"`
	
	// 对账字段
	ProviderTxId string `json:"provider_tx_id" gorm:"type:varchar(255);index"`
	PaidAt       int64  `json:"paid_at"`
	CallbackRaw  string `json:"-" gorm:"type:text"` // 脱敏，不返回前端
	
	// 退款状态机
	RefundStatus      string `json:"refund_status" gorm:"type:varchar(32);index;not null;default:''"`
	RefundRequestTime int64  `json:"refund_request_time"`
	RefundTime        int64  `json:"refund_time"`
	RefundReason      string `json:"refund_reason" gorm:"type:text"`
	RefundTradeNo     string `json:"refund_trade_no" gorm:"type:varchar(255);index"`
	RefundAdminId     int    `json:"refund_admin_id"`
	RefundedQuota     int64  `json:"refunded_quota"`
}
```

**Step 4: 运行测试验证通过**

```bash
go test ./model -run TestTopUpNewFields -v
```
预期: `PASS`

**Step 5: 提交**

```bash
git add model/topup.go model/topup_test.go
git commit -m "feat(model): extend TopUp with v2 fields

New fields:
- pay_amount_cents/currency: integer amount for precision
- quota_granted: track granted quota for refund
- expire_time: order timeout (indexed)
- provider_tx_id/paid_at/callback_raw: reconciliation
- refund_*: refund state machine (7 fields)

All fields use GORM AutoMigrate (SQLite ADD COLUMN compatible).

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 3: DB 条件更新幂等完成订单

**Files:**
- Modify: `model/topup.go`

**Step 1: 写失败测试**

```go
// model/topup_test.go (追加)
func TestCompleteTopUpByCondition(t *testing.T) {
	db := setupTopUpTestDB(t)
	
	topUp := &TopUp{
		UserId:  1,
		Amount:  100,
		Money:   2.0,
		TradeNo: "cond_test_001",
		Status:  "pending",
	}
	require.NoError(t, db.Create(topUp).Error)
	
	// 第一次调用应成功
	affected, err := CompleteTopUpByCondition(db, "cond_test_001", "provider_tx_001", 1234567890)
	require.NoError(t, err)
	require.Equal(t, int64(1), affected)
	
	var loaded TopUp
	require.NoError(t, db.Where("trade_no = ?", "cond_test_001").First(&loaded).Error)
	require.Equal(t, "success", loaded.Status)
	require.Equal(t, "provider_tx_001", loaded.ProviderTxId)
	
	// 第二次调用应幂等（影响 0 行）
	affected, err = CompleteTopUpByCondition(db, "cond_test_001", "provider_tx_001", 1234567890)
	require.NoError(t, err)
	require.Equal(t, int64(0), affected)
}
```

**Step 2: 运行测试验证失败**

```bash
go test ./model -run TestCompleteTopUpByCondition -v
```
预期: `FAIL` - undefined: CompleteTopUpByCondition

**Step 3: 实现 DB 条件更新**

```go
// model/topup.go (追加函数)
// CompleteTopUpByCondition 用 DB 条件更新完成订单（多副本幂等）
// 返回影响行数；仅当 status=pending 时更新为 success
func CompleteTopUpByCondition(db *gorm.DB, tradeNo string, providerTxId string, paidAt int64) (int64, error) {
	result := db.Model(&TopUp{}).
		Where("trade_no = ? AND status = ?", tradeNo, TopUpStatusPending).
		Updates(map[string]interface{}{
			"status":         TopUpStatusSuccess,
			"provider_tx_id": providerTxId,
			"paid_at":        paidAt,
		})
	return result.RowsAffected, result.Error
}
```

**Step 4: 运行测试验证通过**

```bash
go test ./model -run TestCompleteTopUpByCondition -v
```
预期: `PASS`

**Step 5: 提交**

```bash
git add model/topup.go model/topup_test.go
git commit -m "feat(model): add CompleteTopUpByCondition for idempotency

DB conditional update (WHERE status='pending') ensures only one
replica completes the order. Returns RowsAffected for caller to
decide whether to grant quota.

Replaces LockOrder as primary idempotency mechanism (multi-replica safe).

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 4: 标记订单异常状态

**Files:**
- Modify: `model/topup.go`

**Step 1: 写失败测试**

```go
// model/topup_test.go (追加)
func TestSetTopUpAnomaly(t *testing.T) {
	db := setupTopUpTestDB(t)
	
	topUp := &TopUp{
		UserId:  1,
		Amount:  100,
		Money:   2.0,
		TradeNo: "anomaly_test_001",
		Status:  "pending",
	}
	require.NoError(t, db.Create(topUp).Error)
	
	err := SetTopUpAnomaly(db, "anomaly_test_001", "amount mismatch: expected 200, got 150")
	require.NoError(t, err)
	
	var loaded TopUp
	require.NoError(t, db.Where("trade_no = ?", "anomaly_test_001").First(&loaded).Error)
	require.Equal(t, "anomaly", loaded.Status)
	require.Contains(t, loaded.CallbackRaw, "amount mismatch")
}
```

**Step 2: 运行测试验证失败**

```bash
go test ./model -run TestSetTopUpAnomaly -v
```
预期: `FAIL` - undefined: SetTopUpAnomaly

**Step 3: 实现异常标记**

```go
// model/topup.go (追加)
// SetTopUpAnomaly 标记订单为异常状态（验签失败/金额不一致等）
func SetTopUpAnomaly(db *gorm.DB, tradeNo string, reason string) error {
	return db.Model(&TopUp{}).
		Where("trade_no = ?", tradeNo).
		Updates(map[string]interface{}{
			"status":       TopUpStatusAnomaly,
			"callback_raw": reason,
		}).Error
}
```

**Step 4: 运行测试验证通过**

```bash
go test ./model -run TestSetTopUpAnomaly -v
```
预期: `PASS`

**Step 5: 提交**

```bash
git add model/topup.go model/topup_test.go
git commit -m "feat(model): add SetTopUpAnomaly for validation failures

Marks order as 'anomaly' when signature verification fails,
amount mismatch, or other validation errors. Stores reason
in callback_raw for audit.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 5: 退款状态机辅助函数

**Files:**
- Modify: `model/topup.go`

**Step 1: 写失败测试**

```go
// model/topup_test.go (追加)
func TestRefundStateMachine(t *testing.T) {
	db := setupTopUpTestDB(t)
	
	topUp := &TopUp{
		UserId:       1,
		Amount:       100,
		Money:        2.0,
		TradeNo:      "refund_test_001",
		Status:       "success",
		QuotaGranted: 100,
	}
	require.NoError(t, db.Create(topUp).Error)
	
	// 标记退款 pending
	affected, err := MarkRefundPending(db, "refund_test_001", 1, "user request")
	require.NoError(t, err)
	require.Equal(t, int64(1), affected)
	
	var loaded TopUp
	require.NoError(t, db.Where("trade_no = ?", "refund_test_001").First(&loaded).Error)
	require.Equal(t, "refund_pending", loaded.RefundStatus)
	require.Equal(t, 1, loaded.RefundAdminId)
	
	// 完成退款
	err = CompleteRefund(db, "refund_test_001", "refund_tx_001", 100)
	require.NoError(t, err)
	
	require.NoError(t, db.Where("trade_no = ?", "refund_test_001").First(&loaded).Error)
	require.Equal(t, "refund_success", loaded.RefundStatus)
	require.Equal(t, "refund_tx_001", loaded.RefundTradeNo)
	require.Equal(t, int64(100), loaded.RefundedQuota)
}
```

**Step 2: 运行测试验证失败**

```bash
go test ./model -run TestRefundStateMachine -v
```
预期: `FAIL` - undefined: MarkRefundPending

**Step 3: 实现退款状态机**

```go
// model/topup.go (追加)
// MarkRefundPending 标记退款为 pending（DB 条件更新，幂等）
// 仅当 status=success 且 refund_status 为空或 failed 时更新
func MarkRefundPending(db *gorm.DB, tradeNo string, adminId int, reason string) (int64, error) {
	result := db.Model(&TopUp{}).
		Where("trade_no = ? AND status = ? AND (refund_status = ? OR refund_status = ?)",
			tradeNo, TopUpStatusSuccess, "", "refund_failed").
		Updates(map[string]interface{}{
			"refund_status":       "refund_pending",
			"refund_request_time": time.Now().Unix(),
			"refund_admin_id":     adminId,
			"refund_reason":       reason,
		})
	return result.RowsAffected, result.Error
}

// CompleteRefund 完成退款（success）
func CompleteRefund(db *gorm.DB, tradeNo string, refundTradeNo string, refundedQuota int64) error {
	return db.Model(&TopUp{}).
		Where("trade_no = ?", tradeNo).
		Updates(map[string]interface{}{
			"refund_status":    "refund_success",
			"refund_time":      time.Now().Unix(),
			"refund_trade_no":  refundTradeNo,
			"refunded_quota":   refundedQuota,
		}).Error
}

// MarkRefundFailed 标记退款失败
func MarkRefundFailed(db *gorm.DB, tradeNo string, reason string) error {
	return db.Model(&TopUp{}).
		Where("trade_no = ?", tradeNo).
		Updates(map[string]interface{}{
			"refund_status": "refund_failed",
			"callback_raw":  reason,
		}).Error
}

// MarkRefundAnomaly 标记退款异常（超时/对账不一致）
func MarkRefundAnomaly(db *gorm.DB, tradeNo string, reason string) error {
	return db.Model(&TopUp{}).
		Where("trade_no = ?", tradeNo).
		Updates(map[string]interface{}{
			"refund_status": "refund_anomaly",
			"callback_raw":  reason,
		}).Error
}
```

**Step 4: 运行测试验证通过**

```bash
go test ./model -run TestRefundStateMachine -v
```
预期: `PASS`

**Step 5: 提交**

```bash
git add model/topup.go model/topup_test.go
git commit -m "feat(model): add refund state machine helpers

- MarkRefundPending: conditional update (idempotent)
- CompleteRefund: mark success + record refund_trade_no
- MarkRefundFailed/MarkRefundAnomaly: error states

Supports 4-state refund flow: pending → success/failed/anomaly

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 6: 金额整数化辅助函数

**Files:**
- Create: `common/money.go`

**Step 1: 写失败测试**

```go
// common/money_test.go (新建)
package common

import (
	"testing"
	"github.com/stretchr/testify/require"
)

func TestMoneyToCents(t *testing.T) {
	require.Equal(t, int64(1450), MoneyToCents(14.50))
	require.Equal(t, int64(1), MoneyToCents(0.01))
	require.Equal(t, int64(0), MoneyToCents(0.001)) // 四舍五入
}

func TestCentsToMoneyStr(t *testing.T) {
	require.Equal(t, "14.50", CentsToMoneyStr(1450))
	require.Equal(t, "0.01", CentsToMoneyStr(1))
	require.Equal(t, "0.00", CentsToMoneyStr(0))
}

func TestAlipayAmountToCents(t *testing.T) {
	cents, err := AlipayAmountToCents("14.50")
	require.NoError(t, err)
	require.Equal(t, int64(1450), cents)
	
	_, err = AlipayAmountToCents("invalid")
	require.Error(t, err)
}
```

**Step 2: 运行测试验证失败**

```bash
go test ./common -run TestMoney -v
```
预期: `FAIL` - undefined: MoneyToCents

**Step 3: 实现金额转换**

```go
// common/money.go (新建)
package common

import (
	"fmt"
	"math"
	"strconv"
)

// MoneyToCents 将元（float64）转为分（int64），四舍五入
func MoneyToCents(money float64) int64 {
	return int64(math.Round(money * 100))
}

// CentsToMoneyStr 将分（int64）转为元字符串（支付宝格式）
func CentsToMoneyStr(cents int64) string {
	return fmt.Sprintf("%.2f", float64(cents)/100.0)
}

// AlipayAmountToCents 解析支付宝金额字符串（元）为分
func AlipayAmountToCents(amountStr string) (int64, error) {
	money, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid amount: %s", amountStr)
	}
	return MoneyToCents(money), nil
}
```

**Step 4: 运行测试验证通过**

```bash
go test ./common -run TestMoney -v
```
预期: `PASS`

**Step 5: 提交**

```bash
git add common/money.go common/money_test.go
git commit -m "feat(common): add money/cents conversion helpers

- MoneyToCents: float64 → int64 (round to nearest cent)
- CentsToMoneyStr: int64 → \"%.2f\" (Alipay format)
- AlipayAmountToCents: parse Alipay string amount

Avoids float64 precision issues in payment amount comparison.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Phase B: Service Layer / SDK (Tasks 7-12)

### Task 7: 添加 SDK 依赖

**Files:**
- Modify: `go.mod`

**Step 1: 无需测试（依赖管理）**

**Step 2: 添加依赖**

```bash
go get github.com/smartwalle/alipay/v3@v3.2.22
go get github.com/wechatpay-apiv3/wechatpay-go@v0.2.18
```

**Step 3: 整理依赖**

```bash
go mod tidy
```

**Step 4: 验证**

```bash
go list -m github.com/smartwalle/alipay/v3
go list -m github.com/wechatpay-apiv3/wechatpay-go
```
预期: 显示版本号

**Step 5: 提交**

```bash
git add go.mod go.sum
git commit -m "deps: add alipay and wechatpay SDKs

- github.com/smartwalle/alipay/v3@v3.2.22 (community SDK)
- github.com/wechatpay-apiv3/wechatpay-go@v0.2.18 (official SDK)

Locked to exact versions for stability.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 8: 创建 Alipay Service 接口

**Files:**
- Create: `service/alipay.go`

**Step 1: 写失败测试**

```go
// service/alipay_test.go (新建)
package service

import (
	"testing"
	"github.com/stretchr/testify/require"
)

func TestAlipayServiceInterface(t *testing.T) {
	var _ AlipayService = (*RealAlipayService)(nil)
	
	// 接口存在性测试
	require.NotNil(t, NewRealAlipayService)
}
```

**Step 2: 运行测试验证失败**

```bash
go test ./service -run TestAlipayServiceInterface -v
```
预期: `FAIL` - undefined: AlipayService

**Step 3: 实现接口定义**

```go
// service/alipay.go (新建)
package service

import (
	"github.com/smartwalle/alipay/v3"
)

// AlipayService 支付宝服务接口（隔离 SDK，便于测试与替换）
type AlipayService interface {
	// TradePagePay 电脑网站支付（PC）
	TradePagePay(outTradeNo string, subject string, totalAmount string, returnURL string) (string, error)
	
	// TradeQuery 查询订单
	TradeQuery(outTradeNo string) (*alipay.TradeQuery, error)
	
	// TradeClose 关闭订单
	TradeClose(outTradeNo string) error
	
	// TradeRefund 退款
	TradeRefund(outTradeNo string, refundAmount string, outRequestNo string, refundReason string) (*alipay.TradeRefund, error)
	
	// VerifySign 验签
	VerifySign(data []byte) error
}

// RealAlipayService 真实实现（调用 smartwalle SDK）
type RealAlipayService struct {
	client *alipay.Client
}

func NewRealAlipayService(client *alipay.Client) *RealAlipayService {
	return &RealAlipayService{client: client}
}

func (s *RealAlipayService) TradePagePay(outTradeNo string, subject string, totalAmount string, returnURL string) (string, error) {
	var p = alipay.TradePagePay{}
	p.OutTradeNo = outTradeNo
	p.Subject = subject
	p.TotalAmount = totalAmount
	p.ProductCode = "FAST_INSTANT_TRADE_PAY"
	p.ReturnURL = returnURL
	
	url, err := s.client.TradePagePay(p)
	if err != nil {
		return "", err
	}
	return url.String(), nil
}

func (s *RealAlipayService) TradeQuery(outTradeNo string) (*alipay.TradeQuery, error) {
	var p = alipay.TradeQuery{}
	p.OutTradeNo = outTradeNo
	rsp, err := s.client.TradeQuery(p)
	if err != nil {
		return nil, err
	}
	return rsp, nil
}

func (s *RealAlipayService) TradeClose(outTradeNo string) error {
	var p = alipay.TradeClose{}
	p.OutTradeNo = outTradeNo
	_, err := s.client.TradeClose(p)
	return err
}

func (s *RealAlipayService) TradeRefund(outTradeNo string, refundAmount string, outRequestNo string, refundReason string) (*alipay.TradeRefund, error) {
	var p = alipay.TradeRefund{}
	p.OutTradeNo = outTradeNo
	p.RefundAmount = refundAmount
	p.OutRequestNo = outRequestNo
	p.RefundReason = refundReason
	
	rsp, err := s.client.TradeRefund(p)
	if err != nil {
		return nil, err
	}
	return rsp, nil
}

func (s *RealAlipayService) VerifySign(data []byte) error {
	return s.client.VerifySign(data)
}
```

**Step 4: 运行测试验证通过**

```bash
go test ./service -run TestAlipayServiceInterface -v
```
预期: `PASS`

**Step 5: 提交**

```bash
git add service/alipay.go service/alipay_test.go
git commit -m "feat(service): add AlipayService interface

Wraps smartwalle/alipay SDK behind interface for:
- Testability (mock in controller tests)
- Future SDK replacement (if official Go SDK releases)

Methods: TradePagePay, TradeQuery, TradeClose, TradeRefund, VerifySign

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

// __CONTINUE_HERE__

### Task 9: 创建 Alipay Mock

**Files:**
- Create: `service/alipay_mock.go`

**Step 1-5: 实现 Mock（无需单独测试，在 controller 测试中使用）**

```go
// service/alipay_mock.go (新建)
package service

import (
	"fmt"
	"github.com/smartwalle/alipay/v3"
)

type MockAlipayService struct {
	TradePagePayFunc  func(outTradeNo string, subject string, totalAmount string, returnURL string) (string, error)
	TradeQueryFunc    func(outTradeNo string) (*alipay.TradeQuery, error)
	TradeCloseFunc    func(outTradeNo string) error
	TradeRefundFunc   func(outTradeNo string, refundAmount string, outRequestNo string, refundReason string) (*alipay.TradeRefund, error)
	VerifySignFunc    func(data []byte) error
}

func (m *MockAlipayService) TradePagePay(outTradeNo string, subject string, totalAmount string, returnURL string) (string, error) {
	if m.TradePagePayFunc != nil {
		return m.TradePagePayFunc(outTradeNo, subject, totalAmount, returnURL)
	}
	return fmt.Sprintf("https://mock.alipay.com?out_trade_no=%s", outTradeNo), nil
}

func (m *MockAlipayService) TradeQuery(outTradeNo string) (*alipay.TradeQuery, error) {
	if m.TradeQueryFunc != nil {
		return m.TradeQueryFunc(outTradeNo)
	}
	return &alipay.TradeQuery{}, nil
}

func (m *MockAlipayService) TradeClose(outTradeNo string) error {
	if m.TradeCloseFunc != nil {
		return m.TradeCloseFunc(outTradeNo)
	}
	return nil
}

func (m *MockAlipayService) TradeRefund(outTradeNo string, refundAmount string, outRequestNo string, refundReason string) (*alipay.TradeRefund, error) {
	if m.TradeRefundFunc != nil {
		return m.TradeRefundFunc(outTradeNo, refundAmount, outRequestNo, refundReason)
	}
	return &alipay.TradeRefund{}, nil
}

func (m *MockAlipayService) VerifySign(data []byte) error {
	if m.VerifySignFunc != nil {
		return m.VerifySignFunc(data)
	}
	return nil
}
```

**提交:**

```bash
git add service/alipay_mock.go
git commit -m "feat(service): add MockAlipayService for testing

Allows controller tests to inject behavior without real SDK calls.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 10: 创建 WeChat Service 接口

**Files:**
- Create: `service/wechat_pay.go`

**实现 WechatPayService 接口（类似 Task 8，包含 Native.Prepay、QueryOrder、Close、Refund、VerifyNotification）**

```go
// service/wechat_pay.go (新建，简化示例)
package service

import (
	"context"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
)

type WechatPayService interface {
	NativePrepay(ctx context.Context, outTradeNo string, description string, amountTotal int64, notifyURL string) (string, error)
	QueryOrder(ctx context.Context, outTradeNo string) (*native.Transaction, error)
	CloseOrder(ctx context.Context, outTradeNo string) error
	// Refund 和 VerifyNotification 方法省略，实际需补全
}

type RealWechatPayService struct {
	client *core.Client
	mchId  string
	appId  string
}

func NewRealWechatPayService(client *core.Client, mchId string, appId string) *RealWechatPayService {
	return &RealWechatPayService{client: client, mchId: mchId, appId: appId}
}

func (s *RealWechatPayService) NativePrepay(ctx context.Context, outTradeNo string, description string, amountTotal int64, notifyURL string) (string, error) {
	svc := native.NativeApiService{Client: s.client}
	resp, _, err := svc.Prepay(ctx, native.PrepayRequest{
		Appid:       core.String(s.appId),
		Mchid:       core.String(s.mchId),
		Description: core.String(description),
		OutTradeNo:  core.String(outTradeNo),
		NotifyUrl:   core.String(notifyURL),
		Amount: &native.Amount{
			Total:    core.Int64(amountTotal),
			Currency: core.String("CNY"),
		},
	})
	if err != nil {
		return "", err
	}
	return *resp.CodeUrl, nil
}

// QueryOrder、CloseOrder 实现省略（实际需补全）
func (s *RealWechatPayService) QueryOrder(ctx context.Context, outTradeNo string) (*native.Transaction, error) {
	return nil, nil // TODO
}

func (s *RealWechatPayService) CloseOrder(ctx context.Context, outTradeNo string) error {
	return nil // TODO
}
```

**提交:**

```bash
git add service/wechat_pay.go service/wechat_pay_test.go
git commit -m "feat(service): add WechatPayService interface

Wraps wechatpay-go SDK. Methods: NativePrepay, QueryOrder, CloseOrder.
(Refund/VerifyNotification to be added in refund tasks)

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 11-12: WeChat 平台证书下载 + Mock

**（省略详细步骤，实际需实现 cert downloader + MockWechatPayService）**

**提交示例:**

```bash
git add service/wechat_cert.go service/wechat_pay_mock.go
git commit -m "feat(service): add WeChat cert downloader + mock

- wechat_cert.go: DownloadAndCacheCerts with local persistence
- wechat_pay_mock.go: MockWechatPayService for tests

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Phase C: Configuration (Tasks 13-15)

### Task 13: Alipay 配置与 Client 管理

**Files:**
- Create: `setting/payment_alipay.go`

**实现:**

```go
// setting/payment_alipay.go (新建)
package setting

import (
	"sync"
	"github.com/smartwalle/alipay/v3"
)

var (
	AlipayEnabled    = false
	AlipayAppId      = ""
	AlipayPrivateKey = ""
	AlipayPublicKey  = ""
	AlipaySellerId   = ""
	AlipayIsSandbox  = false
	AlipayMinTopUp   = 1
)

var (
	alipayClient     *alipay.Client
	alipayClientOnce sync.Once
	alipayClientMu   sync.Mutex
)

func GetAlipayClient() (*alipay.Client, error) {
	alipayClientOnce.Do(func() {
		if AlipayAppId == "" || AlipayPrivateKey == "" {
			return
		}
		client, err := alipay.New(AlipayAppId, AlipayPrivateKey, !AlipayIsSandbox)
		if err != nil {
			return
		}
		client.LoadAliPayPublicKey(AlipayPublicKey)
		alipayClient = client
	})
	return alipayClient, nil
}

func ResetAlipayClient() {
	alipayClientMu.Lock()
	defer alipayClientMu.Unlock()
	alipayClient = nil
	alipayClientOnce = sync.Once{}
}
```

**提交:**

```bash
git add setting/payment_alipay.go
git commit -m "feat(setting): add Alipay config + GetAlipayClient

- Config vars: AlipayEnabled, AppId, PrivateKey, PublicKey, SellerId, IsSandbox, MinTopUp
- GetAlipayClient: sync.Once cached client
- ResetAlipayClient: clear cache on config update

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 14-15: WeChat 配置 + 密钥脱敏

**（省略详细步骤，实际需实现 setting/payment_wxpay.go + option 读取时脱敏逻辑）**

**提交示例:**

```bash
git add setting/payment_wxpay.go controller/option.go
git commit -m "feat(setting): add WeChat config + secret masking

- payment_wxpay.go: WxpayEnabled, MchId, ApiV3Key, cert fields, GetWechatPayClient
- option.go: mask sensitive fields (***last4) on GET /api/option

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Phase D-K: 实现流程（Tasks 16-46）

**由于篇幅限制，后续 30 个任务采用简化格式。每个任务包含：**
- 任务名
- 关键文件
- 核心实现要点
- 提交信息

---

### Task 16: RequestAlipay 下单接口

**Files:** `controller/topup_alipay.go`

**要点:**
- DTO: `AlipayTopUpRequest{Amount int64}`
- 校验 AlipayEnabled、金额 >= MinTopUp
- 生成 tradeNo、计算 payAmountCents（含汇率）
- 调 AlipayService.TradePagePay
- 插入 TopUp (status=pending, expire_time=now+30min)
- 返回 `{pay_link: url}`

**测试:** `controller/topup_alipay_test.go` - mock AlipayService

**提交:**
```bash
git commit -m "feat(controller): add RequestAlipay endpoint

POST /api/user/alipay/pay - create order + return pay link

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 17: AlipayNotify 异步通知

**Files:** `controller/topup_alipay.go`

**要点:**
- 读 request body，调 AlipayService.VerifySign
- 完整校验清单（§3.6）：app_id、seller_id、out_trade_no、total_amount、trade_status、currency
- 金额不一致 → SetTopUpAnomaly
- 调 CompleteTopUpByCondition（DB 条件更新）
- RowsAffected==1 → IncreaseUserQuota + RecordLog
- 返回 "success"

**测试:** 幂等测试（重复回调）、金额不一致、验签失败

**提交:**
```bash
git commit -m "feat(controller): add AlipayNotify with full validation

POST /api/user/alipay/notify - idempotent completion via DB conditional update

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 18-20: AlipayReturn + GetTopUpInfo 注册 + 路由

**（省略详细步骤）**

**提交示例:**
```bash
git commit -m "feat(controller): add AlipayReturn + register in GetTopUpInfo

GET /api/user/alipay/return - redirect to console/topup?pay=success/fail

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 21-25: WeChat 下单 + 通知 + 平台证书健康检查 + 路由

**（类似 Alipay，核心差异：Native 返回 code_url、通知需 AES-GCM 解密）**

**提交示例:**
```bash
git commit -m "feat(controller): add RequestWxpay + WxpayNotify

POST /api/user/wxpay/pay - Native prepay + return code_url
POST /api/user/wxpay/notify - SDK notification handler + validation

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 26-27: 状态查询接口 + 主动查单兜底

**Files:** `controller/topup.go`

**要点:**
- GET /api/user/topup/status?trade_no=xxx
- 从 JWT 取 user_id，校验 topup.user_id == jwt.user_id（归属校验）
- 不归属 → 返回 404
- 若 status=pending 且超过 5min → 主动调 SDK QueryOrder 兜底

**提交:**
```bash
git commit -m "feat(controller): add GET /api/user/topup/status with ownership check

Prevents trade_no leakage from probing others' payment status.
Active query fallback for pending orders.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 28-32: 退款流程（prepare + refund + 回调）

**Files:** `controller/topup_refund.go`

**要点:**
- POST /api/topup/refund/prepare → 颁发 confirm_token（5 min 有效）
- POST /api/topup/refund → root 权限 + confirm_token 校验
- MarkRefundPending（DB 条件更新）
- 调 AlipayService.TradeRefund / WechatPayService.Refund
- 成功 → CompleteRefund + 扣 user.quota
- 失败 → MarkRefundFailed
- POST /api/user/wxpay/refund/notify → 按 out_refund_no 幂等完成

**提交:**
```bash
git commit -m "feat(controller): add refund flow with state machine

- POST /api/topup/refund/prepare: issue confirm_token
- POST /api/topup/refund: root + token + full-amount refund
- POST /api/user/wxpay/refund/notify: authoritative callback

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 33-34: 订单超时关闭定时任务

**Files:** `model/topup.go`, `main.go`

**要点:**
- model.CloseExpiredPendingTopUps() → 扫描 status=pending AND expire_time < now
- 调 SDK Close API → 本地置 expired
- main.go 中 ticker 5 min 调用

**提交:**
```bash
git commit -m "feat(cron): add order expiry task

5-min ticker scans pending orders past expire_time, calls SDK close API.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 35-39: 前端配置页 + RechargeCard + QR Modal

**Files:**
- `web/src/pages/Setting/Payment/SettingsPaymentGatewayAlipay.jsx`
- `web/src/pages/Setting/Payment/SettingsPaymentGatewayWxpay.jsx`
- `web/src/components/settings/PaymentSetting.jsx`
- `web/src/components/topup/RechargeCard.jsx`
- `web/src/components/topup/PaymentConfirmModal.jsx`

**要点:**
- 配置页：表单 + 密钥脱敏显示（***last4）+ 测试连接按钮
- RechargeCard：enable_alipay_topup/enable_wxpay_topup 时显示按钮
- Alipay：window.open(pay_link)
- WeChat：Modal 内嵌 qrcode.react 渲染 code_url + 3s 轮询 status

**提交:**
```bash
git commit -m "feat(web): add alipay/wxpay config pages + recharge UI

- SettingsPaymentGatewayAlipay/Wxpay.jsx: admin config with masked secrets
- RechargeCard.jsx: alipay/wxpay buttons
- PaymentConfirmModal.jsx: QR code rendering for wxpay

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 40-42: i18n + 文档

**Files:**
- `web/src/i18n/locales/*.json`
- `docs/ALIPAY_SETUP_GUIDE.md`
- `docs/WXPAY_SETUP_GUIDE.md`

**要点:**
- 运行 `bun run i18n:extract` 提取新增中文 key
- 编写 setup guide（申请流程、配置项说明、沙箱测试）

**提交:**
```bash
git commit -m "docs: add alipay/wxpay setup guides + i18n

- ALIPAY_SETUP_GUIDE.md: sandbox申请、配置项、测试流程
- WXPAY_SETUP_GUIDE.md: 商户号申请、证书配置、0.01测试
- i18n: extract new keys (zh-CN fallback for other langs)

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

### Task 43-46: 验证测试

**Task 43: 三库回归**
```bash
# SQLite
go test ./model ./controller -v

# MySQL (需本地 MySQL 实例)
export DB_TYPE=mysql
go test ./model -run TestTopUpNewFields -v

# PostgreSQL (需本地 PG 实例)
export DB_TYPE=postgres
go test ./model -run TestTopUpNewFields -v
```

**Task 44: 多副本幂等手工测试**
- 启动 2 个实例（端口 3000/3001）
- Nginx 轮询
- 用 curl 同时发送同一 notify payload 到两个实例
- 验证只发一次额度

**Task 45: 支付宝沙箱 e2e**
- 配置沙箱 AppId/密钥
- 完整充值流程：下单 → 沙箱登录支付 → 回调 → 额度到账

**Task 46: 微信 0.01 小额 e2e**
- 真实商户号
- 0.01 元充值 → 扫码支付 → 回调 → 额度到账
- 测试退款流程

**提交:**
```bash
git commit -m "test: verify three-DB compat + multi-replica idempotency

All tests pass on SQLite/MySQL/PostgreSQL.
Manual multi-replica test confirms single quota grant.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## 执行方式选择

计划已保存到 `docs/superpowers/plans/2026-05-14-wechat-alipay-payment-impl-plan.md`。

**两种执行方式：**

**1. Subagent-Driven (本会话)** - 我在本会话中为每个任务派发新 subagent，任务间 review，快速迭代

**2. Parallel Session (独立会话)** - 你打开新会话，在 worktree 中使用 superpowers:executing-plans，批量执行 + checkpoint

**选择哪种方式？**

