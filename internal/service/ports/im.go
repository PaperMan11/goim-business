package ports

import (
	"context"
	"errors"
	"time"
)

// ErrIMUserAlreadyExists 表示 IM 侧账号已存在：开通视为成功，但注册不更新既有资料，需补一次同步。
var ErrIMUserAlreadyExists = errors.New("IM 账号已存在")

// IMToken 是 IM 侧签发的用户 token 及到期时间。
type IMToken struct {
	Value    string
	ExpireAt time.Time
}

// IMClient 负责在 IM 侧开通账号、同步资料并兑换用户 token。
type IMClient interface {
	Enabled() bool
	RegisterUser(ctx context.Context, userID, nickname, avatarURL string) error
	UpdateUserInfo(ctx context.Context, userID, nickname, avatarURL string) error
	UserToken(ctx context.Context, userID string, platformID int) (*IMToken, error)
}

// IMTokenCache 缓存按 (userID, platform) 维度的 IM token。
type IMTokenCache interface {
	Get(ctx context.Context, userID string, platform int8) (token string, expireAt time.Time, ok bool, err error)
	Set(ctx context.Context, userID string, platform int8, token string, expireAt time.Time, ttl time.Duration) error
}
