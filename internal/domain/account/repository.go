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
	// RevokeByDevice 撤销用户指定设备的全部活跃会话，返回被撤销的 sid。
	RevokeByDevice(ctx context.Context, userID int64, deviceID string, at time.Time) ([]int64, error)
	ListActive(ctx context.Context, userID int64, at time.Time) ([]*Session, error)
}

// DeviceRepository 是设备登记表仓储端口。
type DeviceRepository interface {
	// Upsert 按 (userID, deviceID) 覆盖写入设备，保留首次注册时间。
	Upsert(ctx context.Context, device *Device) error
	ListByUser(ctx context.Context, userID int64) ([]*Device, error)
	// Delete 删除设备记录；返回记录是否存在。
	Delete(ctx context.Context, userID int64, deviceID string) (bool, error)
}
