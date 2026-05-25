package model

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupLogTestDB creates an in-memory SQLite DB with Log table for testing.
func setupLogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}, &User{}))

	// Point both DB and LOG_DB to in-memory DB
	origDB := DB
	origLogDB := LOG_DB
	origLogConsume := common.LogConsumeEnabled
	origRedis := common.RedisEnabled
	DB = db
	LOG_DB = db
	common.LogConsumeEnabled = true
	common.RedisEnabled = false

	t.Cleanup(func() {
		DB = origDB
		LOG_DB = origLogDB
		common.LogConsumeEnabled = origLogConsume
		common.RedisEnabled = origRedis
	})
	return db
}

func newTestGinContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c
}

// --- Red Phase Tests ---

// Test 1: RecordConsumeLog auto-fills WisemodelPackageId from gin.Context
func TestRecordConsumeLogAutoFillsWisemodelPackageId(t *testing.T) {
	db := setupLogTestDB(t)
	c := newTestGinContext()

	// Simulate middleware setting the wisemodel package ID in context
	c.Set("wisemodel_package_id", "pkg_test_123")
	c.Set("username", "testuser")

	// Call RecordConsumeLog WITHOUT passing WisemodelPackageId in params
	RecordConsumeLog(c, 1, RecordConsumeLogParams{
		ChannelId:    1,
		ModelName:    "deepseek-chat",
		TokenName:    "wisemodel-token",
		Quota:        1000,
		Content:      "test",
		TokenId:      0, // avoid async callback
		IsStream:     false,
		Group:        "default",
	})

	// Verify: DB should have WisemodelPackageId auto-filled from context
	var log Log
	err := db.First(&log).Error
	require.NoError(t, err)
	require.Equal(t, "pkg_test_123", log.WisemodelPackageId)
}

// Test 2: RecordConsumeLog leaves WisemodelPackageId empty when context has no value
func TestRecordConsumeLogNoWisemodelContext(t *testing.T) {
	db := setupLogTestDB(t)
	c := newTestGinContext()

	// No wisemodel_package_id in context (normal non-wisemodel request)
	c.Set("username", "normaluser")

	RecordConsumeLog(c, 2, RecordConsumeLogParams{
		ChannelId: 1,
		ModelName: "gpt-4",
		TokenName: "regular-token",
		Quota:     500,
		Content:   "test",
		TokenId:   0,
		IsStream:  false,
		Group:     "default",
	})

	var log Log
	err := db.First(&log).Error
	require.NoError(t, err)
	require.Equal(t, "", log.WisemodelPackageId)
}

// Test 3: RecordConsumeLogParams no longer has WisemodelPackageId field.
// Context is the single source of truth — callers never pass it explicitly.
func TestRecordConsumeLogContextIsOnlySource(t *testing.T) {
	db := setupLogTestDB(t)
	c := newTestGinContext()

	c.Set("wisemodel_package_id", "pkg_from_context")
	c.Set("username", "testuser")

	RecordConsumeLog(c, 1, RecordConsumeLogParams{
		ChannelId: 1,
		ModelName: "deepseek-chat",
		TokenName: "wisemodel-token",
		Quota:     1000,
		Content:   "test",
		TokenId:   0,
		IsStream:  false,
		Group:     "default",
	})

	var log Log
	err := db.First(&log).Error
	require.NoError(t, err)
	require.Equal(t, "pkg_from_context", log.WisemodelPackageId)
}

// Test 4: Existing CalculatePackageAttribution query still works
// (regression test ensuring the WisemodelPackageId column in logs table
// continues to be queryable after our refactoring)
func TestLogWisemodelPackageIdQueryable(t *testing.T) {
	db := setupLogTestDB(t)

	// Insert logs with different WisemodelPackageId values
	logs := []Log{
		{UserId: 1, Type: LogTypeConsume, Quota: 1000, WisemodelPackageId: "pkg_a", CreatedAt: 1000},
		{UserId: 1, Type: LogTypeConsume, Quota: 2000, WisemodelPackageId: "pkg_a", CreatedAt: 1001},
		{UserId: 1, Type: LogTypeConsume, Quota: 500, WisemodelPackageId: "pkg_b", CreatedAt: 1002},
		{UserId: 1, Type: LogTypeConsume, Quota: 300, WisemodelPackageId: "", CreatedAt: 1003},
	}
	for _, l := range logs {
		require.NoError(t, db.Create(&l).Error)
	}

	// Query: SUM(quota) WHERE wisemodel_package_id = 'pkg_a'
	var sum int64
	err := db.Model(&Log{}).
		Where("wisemodel_package_id = ? AND type = ?", "pkg_a", LogTypeConsume).
		Select("COALESCE(SUM(quota), 0)").Scan(&sum).Error
	require.NoError(t, err)
	require.Equal(t, int64(3000), sum)

	// Query: empty wisemodel_package_id
	var emptyCount int64
	err = db.Model(&Log{}).
		Where("(wisemodel_package_id IS NULL OR wisemodel_package_id = '') AND type = ?", LogTypeConsume).
		Count(&emptyCount).Error
	require.NoError(t, err)
	require.Equal(t, int64(1), emptyCount)
}
