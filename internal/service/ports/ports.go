// Package ports 定义跨业务服务共享的应用层端口，抽象缓存、限流等基础设施能力。
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

// LoginThrottle 记录登录失败次数，用于账号锁定与限流。
type LoginThrottle interface {
	LoginFailureCount(ctx context.Context, loginName string) (int64, error)
	IncrLoginFailure(ctx context.Context, loginName string, window time.Duration) (int64, error)
	ClearLoginFailures(ctx context.Context, loginName string) error
}

// TxManager 提供显式事务边界，事务通过 context 透传给仓储实现。
type TxManager interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}
