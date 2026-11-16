package ports

import (
	"context"
	"time"
)

// LoginThrottle 记录登录失败次数，用于账号锁定与限流。
type LoginThrottle interface {
	LoginFailureCount(ctx context.Context, loginName string) (int64, error)
	IncrLoginFailure(ctx context.Context, loginName string, window time.Duration) (int64, error)
	ClearLoginFailures(ctx context.Context, loginName string) error
}
