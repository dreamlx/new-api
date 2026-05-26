package middleware

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Context keys exposed by PlatformAuth so downstream handlers can resolve the
// authenticated caller without re-querying the database.
const (
	CtxKeyPlatform     = "platform"         // *model.Platform
	CtxKeyShadowUserId = "shadow_user_id"   // int — user.Id of the V2 shadow user owning tokens
	CtxKeyPlatformId   = "auth_platform_id" // string — convenience copy of platform.PlatformId
)

// PlatformAuth is the unified gateway middleware for /api/user/external/* (V1)
// and /api/v2/external/* (V2) endpoint groups. It validates the calling
// downstream platform via two headers and provisions context for handlers.
//
// Required headers:
//   - X-Platform-Id: the platform's external identifier (e.g. "asd")
//   - X-Platform-Sk: the platform's plaintext secret key
//
// Outcomes:
//   - 401 unauthorized: missing/invalid headers, unknown platform, sk mismatch
//   - 403 forbidden: platform exists but is disabled (status != enabled)
//   - 200 (pass through): ctx["platform"], ctx["shadow_user_id"], ctx["auth_platform_id"] set
//
// Implementation notes:
//   - sk verification uses crypto/subtle.ConstantTimeCompare via model.Platform.VerifyPlatformSk
//   - shadow user is lazily provisioned (and ShadowUserId persisted) on first auth
//   - no platform_sk fragment ever appears in response messages or logs
func PlatformAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		platformId := c.GetHeader("X-Platform-Id")
		platformSk := c.GetHeader("X-Platform-Sk")

		if platformId == "" || platformSk == "" {
			respondPlatformUnauthorized(c)
			return
		}

		platform, err := model.GetPlatformByPlatformId(platformId)
		if err != nil {
			// Treat "not found" exactly like "wrong sk" — do not leak existence.
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondPlatformUnauthorized(c)
				return
			}
			respondPlatformServerError(c)
			return
		}

		// Status gate: disabled platforms cannot transact even if sk is correct.
		if platform.Status != common.UserStatusEnabled {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "platform disabled",
			})
			c.Abort()
			return
		}

		// Constant-time verification (see model.Platform.VerifyPlatformSk).
		if !platform.VerifyPlatformSk(platformSk) {
			respondPlatformUnauthorized(c)
			return
		}

		// V2 (and any future feature relying on platform.shadow_user_id) needs
		// a live user row to hang tokens off. V1 handlers ignore this value
		// and continue resolving by external_user_id.
		if platform.ShadowUserId > 0 {
			var user model.User
			if err := model.DB.Select("id").First(&user, platform.ShadowUserId).Error; err != nil {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					respondPlatformServerError(c)
					return
				}
				platform.ShadowUserId = 0
			}
		}
		if platform.ShadowUserId <= 0 {
			user, err := model.GetOrCreatePlatformShadowUser(platform.PlatformId)
			if err != nil || user == nil {
				respondPlatformServerError(c)
				return
			}
			platform.ShadowUserId = user.Id
			if err := model.DB.Model(platform).Update("shadow_user_id", user.Id).Error; err != nil {
				respondPlatformServerError(c)
				return
			}
		}

		c.Set(CtxKeyPlatform, platform)
		c.Set(CtxKeyShadowUserId, platform.ShadowUserId)
		c.Set(CtxKeyPlatformId, platform.PlatformId)
		c.Next()
	}
}

// respondPlatformUnauthorized returns a single generic 401 used for every
// failure path that should not leak which check tripped (missing header,
// unknown platform, sk mismatch). This is the timing-and-message blinding
// half of constant-time auth.
func respondPlatformUnauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{
		"success": false,
		"message": "unauthorized",
	})
	c.Abort()
}

func respondPlatformServerError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{
		"success": false,
		"message": "internal error",
	})
	c.Abort()
}

// PlatformFromContext returns the authenticated platform set by PlatformAuth.
// Returns nil if the middleware did not run or the key is missing.
func PlatformFromContext(c *gin.Context) *model.Platform {
	v, ok := c.Get(CtxKeyPlatform)
	if !ok {
		return nil
	}
	p, _ := v.(*model.Platform)
	return p
}

// ShadowUserIdFromContext returns the shadow user id for the authenticated
// platform; 0 if absent.
func ShadowUserIdFromContext(c *gin.Context) int {
	v, ok := c.Get(CtxKeyShadowUserId)
	if !ok {
		return 0
	}
	id, _ := v.(int)
	return id
}
