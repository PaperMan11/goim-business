package account

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/model"
	"goim-business/internal/pkg/timex"
	"goim-business/internal/repository"
)

func outboxFromRecord(record *model.IMOutbox) domain.OutboxEntry {
	return domain.OutboxEntry{
		ID:          record.ID,
		UserID:      record.UserID,
		Action:      record.Action,
		Payload:     record.Payload,
		Status:      record.Status,
		Attempts:    record.Attempts,
		NextRetryAt: record.NextRetryAt,
		LastError:   record.LastError,
	}
}

// OutboxRepo 是 account_im_outbox 表的仓储实现。
type OutboxRepo struct {
	db *gorm.DB
}

// NewOutboxRepo 构造 outbox 仓储。
func NewOutboxRepo(db *gorm.DB) *OutboxRepo {
	return &OutboxRepo{db: db}
}

func (r *OutboxRepo) dbFrom(ctx context.Context) *gorm.DB {
	return repository.DBFromContext(ctx, r.db)
}

// Enqueue 写入一条待处理任务；复用上下文中的事务，保证与账号数据同事务提交。
func (r *OutboxRepo) Enqueue(ctx context.Context, userID int64, action, payload string) error {
	now := timex.Now()
	return r.dbFrom(ctx).Create(&model.IMOutbox{
		UserID:      userID,
		Action:      action,
		Payload:     payload,
		Status:      domain.OutboxStatusPending,
		Attempts:    0,
		NextRetryAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}).Error
}

// FindPending 返回指定用户与动作最近一条未完成任务；不存在时返回 (nil, nil)。
func (r *OutboxRepo) FindPending(ctx context.Context, userID int64, action string) (*domain.OutboxEntry, error) {
	var record model.IMOutbox
	err := r.dbFrom(ctx).
		Where("user_id = ? AND action = ? AND status IN ?",
			userID, action, []int8{domain.OutboxStatusPending, domain.OutboxStatusFailed}).
		Order("id DESC").
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	entry := outboxFromRecord(&record)
	return &entry, nil
}

// ListDue 取出已到期的待处理/失败任务，按到期时间升序。
func (r *OutboxRepo) ListDue(ctx context.Context, limit int) ([]domain.OutboxEntry, error) {
	if limit <= 0 {
		limit = 1
	}
	var records []model.IMOutbox
	err := r.dbFrom(ctx).
		Where("status IN ?", []int8{domain.OutboxStatusPending, domain.OutboxStatusFailed}).
		Where("next_retry_at <= ?", timex.Now()).
		Order("next_retry_at ASC, id ASC").
		Limit(limit).
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	entries := make([]domain.OutboxEntry, 0, len(records))
	for i := range records {
		entries = append(entries, outboxFromRecord(&records[i]))
	}
	return entries, nil
}

// MarkDone 标记任务已完成。
func (r *OutboxRepo) MarkDone(ctx context.Context, id int64) error {
	return r.dbFrom(ctx).Model(&model.IMOutbox{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     domain.OutboxStatusDone,
			"updated_at": timex.Now(),
		}).Error
}

// MarkRetry 记录一次失败并安排下次重试时间。
func (r *OutboxRepo) MarkRetry(ctx context.Context, id int64, attempts int, lastError string, nextRetryAt time.Time) error {
	return r.dbFrom(ctx).Model(&model.IMOutbox{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":        domain.OutboxStatusFailed,
			"attempts":      attempts,
			"last_error":    lastError,
			"next_retry_at": nextRetryAt,
			"updated_at":    timex.Now(),
		}).Error
}
