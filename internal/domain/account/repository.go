package account

import (
	"context"
	"time"
)

// UserRepository 是用户仓储端口。
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByLoginName(ctx context.Context, loginName string) (*User, error)
	GetByID(ctx context.Context, id int64) (*User, error)
	TouchLastLogin(ctx context.Context, id int64, at time.Time) error
	UpdateIMStatus(ctx context.Context, userID int64, status int8, syncedAt *time.Time, at time.Time) error
}

// CredentialRepository 是密码凭证仓储端口。
type CredentialRepository interface {
	Create(ctx context.Context, cred *Credential) error
	GetByUserID(ctx context.Context, userID int64) (*Credential, error)
	UpdatePasswordHash(ctx context.Context, userID int64, hash string, at time.Time) error
}

// SessionRepository 是会话仓储端口。
type SessionRepository interface {
	Create(ctx context.Context, session *Session) error
	GetByRefreshHash(ctx context.Context, hash string) (*Session, error)
	GetByID(ctx context.Context, id int64) (*Session, error)
	MarkRotated(ctx context.Context, id int64, at time.Time) (bool, error)
	Revoke(ctx context.Context, id int64, at time.Time) error
	RevokeAll(ctx context.Context, userID int64, at time.Time, exceptSID int64) ([]int64, error)
	RevokeFamily(ctx context.Context, familyID string, at time.Time) ([]int64, error)
	ListActive(ctx context.Context, userID int64, at time.Time) ([]*Session, error)
}

// outbox 任务状态。
const (
	OutboxStatusPending int8 = 0
	OutboxStatusDone    int8 = 1
	OutboxStatusFailed  int8 = 2
)

// OutboxActionProvisionIM 是注册后开通 IM 账号的任务类型。
const OutboxActionProvisionIM = "provision_im"

// OutboxEntry 是一条待补偿的 outbox 任务。
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

// IMOutboxRepository 持久化 IM 开通补偿任务。
// Enqueue 必须在调用方已开启的事务内执行，保证与账号数据同生共死。
type IMOutboxRepository interface {
	Enqueue(ctx context.Context, userID int64, action, payload string) error
	// FindPending 返回指定用户与动作最近一条未完成任务；不存在时返回 (nil, nil)。
	FindPending(ctx context.Context, userID int64, action string) (*OutboxEntry, error)
	ListDue(ctx context.Context, limit int) ([]OutboxEntry, error)
	MarkDone(ctx context.Context, id int64) error
	MarkRetry(ctx context.Context, id int64, attempts int, lastError string, nextRetryAt time.Time) error
}
