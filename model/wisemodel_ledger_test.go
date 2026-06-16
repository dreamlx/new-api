package model

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ReclaimExpiredPackages must refund exactly the package's RemainQuota (not a
// whole-window log estimate), zero out remain, and stamp reclaimed_at.
func TestReclaimExpiredPackages_RefundsRemainQuota(t *testing.T) {
	setupPackageTestDB(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "u1", Quota: 1000}).Error)
	past := time.Now().Add(-time.Hour)
	require.NoError(t, DB.Create(&WisemodelPackage{
		UserId: 1, PackageId: "pkg-exp", OrderId: "o",
		QuotaGranted: 1000, RemainQuota: 300, Amount: 1,
		ValidUntil: &past, CreatedAt: time.Now().Add(-48 * time.Hour),
	}).Error)

	require.NoError(t, ReclaimExpiredPackages(1))

	var u User
	require.NoError(t, DB.Where("id = ?", 1).First(&u).Error)
	require.Equal(t, 700, u.Quota) // 1000 - remain(300)

	var got WisemodelPackage
	require.NoError(t, DB.Where("package_id = ?", "pkg-exp").First(&got).Error)
	require.Equal(t, int64(0), got.RemainQuota)
	require.NotNil(t, got.ReclaimedAt)
}

// Reclaim must be idempotent: a second pass must not double-refund.
func TestReclaimExpiredPackages_Idempotent(t *testing.T) {
	setupPackageTestDB(t)
	require.NoError(t, DB.Create(&User{Id: 2, Username: "u2", Quota: 1000}).Error)
	past := time.Now().Add(-time.Hour)
	require.NoError(t, DB.Create(&WisemodelPackage{
		UserId: 2, PackageId: "pkg-exp2", OrderId: "o",
		QuotaGranted: 1000, RemainQuota: 300, Amount: 1,
		ValidUntil: &past, CreatedAt: time.Now().Add(-48 * time.Hour),
	}).Error)

	require.NoError(t, ReclaimExpiredPackages(2))
	require.NoError(t, ReclaimExpiredPackages(2)) // no-op

	var u User
	require.NoError(t, DB.Where("id = ?", 2).First(&u).Error)
	require.Equal(t, 700, u.Quota) // still 700, not double-charged
}

// setupPackageTestDB creates an in-memory SQLite DB with the wisemodel package
// and user tables, pointing the global DB at it for the duration of the test.
func setupPackageTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&WisemodelPackage{}, &User{}, &Log{}, &Option{}))

	// :memory: gives each connection its own DB; pin to one connection so the
	// concurrency test exercises a single shared table.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	origDB := DB
	origLogDB := LOG_DB
	origRedis := common.RedisEnabled
	DB = db
	LOG_DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		DB = origDB
		LOG_DB = origLogDB
		common.RedisEnabled = origRedis
	})
	return db
}

// Backfill initializes remain_quota from historical consumption, once.
func TestBackfillWisemodelRemainQuota_InitializesFromHistory(t *testing.T) {
	setupPackageTestDB(t)
	vu := time.Now().Add(24 * time.Hour)
	require.NoError(t, DB.Create(&WisemodelPackage{
		UserId: 7, PackageId: "pkg-bf", OrderId: "o",
		QuotaGranted: 1000, RemainQuota: 0, // post-AddColumn default
		Amount: 1, ValidUntil: &vu, CreatedAt: time.Now().Add(-time.Hour),
	}).Error)
	require.NoError(t, LOG_DB.Create(&Log{
		UserId: 7, Type: LogTypeConsume, Quota: 300,
		WisemodelPackageId: "pkg-bf", CreatedAt: time.Now().Unix(),
	}).Error)

	require.NoError(t, BackfillWisemodelRemainQuota())

	var got WisemodelPackage
	require.NoError(t, DB.Where("package_id = ?", "pkg-bf").First(&got).Error)
	require.Equal(t, int64(700), got.RemainQuota) // 1000 - 300
}

// Backfill must be idempotent: a second run must not recompute/overwrite.
func TestBackfillWisemodelRemainQuota_Idempotent(t *testing.T) {
	setupPackageTestDB(t)
	vu := time.Now().Add(24 * time.Hour)
	require.NoError(t, DB.Create(&WisemodelPackage{
		UserId: 8, PackageId: "pkg-bf2", OrderId: "o",
		QuotaGranted: 1000, RemainQuota: 0,
		Amount: 1, ValidUntil: &vu, CreatedAt: time.Now().Add(-time.Hour),
	}).Error)
	require.NoError(t, LOG_DB.Create(&Log{
		UserId: 8, Type: LogTypeConsume, Quota: 300,
		WisemodelPackageId: "pkg-bf2", CreatedAt: time.Now().Unix(),
	}).Error)

	require.NoError(t, BackfillWisemodelRemainQuota()) // remain → 700

	// New consumption after backfill must not retrigger a recompute.
	require.NoError(t, LOG_DB.Create(&Log{
		UserId: 8, Type: LogTypeConsume, Quota: 500,
		WisemodelPackageId: "pkg-bf2", CreatedAt: time.Now().Unix(),
	}).Error)
	require.NoError(t, BackfillWisemodelRemainQuota())

	var got WisemodelPackage
	require.NoError(t, DB.Where("package_id = ?", "pkg-bf2").First(&got).Error)
	require.Equal(t, int64(700), got.RemainQuota) // unchanged
}

// TryDeductPackageRemain must atomically subtract when remain is sufficient.
func TestTryDeductPackageRemain_SufficientDeducts(t *testing.T) {
	setupPackageTestDB(t)
	require.NoError(t, DB.Create(&WisemodelPackage{
		UserId: 1, PackageId: "pkg_a", OrderId: "o1",
		QuotaGranted: 100, RemainQuota: 100, Amount: 1,
	}).Error)

	ok, err := TryDeductPackageRemain("pkg_a", 30)
	require.NoError(t, err)
	require.True(t, ok)

	var got WisemodelPackage
	require.NoError(t, DB.Where("package_id = ?", "pkg_a").First(&got).Error)
	require.Equal(t, int64(70), got.RemainQuota)
}

// TryDeductPackageRemain must reject (and leave remain untouched) when insufficient.
func TestTryDeductPackageRemain_InsufficientRejects(t *testing.T) {
	setupPackageTestDB(t)
	require.NoError(t, DB.Create(&WisemodelPackage{
		UserId: 1, PackageId: "pkg_b", OrderId: "o1",
		QuotaGranted: 100, RemainQuota: 100, Amount: 1,
	}).Error)

	ok, err := TryDeductPackageRemain("pkg_b", 150)
	require.NoError(t, err)
	require.False(t, ok)

	var got WisemodelPackage
	require.NoError(t, DB.Where("package_id = ?", "pkg_b").First(&got).Error)
	require.Equal(t, int64(100), got.RemainQuota)
}

// Deduct must reject an expired package even if remain is sufficient (TOCTOU guard).
func TestTryDeductPackageRemain_RejectsExpired(t *testing.T) {
	setupPackageTestDB(t)
	past := time.Now().Add(-time.Hour)
	require.NoError(t, DB.Create(&WisemodelPackage{
		UserId: 1, PackageId: "pkg-exp-deduct", OrderId: "o",
		QuotaGranted: 100, RemainQuota: 100, Amount: 1, ValidUntil: &past,
	}).Error)

	ok, err := TryDeductPackageRemain("pkg-exp-deduct", 30)
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, int64(100), pkgRemainModel(t, "pkg-exp-deduct"))
}

// Deduct must reject a reclaimed package even if (stale) remain looks sufficient.
func TestTryDeductPackageRemain_RejectsReclaimed(t *testing.T) {
	setupPackageTestDB(t)
	future := time.Now().Add(time.Hour)
	reclaimed := time.Now()
	require.NoError(t, DB.Create(&WisemodelPackage{
		UserId: 1, PackageId: "pkg-reclaimed-deduct", OrderId: "o",
		QuotaGranted: 100, RemainQuota: 100, Amount: 1,
		ValidUntil: &future, ReclaimedAt: &reclaimed,
	}).Error)

	ok, err := TryDeductPackageRemain("pkg-reclaimed-deduct", 30)
	require.NoError(t, err)
	require.False(t, ok)
}

// Settlement must NOT resurrect remain on an already-reclaimed package.
func TestAdjustPackageRemain_SkipsReclaimed(t *testing.T) {
	setupPackageTestDB(t)
	future := time.Now().Add(time.Hour)
	reclaimed := time.Now()
	require.NoError(t, DB.Create(&WisemodelPackage{
		UserId: 1, PackageId: "pkg-reclaimed-adj", OrderId: "o",
		QuotaGranted: 100, RemainQuota: 0, Amount: 1,
		ValidUntil: &future, ReclaimedAt: &reclaimed,
	}).Error)

	require.NoError(t, AdjustPackageRemain("pkg-reclaimed-adj", 80))
	require.Equal(t, int64(0), pkgRemainModel(t, "pkg-reclaimed-adj")) // not resurrected
}

func pkgRemainModel(t *testing.T, pkgId string) int64 {
	t.Helper()
	var got WisemodelPackage
	require.NoError(t, DB.Where("package_id = ?", pkgId).First(&got).Error)
	return got.RemainQuota
}

// AdjustPackageRemain releases over-estimate (positive delta) back to the package.
func TestAdjustPackageRemain_ReleaseOverestimate(t *testing.T) {
	setupPackageTestDB(t)
	require.NoError(t, DB.Create(&WisemodelPackage{
		UserId: 1, PackageId: "pkg_d", OrderId: "o1",
		QuotaGranted: 100, RemainQuota: 70, Amount: 1,
	}).Error)

	// pre-consumed 30, actual 10 → release 20
	require.NoError(t, AdjustPackageRemain("pkg_d", 20))

	var got WisemodelPackage
	require.NoError(t, DB.Where("package_id = ?", "pkg_d").First(&got).Error)
	require.Equal(t, int64(90), got.RemainQuota)
}

// AdjustPackageRemain back-charges under-estimate (negative delta).
func TestAdjustPackageRemain_BackfillUnderestimate(t *testing.T) {
	setupPackageTestDB(t)
	require.NoError(t, DB.Create(&WisemodelPackage{
		UserId: 1, PackageId: "pkg_e", OrderId: "o1",
		QuotaGranted: 100, RemainQuota: 70, Amount: 1,
	}).Error)

	// pre-consumed 30, actual 35 → back-charge 5 (delta = -5)
	require.NoError(t, AdjustPackageRemain("pkg_e", -5))

	var got WisemodelPackage
	require.NoError(t, DB.Where("package_id = ?", "pkg_e").First(&got).Error)
	require.Equal(t, int64(65), got.RemainQuota)
}

// Concurrent deductions must never oversell: total deducted == granted, remain >= 0.
func TestTryDeductPackageRemain_ConcurrentNoOversell(t *testing.T) {
	setupPackageTestDB(t)
	require.NoError(t, DB.Create(&WisemodelPackage{
		UserId: 1, PackageId: "pkg_c", OrderId: "o1",
		QuotaGranted: 100, RemainQuota: 100, Amount: 1,
	}).Error)

	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	// 200 goroutines each try to deduct 1; only 100 may succeed.
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := TryDeductPackageRemain("pkg_c", 1)
			if err == nil && ok {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	require.Equal(t, 100, successes)
	var got WisemodelPackage
	require.NoError(t, DB.Where("package_id = ?", "pkg_c").First(&got).Error)
	require.Equal(t, int64(0), got.RemainQuota)
}
