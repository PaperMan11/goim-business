package account

import (
	"context"
	"time"

	"gorm.io/gorm"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/model"
	"goim-business/internal/repository"
)

func credentialRecord(cred *domain.Credential) *model.Credential {
	return &model.Credential{
		UserID:       cred.UserID,
		PasswordHash: cred.PasswordHash,
		UpdatedAt:    cred.UpdatedAt,
	}
}

func credentialFromRecord(record *model.Credential) *domain.Credential {
	return &domain.Credential{
		UserID:       record.UserID,
		PasswordHash: record.PasswordHash,
		UpdatedAt:    record.UpdatedAt,
	}
}

// CredentialRepo 是 account_credentials 表的仓储实现。
type CredentialRepo struct {
	db *gorm.DB
}

// NewCredentialRepo 构造凭证仓储。
func NewCredentialRepo(db *gorm.DB) *CredentialRepo {
	return &CredentialRepo{db: db}
}

func (r *CredentialRepo) dbFrom(ctx context.Context) *gorm.DB {
	return repository.DBFromContext(ctx, r.db)
}

// Create 插入密码凭证。
func (r *CredentialRepo) Create(ctx context.Context, cred *domain.Credential) error {
	return mapErr(r.dbFrom(ctx).Create(credentialRecord(cred)).Error)
}

// GetByUserID 查询密码凭证。
func (r *CredentialRepo) GetByUserID(ctx context.Context, userID int64) (*domain.Credential, error) {
	var record model.Credential
	err := r.dbFrom(ctx).Where("user_id = ?", userID).First(&record).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return credentialFromRecord(&record), nil
}

// UpdatePasswordHash 更新密码哈希。
func (r *CredentialRepo) UpdatePasswordHash(ctx context.Context, userID int64, hash string, at time.Time) error {
	return mapErr(r.dbFrom(ctx).Model(&model.Credential{}).
		Where("user_id = ?", userID).
		Updates(map[string]any{"password_hash": hash, "updated_at": at}).Error)
}
