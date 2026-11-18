// Package file 封装文件对象元数据的 MySQL 数据访问。
package file

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	domain "goim-business/internal/domain/file"
	"goim-business/internal/model"
	"goim-business/internal/repository"
)

// Repo 是文件对象仓储实现。
type Repo struct {
	db *gorm.DB
}

// New 构造文件对象仓储。
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// Create 写入待上传记录。
func (r *Repo) Create(ctx context.Context, f *domain.File) error {
	rec := toRecord(f)
	if err := repository.DBFromContext(ctx, r.db).Create(rec).Error; err != nil {
		return err
	}
	f.ID = rec.ID
	f.CreatedAt = rec.CreatedAt
	f.UpdatedAt = rec.UpdatedAt
	return nil
}

// GetByID 按主键读取文件元数据。
func (r *Repo) GetByID(ctx context.Context, id int64) (*domain.File, error) {
	var rec model.FileObject
	err := repository.DBFromContext(ctx, r.db).Where("id = ?", id).First(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return fromRecord(&rec), nil
}

// MarkReady 把 pending 记录置为 ready，并回填对象实际大小与 MIME。
func (r *Repo) MarkReady(ctx context.Context, id, userID, size int64, mime string, at time.Time) (bool, error) {
	result := repository.DBFromContext(ctx, r.db).
		Model(&model.FileObject{}).
		Where("id = ? AND user_id = ? AND status = ?", id, userID, model.FileStatusPending).
		Updates(map[string]any{
			"size":       size,
			"mime":       mime,
			"status":     model.FileStatusReady,
			"updated_at": at,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// MarkDeleted 软删文件；pending 与 ready 均可删除。
func (r *Repo) MarkDeleted(ctx context.Context, id, userID int64, at time.Time) (bool, error) {
	result := repository.DBFromContext(ctx, r.db).
		Model(&model.FileObject{}).
		Where("id = ? AND user_id = ? AND status <> ?", id, userID, model.FileStatusDeleted).
		Updates(map[string]any{
			"status":     model.FileStatusDeleted,
			"updated_at": at,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func toRecord(f *domain.File) *model.FileObject {
	return &model.FileObject{
		ID:        f.ID,
		UserID:    f.UserID,
		Purpose:   f.Purpose,
		ObjectKey: f.ObjectKey,
		FileName:  f.FileName,
		MIME:      f.MIME,
		Size:      f.Size,
		SHA256:    f.SHA256,
		UploadID:  f.UploadID,
		Status:    f.Status,
		ExpiresAt: f.ExpiresAt,
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
	}
}

func fromRecord(rec *model.FileObject) *domain.File {
	return &domain.File{
		ID:        rec.ID,
		UserID:    rec.UserID,
		Purpose:   rec.Purpose,
		ObjectKey: rec.ObjectKey,
		FileName:  rec.FileName,
		MIME:      rec.MIME,
		Size:      rec.Size,
		SHA256:    rec.SHA256,
		UploadID:  rec.UploadID,
		Status:    rec.Status,
		ExpiresAt: rec.ExpiresAt,
		CreatedAt: rec.CreatedAt,
		UpdatedAt: rec.UpdatedAt,
	}
}
