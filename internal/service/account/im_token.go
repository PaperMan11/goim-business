package account

import (
	"context"
	"errors"
	"math"
	"time"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/pkg/convert"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/types"
)

// imTokenTTLGuard 是写缓存时从剩余有效期扣减的时间：缓存命中时 token 至少还有该时长的有效期。
const imTokenTTLGuard = 10 * time.Minute

// IMToken 为指定用户与平台兑换 IM token，命中缓存时直接返回。
func (s *Service) IMToken(ctx context.Context, userID int64, platform int8) (*types.IMTokenResp, error) {
	if s.im == nil || !s.im.Enabled() {
		return nil, errcode.ErrDependency.WithMessage("IM 未启用")
	}
	if platform < 1 || platform >= 10 {
		return nil, errcode.ErrInvalidParam.WithMessage("平台不支持")
	}

	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		return nil, s.dbErr("查询 IM token 用户", err)
	}
	if err := user.CanAuthenticate(); err != nil {
		if errors.Is(err, domain.ErrAccountFrozen) {
			return nil, errcode.ErrAccountFrozen
		}
		return nil, errcode.ErrUnauthenticated
	}
	if user.IMStatus != domain.IMStatusReady {
		return nil, errcode.ErrDependency.WithMessage("IM 账号尚未开通完成，请稍后重试")
	}

	key := convert.ToString(userID)
	var (
		tokenValue string
		expireAt   time.Time
	)
	hit := false
	if s.imTokens != nil {
		var cacheErr error
		tokenValue, expireAt, hit, cacheErr = s.imTokens.Get(ctx, key, platform)
		if cacheErr != nil {
			// 缓存读失败按未命中处理，不让 IM token 兑换被 Redis 拖垮。
			s.log.Errorf("读取 IM token 缓存失败: user_id=%d, platform=%d, err=%v", userID, platform, cacheErr)
			hit = false
		}
	}
	if hit {
		return s.imTokenResp(userID, platform, tokenValue, expireAt), nil
	}

	tok, err := s.im.UserToken(ctx, key, int(platform))
	if err != nil {
		s.log.Errorf("兑换 IM token 失败: user_id=%d, platform=%d, err=%v", userID, platform, err)
		return nil, errcode.ErrDependency
	}
	expireAt = tok.ExpireAt
	if s.imTokens != nil {
		if ttl := expireAt.Sub(s.now()) - imTokenTTLGuard; ttl > 0 {
			if err := s.imTokens.Set(ctx, key, platform, tok.Value, expireAt, ttl); err != nil {
				s.log.Errorf("写入 IM token 缓存失败: user_id=%d, platform=%d, err=%v", userID, platform, err)
			}
		}
	}
	return s.imTokenResp(userID, platform, tok.Value, expireAt), nil
}

// IMStatus 返回用户 IM 账号的开通状态。
func (s *Service) IMStatus(ctx context.Context, userID int64) (*types.IMStatusResp, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		return nil, s.dbErr("查询 IM 开通状态", err)
	}
	return &types.IMStatusResp{
		UserID:   convert.ToString(user.ID),
		Status:   user.IMStatus,
		SyncedAt: user.IMSyncedAt,
	}, nil
}

// imTokenResp 按剩余有效期组装响应。
func (s *Service) imTokenResp(userID int64, platform int8, token string, expireAt time.Time) *types.IMTokenResp {
	seconds := int64(math.Ceil(expireAt.Sub(s.now()).Seconds()))
	if seconds < 0 {
		seconds = 0
	}
	return &types.IMTokenResp{
		UserID:            convert.ToString(userID),
		Token:             token,
		ExpireTimeSeconds: seconds,
		APIBaseURL:        s.cfg.IMAPIBaseURL,
		WSURL:             s.cfg.IMWSURL,
		PlatformID:        platform,
	}
}
