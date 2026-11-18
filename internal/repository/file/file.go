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
// 分片上传完成后清空 upload_id：此后记录只对应一个已完成对象，回收时不再需要 abort。
func (r *Repo) MarkReady(ctx context.Context, id, userID, size int64, mime string, at time.Time) (bool, error) {
	result := repository.DBFromContext(ctx, r.db).
		Model(&model.FileObject{}).
		Where("id = ? AND user_id = ? AND status = ?", id, userID, model.FileStatusPending).
		Updates(map[string]any{
			"size":       size,
			"mime":       mime,
			"upload_id":  "",
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

// ListExpiredPending 返回已过期且仍为 pending 的记录，供清理任务回收对象。
func (r *Repo) ListExpiredPending(ctx context.Context, now time.Time, limit int) ([]*domain.File, error) {
	var recs []model.FileObject
	err := repository.DBFromContext(ctx, r.db).
		Where("status = ? AND expires_at IS NOT NULL AND expires_at < ?", model.FileStatusPending, now).
		Order("expires_at ASC, id ASC").
		Limit(limit).
		Find(&recs).Error
	if err != nil {
		return nil, err
	}
	files := make([]*domain.File, 0, len(recs))
	for idx := range recs {
		files = append(files, fromRecord(&recs[idx]))
	}
	return files, nil
}

// MarkDeletedIfPending 仅把仍为 pending 的记录置为 deleted，避免与 complete 竞争。
func (r *Repo) MarkDeletedIfPending(ctx context.Context, id, userID int64, at time.Time) (bool, error) {
	result := repository.DBFromContext(ctx, r.db).
		Model(&model.FileObject{}).
		Where("id = ? AND user_id = ? AND status = ?", id, userID, model.FileStatusPending).
		Updates(map[string]any{
			"status":     model.FileStatusDeleted,
			"updated_at": at,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// ListDeletedObjects 返回已删除但对象尚未确认回收的记录。
func (r *Repo) ListDeletedObjects(ctx context.Context, limit int) ([]*domain.File, error) {
	if limit <= 0 {
		limit = 1
	}
	var recs []model.FileObject
	err := repository.DBFromContext(ctx, r.db).
		Where("status = ? AND object_deleted_at IS NULL", model.FileStatusDeleted).
		Order("updated_at ASC, id ASC").
		Limit(limit).
		Find(&recs).Error
	if err != nil {
		return nil, err
	}
	files := make([]*domain.File, 0, len(recs))
	for idx := range recs {
		files = append(files, fromRecord(&recs[idx]))
	}
	return files, nil
}

// MarkObjectDeleted 记录对象已确认回收；条件更新保证重复执行幂等。
func (r *Repo) MarkObjectDeleted(ctx context.Context, id int64, at time.Time) (bool, error) {
	result := repository.DBFromContext(ctx, r.db).
		Model(&model.FileObject{}).
		Where("id = ? AND status = ? AND object_deleted_at IS NULL", id, model.FileStatusDeleted).
		Updates(map[string]any{
			"object_deleted_at": at,
			"updated_at":        at,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// MarkDeletedByUser 把用户全部未删除记录置为 deleted，返回受影响条数。
func (r *Repo) MarkDeletedByUser(ctx context.Context, userID int64, at time.Time) (int64, error) {
	result := repository.DBFromContext(ctx, r.db).
		Model(&model.FileObject{}).
		Where("user_id = ? AND status <> ?", userID, model.FileStatusDeleted).
		Updates(map[string]any{
			"status":     model.FileStatusDeleted,
			"updated_at": at,
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

func toRecord(f *domain.File) *model.FileObject {
	return &model.FileObject{
		ID:              f.ID,
		UserID:          f.UserID,
		Purpose:         f.Purpose,
		ObjectKey:       f.ObjectKey,
		FileName:        f.FileName,
		MIME:            f.MIME,
		Size:            f.Size,
		SHA256:          f.SHA256,
		UploadID:        f.UploadID,
		Status:          f.Status,
		ExpiresAt:       f.ExpiresAt,
		ObjectDeletedAt: f.ObjectDeletedAt,
		CreatedAt:       f.CreatedAt,
		UpdatedAt:       f.UpdatedAt,
	}
}

func fromRecord(rec *model.FileObject) *domain.File {
	return &domain.File{
		ID:              rec.ID,
		UserID:          rec.UserID,
		Purpose:         rec.Purpose,
		ObjectKey:       rec.ObjectKey,
		FileName:        rec.FileName,
		MIME:            rec.MIME,
		Size:            rec.Size,
		SHA256:          rec.SHA256,
		UploadID:        rec.UploadID,
		Status:          rec.Status,
		ExpiresAt:       rec.ExpiresAt,
		ObjectDeletedAt: rec.ObjectDeletedAt,
		CreatedAt:       rec.CreatedAt,
		UpdatedAt:       rec.UpdatedAt,
	}
}
