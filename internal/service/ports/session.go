package ports

import (
	"context"
	"time"
)

// SessionStateCache 维护会话活跃标记，供鉴权链路快速判断登录态。
type SessionStateCache interface {
	Activate(ctx context.Context, sid, userID string, ttl time.Duration) error
	Deactivate(ctx context.Context, sid string) error
}
