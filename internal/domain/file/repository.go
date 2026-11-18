package file

import (
	"context"
	"time"
)

// Repository 是文件对象仓储端口。
type Repository interface {
	Create(ctx context.Context, f *File) error
	GetByID(ctx context.Context, id int64) (*File, error)
	// MarkReady 仅把 pending 记录置为 ready 并回填实际大小与 MIME；返回是否更新成功。
	MarkReady(ctx context.Context, id, userID, size int64, mime string, at time.Time) (bool, error)
	// MarkDeleted 仅把 ready 记录置为 deleted；返回是否更新成功。
	MarkDeleted(ctx context.Context, id, userID int64, at time.Time) (bool, error)
	// ListExpiredPending 返回 expires_at 已过期且仍为 pending 的记录，按过期时间升序，最多 limit 条。
	ListExpiredPending(ctx context.Context, now time.Time, limit int) ([]*File, error)
	// MarkDeletedIfPending 仅把仍为 pending 的记录置为 deleted；返回是否更新成功。
	MarkDeletedIfPending(ctx context.Context, id, userID int64, at time.Time) (bool, error)
}
