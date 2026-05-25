package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupAuthTestDB mirrors the controller test setup but only includes the
// tables PlatformAuth touches: platforms (lookup), users (shadow user lazy
// provision). Keep this minimal — middleware tests should not depend on
// channels / tokens / logs.
func setupAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Platform{}))
	common.RedisEnabled = false
	model.InitColumnsForTest()
	return db
}

// makeAuthTestRouter mounts PlatformAuth on a single test endpoint that
// returns the ctx values the middleware injected.
func makeAuthTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/probe", PlatformAuth(), func(c *gin.Context) {
		p := PlatformFromContext(c)
		sid := ShadowUserIdFromContext(c)
		c.JSON(http.StatusOK, gin.H{
			"platform_id":    p.PlatformId,
			"shadow_user_id": sid,
		})
	})
	return r
}

func seedPlatform(t *testing.T, platformId, sk string, status int) *model.Platform {
	t.Helper()
	shadow, err := model.GetOrCreatePlatformShadowUser(platformId)
	require.NoError(t, err)
	p := &model.Platform{
		PlatformId: platformId, Name: "T", ShadowUserId: shadow.Id,
		PlatformSkHash: model.HashPlatformSk(sk), Status: status,
	}
	require.NoError(t, p.Insert())
	return p
}

func TestPlatformAuth_MissingHeaders_401(t *testing.T) {
	model.DB = setupAuthTestDB(t)
	r := makeAuthTestRouter(t)

	req := httptest.NewRequest("GET", "/probe", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}

func TestPlatformAuth_MissingSkOnly_401(t *testing.T) {
	model.DB = setupAuthTestDB(t)
	seedPlatform(t, "p_only_id", "pk_correct", common.UserStatusEnabled)
	r := makeAuthTestRouter(t)

	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("X-Platform-Id", "p_only_id")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}

func TestPlatformAuth_UnknownPlatform_401(t *testing.T) {
	model.DB = setupAuthTestDB(t)
	r := makeAuthTestRouter(t)

	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("X-Platform-Id", "ghost")
	req.Header.Set("X-Platform-Sk", "pk_anything")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}

func TestPlatformAuth_WrongSk_401(t *testing.T) {
	model.DB = setupAuthTestDB(t)
	seedPlatform(t, "p_wrong_sk", "pk_correct", common.UserStatusEnabled)
	r := makeAuthTestRouter(t)

	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("X-Platform-Id", "p_wrong_sk")
	req.Header.Set("X-Platform-Sk", "pk_wrong")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}

func TestPlatformAuth_DisabledStatus_403(t *testing.T) {
	model.DB = setupAuthTestDB(t)
	seedPlatform(t, "p_disabled", "pk_correct", common.UserStatusDisabled)
	r := makeAuthTestRouter(t)

	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("X-Platform-Id", "p_disabled")
	req.Header.Set("X-Platform-Sk", "pk_correct")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 403, w.Code)
}

func TestPlatformAuth_Success_InjectsCtx(t *testing.T) {
	model.DB = setupAuthTestDB(t)
	p := seedPlatform(t, "p_ok", "pk_correct_xyz", common.UserStatusEnabled)
	r := makeAuthTestRouter(t)

	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("X-Platform-Id", "p_ok")
	req.Header.Set("X-Platform-Sk", "pk_correct_xyz")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "\"platform_id\":\"p_ok\"")
	// Shadow user id should match the row seedPlatform created.
	assert.Contains(t, w.Body.String(),
		"\"shadow_user_id\":"+itoa(p.ShadowUserId))
}

func TestPlatformAuth_LazyShadowUserCreation(t *testing.T) {
	// Insert a Platform row with ShadowUserId=0 so middleware must lazy-create.
	model.DB = setupAuthTestDB(t)
	hash := model.HashPlatformSk("pk_lazy")
	require.NoError(t, model.DB.Create(&model.Platform{
		PlatformId: "p_lazy", PlatformSkHash: hash,
		ShadowUserId: 0, Status: common.UserStatusEnabled,
	}).Error)
	r := makeAuthTestRouter(t)

	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("X-Platform-Id", "p_lazy")
	req.Header.Set("X-Platform-Sk", "pk_lazy")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code, w.Body.String())

	// Verify shadow user now exists and platform.shadow_user_id was persisted.
	var u model.User
	require.NoError(t, model.DB.Where("username = ?", "platform_p_lazy").First(&u).Error)
	assert.True(t, u.Id > 0)

	var p model.Platform
	require.NoError(t, model.DB.Where("platform_id = ?", "p_lazy").First(&p).Error)
	assert.Equal(t, u.Id, p.ShadowUserId)
}

// itoa is a tiny stdlib-only int-to-string for assertion substring builds.
// strconv.Itoa would do, but we keep this file's import surface minimal.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
