// Package outbox 实现通用 outbox 任务表（outbox_events）的持久化。
package outbox

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"goim-business/internal/model"
	"goim-business/internal/pkg/timex"
	"goim-business/internal/repository"
	"goim-business/internal/service/ports"
)

func fromRecord(record *model.Outbox) ports.OutboxEntry {
	return ports.OutboxEntry{
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

// Repo 是 outbox_events 表的仓储实现。
type Repo struct {
	db *gorm.DB
}

// New 构造 outbox 仓储。
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

func (r *Repo) dbFrom(ctx context.Context) *gorm.DB {
	return repository.DBFromContext(ctx, r.db)
}

// Enqueue 写入一条待处理任务并返回任务 ID；复用上下文中的事务，保证与业务数据同事务提交。
func (r *Repo) Enqueue(ctx context.Context, userID int64, action, payload string) (int64, error) {
	now := timex.Now()
	record := &model.Outbox{
		UserID:      userID,
		Action:      action,
		Payload:     payload,
		Status:      ports.OutboxStatusPending,
		Attempts:    0,
		NextRetryAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := r.dbFrom(ctx).Create(record).Error; err != nil {
		return 0, err
	}
	return record.ID, nil
}

// GetByID 读取指定任务；不存在时返回 (nil, nil)。
func (r *Repo) GetByID(ctx context.Context, id int64) (*ports.OutboxEntry, error) {
	var record model.Outbox
	err := r.dbFrom(ctx).Where("id = ?", id).First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	entry := fromRecord(&record)
	return &entry, nil
}

// ListDue 取出已到期的待处理/失败任务，按到期时间升序。
func (r *Repo) ListDue(ctx context.Context, limit int) ([]ports.OutboxEntry, error) {
	if limit <= 0 {
		limit = 1
	}
	var records []model.Outbox
	err := r.dbFrom(ctx).
		Where("status IN ?", []int8{ports.OutboxStatusPending, ports.OutboxStatusFailed}).
		Where("next_retry_at <= ?", timex.Now()).
		Order("next_retry_at ASC, id ASC").
		Limit(limit).
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	entries := make([]ports.OutboxEntry, 0, len(records))
	for i := range records {
		entries = append(entries, fromRecord(&records[i]))
	}
	return entries, nil
}

// MarkDone 标记任务已完成；仅允许从待处理/失败态迁移，避免并发请求把已完成任务改回其他状态。
func (r *Repo) MarkDone(ctx context.Context, id int64) error {
	return r.dbFrom(ctx).Model(&model.Outbox{}).
		Where("id = ? AND status IN ?", id, []int8{ports.OutboxStatusPending, ports.OutboxStatusFailed}).
		Updates(map[string]any{
			"status":     ports.OutboxStatusDone,
			"updated_at": timex.Now(),
		}).Error
}

// MarkRetry 记录一次失败并安排下次重试时间；已完成任务不会被并发失败重新打开。
func (r *Repo) MarkRetry(ctx context.Context, id int64, attempts int, lastError string, nextRetryAt time.Time) error {
	return r.dbFrom(ctx).Model(&model.Outbox{}).
		Where("id = ? AND status IN ?", id, []int8{ports.OutboxStatusPending, ports.OutboxStatusFailed}).
		Updates(map[string]any{
			"status":        ports.OutboxStatusFailed,
			"attempts":      attempts,
			"last_error":    lastError,
			"next_retry_at": nextRetryAt,
			"updated_at":    timex.Now(),
		}).Error
}
