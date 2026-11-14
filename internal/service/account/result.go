package account

import (
	"strconv"
	"time"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/types"
)

// TokenPair 是本次签发的令牌。
type TokenPair struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
	SessionID        int64
	Platform         int8
}

// authResp 组装注册/登录/刷新成功响应，供 handler 直接输出。
func (s *Service) authResp(user *domain.User, tokens TokenPair) *types.AuthResp {
	return &types.AuthResp{
		User: types.UserView{
			UserID:    strconv.FormatInt(user.ID, 10),
			LoginName: user.LoginName,
			Nickname:  user.Nickname,
		},
		Token: types.TokenView{
			AccessToken: tokens.AccessToken,
			ExpiresIn:   int64(s.cfg.AccessTTL.Seconds()),
		},
		RefreshToken:     tokens.RefreshToken,
		RefreshExpiresIn: int64(s.cfg.RefreshTTL.Seconds()),
		SessionID:        strconv.FormatInt(tokens.SessionID, 10),
		Platform:         tokens.Platform,
	}
}
