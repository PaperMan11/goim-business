package ports

import (
	"context"
	"time"
)

// IMToken 是 IM 侧签发的用户 token 及到期时间。
type IMToken struct {
	Value    string
	ExpireAt time.Time
}

// IMClient 负责在 IM 侧开通账号、同步资料并兑换用户 token。
type IMClient interface {
	Enabled() bool
	RegisterUser(ctx context.Context, userID, nickname string) error
	UpdateUserInfo(ctx context.Context, userID, nickname, avatarURL string) error
	UserToken(ctx context.Context, userID string, platformID int) (*IMToken, error)
}

// IMTokenCache 缓存按 (userID, platform) 维度的 IM token。
type IMTokenCache interface {
	Get(ctx context.Context, userID string, platform int8) (token string, expireAt time.Time, ok bool, err error)
	Set(ctx context.Context, userID string, platform int8, token string, expireAt time.Time, ttl time.Duration) error
}
