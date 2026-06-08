package controller

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ==================== Request / Response Structs ====================

type CreatePlatformRequest struct {
	PlatformId   string `json:"platform_id" binding:"required,min=1,max=80"`
	Name         string `json:"name" binding:"max=200"`
	Status       *int   `json:"status"` // 1=enabled (default), 2=disabled
	ShadowUserId *int   `json:"shadow_user_id"`
}

type UpdatePlatformRequest struct {
	Name         *string `json:"name"`
	Status       *int    `json:"status"` // 1=enabled, 2=disabled
	ShadowUserId *int    `json:"shadow_user_id"`
}

// platformIdPattern restricts platform_id to a charset that produces a valid
// shadow-user `username = "platform_<id>"` without surprises.
var platformIdPattern = regexp.MustCompile(`^[a-z0-9_-]{1,80}$`)

// platformDTO renders a Platform for admin responses; never includes the hash.
func platformDTO(p *model.Platform) gin.H {
	return gin.H{
		"id":             p.Id,
		"platform_id":    p.PlatformId,
		"name":           p.Name,
		"status":         p.Status,
		"shadow_user_id": p.ShadowUserId,
		"created_at":     time.Unix(p.CreatedTime, 0).UTC().Format(time.RFC3339),
		"updated_at":     time.Unix(p.UpdatedTime, 0).UTC().Format(time.RFC3339),
	}
}

func resolvePlatformShadowUserId(platformId string, shadowUserId *int) (int, error) {
	if shadowUserId != nil {
		if *shadowUserId <= 0 {
			return 0, errors.New("shadow_user_id 必须为正整数")
		}
		var user model.User
		if err := model.DB.First(&user, *shadowUserId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return 0, errors.New("shadow_user_id 对应用户不存在")
			}
			return 0, err
		}
		return user.Id, nil
	}

	shadowUser, err := model.GetOrCreatePlatformShadowUser(platformId)
	if err != nil || shadowUser == nil {
		return 0, errors.New("创建平台 shadow user 失败")
	}
	return shadowUser.Id, nil
}

// ==================== Handlers ====================

// CreatePlatform provisions a new platform. The plaintext platform_sk is
// generated server-side and returned exactly once in the response — it is
// never persisted in plaintext and cannot be retrieved later. Callers must
// store it securely; loss requires DELETE + recreate.
//
// POST /api/admin/v2/platforms
// AdminAuth required.
func CreatePlatform(c *gin.Context) {
	var req CreatePlatformRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "请求参数错误: "+err.Error())
		return
	}
	if !platformIdPattern.MatchString(req.PlatformId) {
		common.ApiErrorMsg(c, "platform_id 格式错误：仅允许小写字母、数字、下划线、短横线，长度 1-80")
		return
	}
	if req.Status != nil && *req.Status != common.UserStatusEnabled && *req.Status != common.UserStatusDisabled {
		common.ApiErrorMsg(c, "status 仅允许 1 (enabled) 或 2 (disabled)")
		return
	}
	status := common.UserStatusEnabled
	if req.Status != nil {
		status = *req.Status
	}

	shadowUserId, err := resolvePlatformShadowUserId(req.PlatformId, req.ShadowUserId)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	// Generate a high-entropy sk; "pk_" prefix distinguishes platform creds
	// from end-user tokens (sk-...). 40 random alphanumerics ≈ 238 bits entropy.
	plaintextSk := "pk_" + common.GetRandomString(40)

	platform := &model.Platform{
		PlatformId:     req.PlatformId,
		Name:           req.Name,
		PlatformSkHash: model.HashPlatformSk(plaintextSk),
		ShadowUserId:   shadowUserId,
		Status:         status,
	}

	if err := platform.Insert(); err != nil {
		if err.Error() == "platform_id already exists" {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "platform_id 已存在"})
			return
		}
		common.ApiErrorMsg(c, "创建平台失败: "+err.Error())
		return
	}

	common.ApiSuccess(c, gin.H{
		"id":             platform.Id,
		"platform_id":    platform.PlatformId,
		"name":           platform.Name,
		"status":         platform.Status,
		"shadow_user_id": platform.ShadowUserId,
		"platform_sk":    plaintextSk, // one-time exposure
		"created_at":     time.Unix(platform.CreatedTime, 0).UTC().Format(time.RFC3339),
		"warning":        "请妥善保管 platform_sk，此密钥仅本次返回，丢失需删除并重新创建平台",
	})
}

// ListPlatforms returns paginated platforms. The hash is never returned;
// admins can identify platforms by platform_id / name.
//
// GET /api/admin/v2/platforms?page=1&page_size=20
func ListPlatforms(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	items, total, err := model.ListPlatforms((page-1)*pageSize, pageSize)
	if err != nil {
		common.ApiErrorMsg(c, "查询平台列表失败")
		return
	}

	out := make([]gin.H, 0, len(items))
	for _, p := range items {
		out = append(out, platformDTO(p))
	}

	common.ApiSuccess(c, gin.H{
		"items":     out,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetPlatform returns a single platform by integer id.
//
// GET /api/admin/v2/platforms/:id
func GetPlatform(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "id 必须为正整数")
		return
	}
	p, err := model.GetPlatformById(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "平台不存在"})
			return
		}
		common.ApiErrorMsg(c, "查询失败")
		return
	}
	common.ApiSuccess(c, platformDTO(p))
}

// UpdatePlatform allows admins to rename a platform or toggle its status.
// platform_id and sk are intentionally not mutable through this endpoint.
//
// PATCH /api/admin/v2/platforms/:id
// Body: { name?, status? }
func UpdatePlatform(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "id 必须为正整数")
		return
	}
	var req UpdatePlatformRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "请求参数错误: "+err.Error())
		return
	}
	if req.Status != nil && *req.Status != common.UserStatusEnabled && *req.Status != common.UserStatusDisabled {
		common.ApiErrorMsg(c, "status 仅允许 1 (enabled) 或 2 (disabled)")
		return
	}

	p, err := model.GetPlatformById(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "平台不存在"})
			return
		}
		common.ApiErrorMsg(c, "查询失败")
		return
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		if len(*req.Name) > 200 {
			common.ApiErrorMsg(c, "name 长度不能超过 200 字符")
			return
		}
		updates["name"] = *req.Name
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.ShadowUserId != nil {
		shadowUserId, err := resolvePlatformShadowUserId(p.PlatformId, req.ShadowUserId)
		if err != nil {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		updates["shadow_user_id"] = shadowUserId
	}
	if len(updates) == 0 {
		common.ApiErrorMsg(c, "无可更新字段")
		return
	}

	if err := p.UpdateFields(updates); err != nil {
		common.ApiErrorMsg(c, "更新失败: "+err.Error())
		return
	}

	// Re-read to surface the updated_at change.
	updated, err := model.GetPlatformById(id)
	if err != nil {
		common.ApiErrorMsg(c, "查询失败")
		return
	}
	common.ApiSuccess(c, platformDTO(updated))
}

// DeletePlatform soft-deletes a platform and disables all tokens owned by
// its shadow user (cascading via Platform.Delete's transaction). Historical
// tokens are kept (status=disabled) so log integrity is preserved.
//
// DELETE /api/admin/v2/platforms/:id
func DeletePlatform(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "id 必须为正整数")
		return
	}
	p, err := model.GetPlatformById(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "平台不存在"})
			return
		}
		common.ApiErrorMsg(c, "查询失败")
		return
	}

	tokensDisabled, err := p.Delete()
	if err != nil {
		common.ApiErrorMsg(c, "删除失败: "+err.Error())
		return
	}

	common.ApiSuccess(c, gin.H{
		"id":              p.Id,
		"platform_id":     p.PlatformId,
		"tokens_disabled": tokensDisabled,
	})
}
