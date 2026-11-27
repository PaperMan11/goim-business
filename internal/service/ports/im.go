package ports

import (
	"context"
	"errors"
	"time"
)

// ErrIMUserAlreadyExists 表示 IM 侧账号已存在：开通视为成功，但注册不更新既有资料，需补一次同步。
var ErrIMUserAlreadyExists = errors.New("IM 账号已存在")

// ErrIMUserNotFound 表示 IM 侧没有该用户：注销占位可视为已完成。
var ErrIMUserNotFound = errors.New("IM 账号不存在")

// IMToken 是 IM 侧签发的用户 token 及到期时间。
type IMToken struct {
	Value    string
	ExpireAt time.Time
}

// IMClient 负责在 IM 侧开通账号、同步资料、兑换用户 token，并把业务侧的会话撤销同步到 IM。
// 实现必须幂等：设备不在线或 IM 未启用时不应报错。
type IMClient interface {
	Enabled() bool
	RegisterUser(ctx context.Context, userID, nickname, avatarURL string) error
	UpdateUserInfo(ctx context.Context, userID, nickname, avatarURL string) error
	UserToken(ctx context.Context, userID string, platformID int) (*IMToken, error)
	// KickDevice 断开目标设备的连接并使其 IM token 失效。
	KickDevice(ctx context.Context, userID string, platform int8, deviceID string) error
}

// IMTokenCache 缓存按 (userID, platform) 维度的 IM token。
type IMTokenCache interface {
	Get(ctx context.Context, userID string, platform int8) (token string, expireAt time.Time, ok bool, err error)
	Set(ctx context.Context, userID string, platform int8, token string, expireAt time.Time, ttl time.Duration) error
	// Delete 清除指定平台的缓存；注销时按平台逐个删除，不使用 SCAN。
	Delete(ctx context.Context, userID string, platform int8) error
}
