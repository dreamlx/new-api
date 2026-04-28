package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupWisemodelQuotaTest wires up an in-memory Redis (miniredis) and an in-memory
// SQLite DB, then restores originals on cleanup. Returns the miniredis handle so
// individual tests can seed keys or check state directly.
func setupWisemodelQuotaTest(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err)

	origRDB := common.RDB
	origRedisEnabled := common.RedisEnabled

	common.RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	common.RedisEnabled = true

	// Set up in-memory SQLite for model.DB / model.LOG_DB so ComputeWisemodelPackageRemain works
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.WisemodelPackage{}, &model.Log{}))

	origDB := model.DB
	origLogDB := model.LOG_DB
	model.DB = db
	model.LOG_DB = db

	t.Cleanup(func() {
		common.RDB = origRDB
		common.RedisEnabled = origRedisEnabled
		model.DB = origDB
		model.LOG_DB = origLogDB
		mr.Close()
	})
	return mr
}

// newQuotaTestCtx returns a minimal gin.Context with wisemodel_package_id set.
func newQuotaTestCtx(pkgId string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	if pkgId != "" {
		c.Set("wisemodel_package_id", pkgId)
	}
	return c
}

// seedPackage inserts a WisemodelPackage record so ComputeWisemodelPackageRemain can find it.
func seedPackage(t *testing.T, pkgId string, quotaGranted int64) {
	t.Helper()
	now := time.Now()
	expire := now.Add(365 * 24 * time.Hour)
	pkg := &model.WisemodelPackage{
		PackageId:      pkgId,
		UserId:         1,
		OrderId:        "order-test",
		QuotaGranted:   quotaGranted,
		OriginalPoints: 1000,
		Amount:         1.0,
		ValidUntil:     &expire,
		CreatedAt:      now,
	}
	require.NoError(t, model.DB.Create(pkg).Error)
}

// --- Risk 3 & 4: Pre-consume lazy-init ---

func TestPreConsumeWisemodelPkg_KeyAbsent_LazyInit(t *testing.T) {
	mr := setupWisemodelQuotaTest(t)
	pkgId := "pkg-lazy-init-test"
	quotaGranted := int64(500_000)
	seedPackage(t, pkgId, quotaGranted)

	c := newQuotaTestCtx(pkgId)
	err := PreConsumeWisemodelPkg(c, 100)
	require.NoError(t, err)

	// Redis key should now exist with value = quotaGranted - 100
	val, redisErr := mr.Get("wm:pkg:remain:" + pkgId)
	require.NoError(t, redisErr)
	require.Equal(t, "499900", val)

	// Context should record pre-consumed amount
	preConsumed := c.GetInt("wisemodel_pre_consumed_quota")
	require.Equal(t, 100, preConsumed)
}

func TestPreConsumeWisemodelPkg_KeyPresent_Decremented(t *testing.T) {
	mr := setupWisemodelQuotaTest(t)
	pkgId := "pkg-present-test"

	// Seed Redis key directly (simulates a previously initialized key)
	require.NoError(t, mr.Set("wm:pkg:remain:"+pkgId, "1000"))

	c := newQuotaTestCtx(pkgId)
	err := PreConsumeWisemodelPkg(c, 300)
	require.NoError(t, err)

	val, _ := mr.Get("wm:pkg:remain:" + pkgId)
	require.Equal(t, "700", val)
	require.Equal(t, 300, c.GetInt("wisemodel_pre_consumed_quota"))
}

func TestPreConsumeWisemodelPkg_ExhaustedRollback(t *testing.T) {
	mr := setupWisemodelQuotaTest(t)
	pkgId := "pkg-exhausted-test"

	require.NoError(t, mr.Set("wm:pkg:remain:"+pkgId, "5"))

	c := newQuotaTestCtx(pkgId)
	err := PreConsumeWisemodelPkg(c, 100)
	require.Error(t, err)
	require.Contains(t, err.Error(), "wisemodel package quota exhausted")

	// Key must be rolled back to original value
	val, _ := mr.Get("wm:pkg:remain:" + pkgId)
	require.Equal(t, "5", val)

	// Context must NOT have pre-consumed set
	require.Equal(t, 0, c.GetInt("wisemodel_pre_consumed_quota"))
}

func TestPreConsumeWisemodelPkg_RedisDisabled_FailOpen(t *testing.T) {
	_ = setupWisemodelQuotaTest(t)
	common.RedisEnabled = false // override after setup

	c := newQuotaTestCtx("pkg-redis-disabled")
	err := PreConsumeWisemodelPkg(c, 1000)
	require.NoError(t, err) // fail-open: no error even without Redis
}

func TestPreConsumeWisemodelPkg_EmptyPkgId_NoOp(t *testing.T) {
	mr := setupWisemodelQuotaTest(t)

	c := newQuotaTestCtx("") // no wisemodel_package_id in context
	err := PreConsumeWisemodelPkg(c, 500)
	require.NoError(t, err)

	// miniredis should have no keys
	keys := mr.Keys()
	require.Empty(t, keys)
}

// --- Risk 6: Settle delta ---

func TestSettleWisemodelPkg_Overage_Refunds(t *testing.T) {
	mr := setupWisemodelQuotaTest(t)
	pkgId := "pkg-settle-overage"

	// Pre-consumed 100, Redis was decremented: key = 900
	require.NoError(t, mr.Set("wm:pkg:remain:"+pkgId, "900"))

	c := newQuotaTestCtx(pkgId)
	c.Set("wisemodel_pre_consumed_quota", 100)

	SettleWisemodelPkg(c, 60) // actual = 60, pre = 100 → refund 40

	val, _ := mr.Get("wm:pkg:remain:" + pkgId)
	require.Equal(t, "940", val)
}

func TestSettleWisemodelPkg_Underage_Charges(t *testing.T) {
	mr := setupWisemodelQuotaTest(t)
	pkgId := "pkg-settle-underage"

	require.NoError(t, mr.Set("wm:pkg:remain:"+pkgId, "940"))

	c := newQuotaTestCtx(pkgId)
	c.Set("wisemodel_pre_consumed_quota", 60)

	SettleWisemodelPkg(c, 100) // actual = 100, pre = 60 → charge 40 more

	val, _ := mr.Get("wm:pkg:remain:" + pkgId)
	require.Equal(t, "900", val)
}

func TestSettleWisemodelPkg_UpstreamFailure_FullRefund(t *testing.T) {
	mr := setupWisemodelQuotaTest(t)
	pkgId := "pkg-settle-upstream-fail"

	require.NoError(t, mr.Set("wm:pkg:remain:"+pkgId, "800"))

	c := newQuotaTestCtx(pkgId)
	c.Set("wisemodel_pre_consumed_quota", 200)

	SettleWisemodelPkg(c, 0) // actual = 0 (upstream failed) → full refund of 200

	val, _ := mr.Get("wm:pkg:remain:" + pkgId)
	require.Equal(t, "1000", val)
}

func TestSettleWisemodelPkg_NoPkgId_NoOp(t *testing.T) {
	mr := setupWisemodelQuotaTest(t)

	c := newQuotaTestCtx("") // no pkg id
	c.Set("wisemodel_pre_consumed_quota", 100)

	SettleWisemodelPkg(c, 50)

	require.Empty(t, mr.Keys())
}

func TestSettleWisemodelPkg_ZeroDelta_NoRedisCall(t *testing.T) {
	mr := setupWisemodelQuotaTest(t)
	pkgId := "pkg-settle-zero-delta"

	require.NoError(t, mr.Set("wm:pkg:remain:"+pkgId, "500"))

	c := newQuotaTestCtx(pkgId)
	c.Set("wisemodel_pre_consumed_quota", 100)

	SettleWisemodelPkg(c, 100) // delta = 0, no change

	val, _ := mr.Get("wm:pkg:remain:" + pkgId)
	require.Equal(t, "500", val) // unchanged
}

func TestPreConsumeWisemodelPkg_ZeroEstimatedQuota_NoOp(t *testing.T) {
	mr := setupWisemodelQuotaTest(t)
	pkgId := "pkg-zero-quota"

	require.NoError(t, mr.Set("wm:pkg:remain:"+pkgId, "500"))

	c := newQuotaTestCtx(pkgId)
	err := PreConsumeWisemodelPkg(c, 0)
	require.NoError(t, err)

	val, _ := mr.Get("wm:pkg:remain:" + pkgId)
	require.Equal(t, "500", val) // unchanged
}

func TestPreConsumeWisemodelPkg_LazyInit_DBLookup(t *testing.T) {
	mr := setupWisemodelQuotaTest(t)
	pkgId := "pkg-lazy-db-lookup"
	quotaGranted := int64(10_000)
	seedPackage(t, pkgId, quotaGranted)

	// Insert a consume log attributed to this package (simulates prior consumption)
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId:             1,
		Type:               model.LogTypeConsume,
		Quota:              3000,
		WisemodelPackageId: pkgId,
		CreatedAt:          time.Now().Unix(),
	}).Error)

	// Redis key absent — lazy init should compute: 10_000 - 3_000 = 7_000
	c := newQuotaTestCtx(pkgId)
	err := PreConsumeWisemodelPkg(c, 100)
	require.NoError(t, err)

	// After deducting 100: 7_000 - 100 = 6_900
	ctx := context.Background()
	redisVal, redisErr := common.RDB.Get(ctx, "wm:pkg:remain:"+pkgId).Int64()
	require.NoError(t, redisErr)
	_ = mr
	require.Equal(t, int64(6900), redisVal)
}
