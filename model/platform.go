package model

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Platform represents a downstream channel/reseller that authenticates via
// X-Platform-Id + X-Platform-Sk headers to access the /api/user/external/*
// (V1) and /api/v2/external/* (V2) endpoint groups.
//
// Platform identity is intentionally kept separate from User rows: users
// represents account/quota carriers (including auto-created shadow users
// for V2 token ownership), while Platform represents external API credentials.
// See pkg/billingexpr/expr.md style note: platform_sk is hashed with SHA-256
// (high-entropy random key, no salt needed) and verified via
// crypto/subtle.ConstantTimeCompare to defeat timing attacks.
type Platform struct {
	Id             int            `json:"id"`
	PlatformId     string         `json:"platform_id" gorm:"type:varchar(100);column:platform_id;uniqueIndex;not null"`
	Name           string         `json:"name" gorm:"type:varchar(200);column:name;default:''"`
	PlatformSkHash string         `json:"-" gorm:"type:varchar(64);column:platform_sk_hash;not null"` // hex(sha256(sk)), 64 chars; never serialized to JSON
	ShadowUserId   int            `json:"shadow_user_id" gorm:"column:shadow_user_id;not null;index"`
	Status         int            `json:"status" gorm:"type:int;default:1"` // 1=enabled, 2=disabled
	CreatedTime    int64          `json:"created_time" gorm:"bigint;autoCreateTime"`
	UpdatedTime    int64          `json:"updated_time" gorm:"bigint;autoUpdateTime"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`
}

// HashPlatformSk produces the canonical storage form of a plaintext sk:
// hex-encoded SHA-256. High-entropy random keys do not need a salt; collisions
// are negligible given 128+ bits of input entropy.
func HashPlatformSk(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// VerifyPlatformSk returns true iff the plaintext hashes to the stored value.
// Uses constant-time comparison to prevent timing side-channels.
func (p *Platform) VerifyPlatformSk(plaintext string) bool {
	if p == nil || p.PlatformSkHash == "" || plaintext == "" {
		return false
	}
	computed := HashPlatformSk(plaintext)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(p.PlatformSkHash)) == 1
}

// GetPlatformByPlatformId resolves a Platform by its external string id.
// Returns gorm.ErrRecordNotFound if the platform does not exist (or is soft-deleted).
func GetPlatformByPlatformId(platformId string) (*Platform, error) {
	if platformId == "" {
		return nil, errors.New("platform_id is empty")
	}
	var p Platform
	if err := DB.Where("platform_id = ?", platformId).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// GetPlatformById resolves a Platform by integer primary key.
func GetPlatformById(id int) (*Platform, error) {
	if id <= 0 {
		return nil, errors.New("invalid platform id")
	}
	var p Platform
	if err := DB.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// ListPlatforms returns a paginated list of platforms ordered by id desc.
// total is the unfiltered count for client-side pagination math.
func ListPlatforms(offset, limit int) (items []*Platform, total int64, err error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if err = DB.Model(&Platform{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err = DB.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

// Insert creates a new platform row. CreatedTime / UpdatedTime are populated
// by GORM autoCreate/UpdateTime tags.
func (p *Platform) Insert() error {
	if p.PlatformId == "" {
		return errors.New("platform_id is empty")
	}
	if p.PlatformSkHash == "" {
		return errors.New("platform_sk_hash is empty")
	}
	if p.Status == 0 {
		p.Status = common.UserStatusEnabled
	}
	if err := DB.Create(p).Error; err != nil {
		if strings.Contains(err.Error(), "Duplicate") ||
			strings.Contains(err.Error(), "UNIQUE") ||
			strings.Contains(err.Error(), "duplicate") {
			return errors.New("platform_id already exists")
		}
		return err
	}
	return nil
}

// UpdateFields applies a whitelist of mutable fields. platform_id and sk hash
// are immutable through this method by design (rotate-sk is out of scope per plan).
func (p *Platform) UpdateFields(updates map[string]interface{}) error {
	if p.Id <= 0 {
		return errors.New("platform not loaded")
	}
	allowed := map[string]bool{"name": true, "status": true}
	filtered := make(map[string]interface{}, len(updates))
	for k, v := range updates {
		if allowed[k] {
			filtered[k] = v
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return DB.Model(p).Updates(filtered).Error
}

// Delete soft-deletes the platform and cascades by disabling every token
// owned by its shadow user. All operations run inside a single transaction
// so partial failure leaves both sides intact.
func (p *Platform) Delete() (tokensDisabled int64, err error) {
	if p.Id <= 0 {
		return 0, errors.New("platform not loaded")
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		// Disable all enabled tokens belonging to the shadow user.
		// Match V1 DeactivateExternalUser semantics: keep historical tokens
		// for log integrity, just flip status to disabled.
		if p.ShadowUserId > 0 {
			res := tx.Model(&Token{}).
				Where("user_id = ? AND status = ?", p.ShadowUserId, common.TokenStatusEnabled).
				Update("status", common.TokenStatusDisabled)
			if res.Error != nil {
				return res.Error
			}
			tokensDisabled = res.RowsAffected
		}
		// Soft-delete the platform row (DeletedAt index makes it invisible
		// to GetPlatformByPlatformId on subsequent calls).
		if err := tx.Delete(p).Error; err != nil {
			return err
		}
		return nil
	})
	return tokensDisabled, err
}
