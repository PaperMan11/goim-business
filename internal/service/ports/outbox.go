package ports

import (
	"context"
	"time"
)

// outbox 任务状态。
const (
	OutboxStatusPending int8 = 0
	OutboxStatusDone    int8 = 1
	OutboxStatusFailed  int8 = 2
)

// OutboxActionProvisionIM 是注册后开通 IM 账号的任务类型。
const OutboxActionProvisionIM = "provision_im"

// OutboxActionProfileSync 是用户资料变更后同步到 IM 的任务类型。
const OutboxActionProfileSync = "profile_sync"

// OutboxEntry 是一条待处理的 outbox 任务。
type OutboxEntry struct {
	ID          int64
	UserID      int64
	Action      string
	Payload     string
	Status      int8
	Attempts    int
	NextRetryAt time.Time
	LastError   string
}

// OutboxRepository 是可靠投递任务的持久化端口，与具体业务无关。
// Enqueue 必须在调用方已开启的事务内执行，保证与业务数据同生共死。
type OutboxRepository interface {
	Enqueue(ctx context.Context, userID int64, action, payload string) error
	// FindPending 返回指定用户与动作最近一条未完成任务；不存在时返回 (nil, nil)。
	FindPending(ctx context.Context, userID int64, action string) (*OutboxEntry, error)
	ListDue(ctx context.Context, limit int) ([]OutboxEntry, error)
	MarkDone(ctx context.Context, id int64) error
	MarkRetry(ctx context.Context, id int64, attempts int, lastError string, nextRetryAt time.Time) error
}
