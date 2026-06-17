package middleware

import (
	"bytes"
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

func setupWmMiddlewareDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.WisemodelPackage{}))
	orig := model.DB
	origRedis := common.RedisEnabled
	model.DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = orig
		common.RedisEnabled = origRedis
	})
}

func seedActivePkg(t *testing.T, userId int, pkgId, models string) {
	t.Helper()
	vu := time.Now().Add(24 * time.Hour)
	require.NoError(t, model.DB.Create(&model.WisemodelPackage{
		UserId: userId, PackageId: pkgId, OrderId: "o",
		QuotaGranted: 1000, RemainQuota: 1000, AvailableModels: models,
		Amount: 1, ValidUntil: &vu,
	}).Error)
}

func runWmCheck(userId int, reqModel string) (status int, reached, pkgIdSet bool) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("id", userId)
		c.Set("token_name", "wisemodel-token")
	})
	r.Use(WisemodelPackageCheck())
	r.POST("/", func(c *gin.Context) {
		reached = true
		pkgIdSet = c.GetString("wisemodel_package_id") != ""
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"model":"`+reqModel+`"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w.Code, reached, pkgIdSet
}

// Middleware is now a pure existence pre-check: it must allow the request through
// WITHOUT selecting a package or reading any quota figure (that is the relay
// pre-consume hook's single responsibility).
func TestWmCheck_AllowsWithoutSettingPackageId(t *testing.T) {
	setupWmMiddlewareDB(t)
	seedActivePkg(t, 1, "pkg-1", "minimax-m2,")

	status, reached, pkgIdSet := runWmCheck(1, "minimax-m2")
	require.Equal(t, http.StatusOK, status)
	require.True(t, reached)
	require.False(t, pkgIdSet, "middleware must not select/set wisemodel_package_id")
}

func TestWmCheck_BlocksNoActivePackage(t *testing.T) {
	setupWmMiddlewareDB(t)
	status, reached, _ := runWmCheck(2, "minimax-m2")
	require.Equal(t, http.StatusForbidden, status)
	require.False(t, reached)
}

func TestWmCheck_BlocksUnsupportedModel(t *testing.T) {
	setupWmMiddlewareDB(t)
	seedActivePkg(t, 3, "pkg-3", "minimax-m2.5-highspeed,")

	status, reached, _ := runWmCheck(3, "minimax-m2")
	require.Equal(t, http.StatusForbidden, status)
	require.False(t, reached)
}
