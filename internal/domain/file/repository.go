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
}
