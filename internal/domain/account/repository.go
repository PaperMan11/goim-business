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
	GetByIDs(ctx context.Context, ids []int64) ([]*User, error)
	TouchLastLogin(ctx context.Context, id int64, at time.Time) error
	UpdateIMStatus(ctx context.Context, userID int64, status int8, syncedAt *time.Time, at time.Time) error
	UpdateProfile(ctx context.Context, userID int64, nickname, avatarKey, bio string, at time.Time) error
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
