package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupWisemodelLedgerTest points model.DB at an in-memory SQLite DB (single
// connection so the shared table survives concurrency) and disables Redis.
func setupWisemodelLedgerTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.WisemodelPackage{}, &model.Log{}, &model.User{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	origDB := model.DB
	origLogDB := model.LOG_DB
	origRedis := common.RedisEnabled
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = origDB
		model.LOG_DB = origLogDB
		common.RedisEnabled = origRedis
	})
}

func newWmCtx(userID int) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("id", userID)
	c.Set("token_name", "wisemodel-token")
	return c
}

func seedPkg(t *testing.T, userID int, pkgId, models string, remain int64, validUntil, createdAt time.Time) {
	t.Helper()
	vu := validUntil
	require.NoError(t, model.DB.Create(&model.WisemodelPackage{
		UserId: userID, PackageId: pkgId, OrderId: "o",
		QuotaGranted: remain, RemainQuota: remain,
		AvailableModels: models, Amount: 1,
		ValidUntil: &vu, CreatedAt: createdAt,
	}).Error)
}

func pkgRemain(t *testing.T, pkgId string) int64 {
	t.Helper()
	var got model.WisemodelPackage
	require.NoError(t, model.DB.Where("package_id = ?", pkgId).First(&got).Error)
	return got.RemainQuota
}

// Earliest-expiring package is deducted first (FIFO).
func TestWmGate_DeductsEarliestExpiringPackage(t *testing.T) {
	setupWisemodelLedgerTest(t)
	now := time.Now()
	seedPkg(t, 42, "pkg-early", "minimax-m2,", 1000, now.Add(24*time.Hour), now)
	seedPkg(t, 42, "pkg-late", "minimax-m2,", 1000, now.Add(72*time.Hour), now.Add(time.Hour))

	c := newWmCtx(42)
	require.NoError(t, PrepareWisemodelPackageForPreConsume(c, "minimax-m2", 300))
	require.NoError(t, PreConsumeWisemodelPkg(c, 300))

	require.Equal(t, "pkg-early", c.GetString("wisemodel_package_id"))
	require.Equal(t, 300, c.GetInt("wisemodel_pre_consumed_quota"))
	require.Equal(t, int64(700), pkgRemain(t, "pkg-early"))
	require.Equal(t, int64(1000), pkgRemain(t, "pkg-late"))
}

// When the earliest package can't cover the request, billing rolls over to the
// fresh sibling (production scenario for user 164).
func TestWmGate_RollsOverWhenEarliestExhausted(t *testing.T) {
	setupWisemodelLedgerTest(t)
	now := time.Now()
	seedPkg(t, 164, "pkg-258", "glm-5.1-count,", 5, now.Add(24*time.Hour), now)
	seedPkg(t, 164, "pkg-264", "glm-5.1-count,", 1400, now.Add(72*time.Hour), now.Add(time.Hour))

	c := newWmCtx(164)
	require.NoError(t, PrepareWisemodelPackageForPreConsume(c, "glm-5.1-count", 100))
	require.NoError(t, PreConsumeWisemodelPkg(c, 100))

	require.Equal(t, "pkg-264", c.GetString("wisemodel_package_id"))
	require.Equal(t, int64(5), pkgRemain(t, "pkg-258"))    // untouched
	require.Equal(t, int64(1300), pkgRemain(t, "pkg-264")) // deducted 100
}

// All eligible packages insufficient → exhausted.
func TestWmGate_ExhaustedWhenAllInsufficient(t *testing.T) {
	setupWisemodelLedgerTest(t)
	now := time.Now()
	seedPkg(t, 50, "pkg-x", "minimax-m2,", 5, now.Add(24*time.Hour), now)

	c := newWmCtx(50)
	require.NoError(t, PrepareWisemodelPackageForPreConsume(c, "minimax-m2", 100))
	err := PreConsumeWisemodelPkg(c, 100)
	require.Error(t, err)
	require.Contains(t, err.Error(), "wisemodel package quota exhausted")
}

// Requested model not covered by any active package → distinct error.
func TestWmGate_UnsupportedModelRejected(t *testing.T) {
	setupWisemodelLedgerTest(t)
	now := time.Now()
	seedPkg(t, 51, "pkg-mm", "minimax-m2.5-highspeed,", 1000, now.Add(24*time.Hour), now)

	c := newWmCtx(51)
	err := PrepareWisemodelPackageForPreConsume(c, "minimax-m2", 100)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not support model")
}

// No active package at all → exhausted.
func TestWmGate_NoActivePackage(t *testing.T) {
	setupWisemodelLedgerTest(t)
	c := newWmCtx(52)
	err := PrepareWisemodelPackageForPreConsume(c, "minimax-m2", 100)
	require.Error(t, err)
	require.Contains(t, err.Error(), "wisemodel package quota exhausted")
}

// Zero estimate (per-call model): no pre-deduct, but settlement charges actual usage.
func TestWmGate_ZeroEstimateChargesAtSettle(t *testing.T) {
	setupWisemodelLedgerTest(t)
	now := time.Now()
	seedPkg(t, 53, "pkg-count", "glm-5.1-count,", 1400, now.Add(24*time.Hour), now)

	c := newWmCtx(53)
	require.NoError(t, PrepareWisemodelPackageForPreConsume(c, "glm-5.1-count", 0))
	require.NoError(t, PreConsumeWisemodelPkg(c, 0))
	require.Equal(t, "pkg-count", c.GetString("wisemodel_package_id"))
	require.Equal(t, int64(1400), pkgRemain(t, "pkg-count")) // not pre-deducted

	SettleWisemodelPkg(c, 7) // charge actual 7
	require.Equal(t, int64(1393), pkgRemain(t, "pkg-count"))
}

// Settlement releases over-estimate back to the package.
func TestWmSettle_ReleasesOverestimate(t *testing.T) {
	setupWisemodelLedgerTest(t)
	now := time.Now()
	seedPkg(t, 54, "pkg-s", "minimax-m2,", 1000, now.Add(24*time.Hour), now)

	c := newWmCtx(54)
	require.NoError(t, PrepareWisemodelPackageForPreConsume(c, "minimax-m2", 100))
	require.NoError(t, PreConsumeWisemodelPkg(c, 100)) // remain 900
	SettleWisemodelPkg(c, 60)                          // release 40 → 940
	require.Equal(t, int64(940), pkgRemain(t, "pkg-s"))
}

// Upstream failure (actual 0) → full refund of pre-consumed.
func TestWmSettle_UpstreamFailFullRefund(t *testing.T) {
	setupWisemodelLedgerTest(t)
	now := time.Now()
	seedPkg(t, 55, "pkg-f", "minimax-m2,", 1000, now.Add(24*time.Hour), now)

	c := newWmCtx(55)
	require.NoError(t, PrepareWisemodelPackageForPreConsume(c, "minimax-m2", 200))
	require.NoError(t, PreConsumeWisemodelPkg(c, 200)) // remain 800
	SettleWisemodelPkg(c, 0)                           // full refund → 1000
	require.Equal(t, int64(1000), pkgRemain(t, "pkg-f"))
}

// Settlement is idempotent: a second call must not double-charge/refund.
func TestWmSettle_Idempotent(t *testing.T) {
	setupWisemodelLedgerTest(t)
	now := time.Now()
	seedPkg(t, 57, "pkg-i", "minimax-m2,", 1000, now.Add(24*time.Hour), now)

	c := newWmCtx(57)
	require.NoError(t, PrepareWisemodelPackageForPreConsume(c, "minimax-m2", 100))
	require.NoError(t, PreConsumeWisemodelPkg(c, 100)) // remain 900
	SettleWisemodelPkg(c, 60)                          // → 940
	SettleWisemodelPkg(c, 60)                          // must be a no-op
	require.Equal(t, int64(940), pkgRemain(t, "pkg-i"))
}

// Expired packages must be skipped entirely; only the active package is charged.
func TestWmGate_SkipsExpiredPackage(t *testing.T) {
	setupWisemodelLedgerTest(t)
	now := time.Now()
	seedPkg(t, 60, "pkg-expired", "minimax-m2,", 1000, now.Add(-time.Hour), now.Add(-48*time.Hour))
	seedPkg(t, 60, "pkg-active", "minimax-m2,", 1000, now.Add(24*time.Hour), now)

	c := newWmCtx(60)
	require.NoError(t, PrepareWisemodelPackageForPreConsume(c, "minimax-m2", 300))
	require.NoError(t, PreConsumeWisemodelPkg(c, 300))

	require.Equal(t, "pkg-active", c.GetString("wisemodel_package_id"))
	require.Equal(t, int64(1000), pkgRemain(t, "pkg-expired")) // untouched
	require.Equal(t, int64(700), pkgRemain(t, "pkg-active"))
}

// A transient DB error must be reported as a retryable service-unavailable
// condition, NOT as a permanent "quota exhausted" 403.
func TestWmGate_DBError_IsRetryableNotExhausted(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// Intentionally do NOT migrate WisemodelPackage → any query errors.
	origDB := model.DB
	origRedis := common.RedisEnabled
	model.DB = db
	common.RedisEnabled = false
	t.Cleanup(func() { model.DB = origDB; common.RedisEnabled = origRedis })

	c := newWmCtx(99)
	err = PrepareWisemodelPackageForPreConsume(c, "minimax-m2", 100)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrWisemodelServiceUnavailable),
		"DB error must wrap ErrWisemodelServiceUnavailable so relay maps it to 503, not 403")
	require.NotContains(t, err.Error(), "quota exhausted")
}

// Non-wisemodel token bypasses the gate entirely.
func TestWmGate_NonWisemodelTokenNoOp(t *testing.T) {
	setupWisemodelLedgerTest(t)
	c := newWmCtx(56)
	c.Set("token_name", "regular-token")
	require.NoError(t, PrepareWisemodelPackageForPreConsume(c, "gpt-4", 100))
	require.NoError(t, PreConsumeWisemodelPkg(c, 100))
}
