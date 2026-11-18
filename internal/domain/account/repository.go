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
	// GetByIDForUpdate 在当前事务内按主键加行锁读取用户，用于登录/刷新与注销串行化。
	GetByIDForUpdate(ctx context.Context, id int64) (*User, error)
	GetByIDs(ctx context.Context, ids []int64) ([]*User, error)
	TouchLastLogin(ctx context.Context, id int64, at time.Time) error
	UpdateIMStatus(ctx context.Context, userID int64, status int8, syncedAt *time.Time, at time.Time) error
	// UpdateProfile 仅在账号正常时更新资料；返回是否更新成功。
	UpdateProfile(ctx context.Context, userID int64, nickname, avatarKey, bio string, at time.Time) (bool, error)
	// MarkDeleting 把正常账号置为注销中并写入物理清除时间；返回是否更新成功。
	MarkDeleting(ctx context.Context, userID int64, purgeAt, at time.Time) (bool, error)
	// ListDueForPurge 返回注销中且已到清除时间的账号，按 purge_at 升序，最多 limit 条。
	ListDueForPurge(ctx context.Context, now time.Time, limit int) ([]*User, error)
	// MarkPurged 把注销中账号置为已注销并清除资料；loginName 用于释放原登录名。
	MarkPurged(ctx context.Context, userID int64, loginName string, at time.Time) (bool, error)
}

// CredentialRepository 是密码凭证仓储端口。
type CredentialRepository interface {
	Create(ctx context.Context, cred *Credential) error
	GetByUserID(ctx context.Context, userID int64) (*Credential, error)
	UpdatePasswordHash(ctx context.Context, userID int64, hash string, at time.Time) error
	// DeleteByUser 删除用户密码凭证；不存在时视为成功。
	DeleteByUser(ctx context.Context, userID int64) error
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
	// DeleteByUser 物理删除用户全部会话，返回删除条数。
	DeleteByUser(ctx context.Context, userID int64) (int64, error)
}

// DeviceRepository 是设备登记表仓储端口。
type DeviceRepository interface {
	// Upsert 按 (userID, deviceID) 覆盖写入设备，保留首次注册时间。
	Upsert(ctx context.Context, device *Device) error
	ListByUser(ctx context.Context, userID int64) ([]*Device, error)
	// Delete 删除设备记录；返回记录是否存在。
	Delete(ctx context.Context, userID int64, deviceID string) (bool, error)
	// DeleteByUser 物理删除用户全部设备记录，返回删除条数。
	DeleteByUser(ctx context.Context, userID int64) (int64, error)
}
