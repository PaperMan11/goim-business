// Package account 封装账号域的 MySQL 数据访问。
package account

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/model"
	"goim-business/internal/repository"
)

// mapErr 把 gorm 错误翻译为账号领域错误。
func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.ErrNotFound
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return domain.ErrDuplicate
	default:
		return err
	}
}

func userRecord(user *domain.User) *model.User {
	return &model.User{
		ID:          user.ID,
		LoginName:   user.LoginName,
		Nickname:    user.Nickname,
		AvatarKey:   user.AvatarKey,
		Bio:         user.Bio,
		Status:      user.Status,
		LastLoginAt: user.LastLoginAt,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
	}
}

func userFromRecord(record *model.User) *domain.User {
	return &domain.User{
		ID:          record.ID,
		LoginName:   record.LoginName,
		Nickname:    record.Nickname,
		AvatarKey:   record.AvatarKey,
		Bio:         record.Bio,
		Status:      record.Status,
		LastLoginAt: record.LastLoginAt,
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	}
}

// UserRepo 是 account_users 表的仓储实现。
type UserRepo struct {
	db *gorm.DB
}

// NewUserRepo 构造用户仓储。
func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) dbFrom(ctx context.Context) *gorm.DB {
	return repository.DBFromContext(ctx, r.db)
}

// Create 插入用户并回填自增 ID；登录名冲突返回 domain.ErrDuplicate。
func (r *UserRepo) Create(ctx context.Context, user *domain.User) error {
	record := userRecord(user)
	if err := mapErr(r.dbFrom(ctx).Create(record).Error); err != nil {
		return err
	}
	user.ID = record.ID
	return nil
}

// GetByLoginName 按登录名查询用户。
func (r *UserRepo) GetByLoginName(ctx context.Context, loginName string) (*domain.User, error) {
	var record model.User
	err := r.dbFrom(ctx).Where("login_name = ?", loginName).First(&record).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return userFromRecord(&record), nil
}

// GetByID 按 userID 查询用户。
func (r *UserRepo) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	var record model.User
	err := r.dbFrom(ctx).Where("id = ?", id).First(&record).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return userFromRecord(&record), nil
}

// TouchLastLogin 更新最后登录时间。
func (r *UserRepo) TouchLastLogin(ctx context.Context, id int64, at time.Time) error {
	return mapErr(r.dbFrom(ctx).Model(&model.User{}).
		Where("id = ?", id).
		Updates(map[string]any{"last_login_at": at, "updated_at": at}).Error)
}
