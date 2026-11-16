// Package account 是账号域：定义实体、值对象、领域策略与仓储端口。
// 该包只依赖标准库，不感知数据库、HTTP 与外部错误码。
package account

import "time"

// 用户状态。
const (
	StatusNormal   int8 = 1
	StatusFrozen   int8 = 2
	StatusDeleting int8 = 3
	StatusDeleted  int8 = 4
)

// IM 账号开通状态。
const (
	IMStatusPending int8 = 0
	IMStatusReady   int8 = 1
	IMStatusFailed  int8 = 2
)

// User 是业务账号；ID 是数据库自增主键，对 IM 下发时转为字符串。
type User struct {
	ID          int64
	LoginName   string
	Nickname    string
	AvatarKey   string
	Bio         string
	Status      int8
	IMStatus    int8
	IMSyncedAt  *time.Time
	LastLoginAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CanAuthenticate 判断账号是否允许登录/刷新。
func (u User) CanAuthenticate() error {
	switch u.Status {
	case StatusNormal:
		return nil
	case StatusFrozen:
		return ErrAccountFrozen
	default:
		return ErrAccountDisabled
	}
}
