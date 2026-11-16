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

// RetryIMProvision 重新为指定用户开通 IM 账号，供上层在开通失败后重试。
func (s *Service) RetryIMProvision(ctx context.Context, userID int64) error {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return errcode.ErrNotFound
		}
		return s.dbErr("查询 IM 开通用户", err)
	}
	if s.im == nil || !s.im.Enabled() {
		return errcode.ErrDependency.WithMessage("IM 未启用")
	}
	// 已开通的用户重复调用会让 IM 返回“已注册”，从而把 Ready 误写回 Failed。
	if user.IMStatus == domain.IMStatusReady {
		return nil
	}
	if err := s.provisionIM(ctx, user); err != nil {
		return errcode.ErrDependency
	}
	return nil
}

// provisionIM 在 IM 侧开通用户并回写开通状态；失败只记录日志并返回错误，由调用方决定影响范围。
func (s *Service) provisionIM(ctx context.Context, user *domain.User) error {
	// 客户端断开不应导致已发出的开通请求被误判失败，回写状态同样需要继续执行。
	provisionCtx := context.WithoutCancel(ctx)
	userID := convert.ToString(user.ID)

	if err := s.im.RegisterUser(provisionCtx, userID, user.Nickname); err != nil {
		s.log.Warnf("IM 开通失败: user_id=%d, err=%v", user.ID, err)
		if writeErr := s.users.UpdateIMStatus(provisionCtx, user.ID, domain.IMStatusFailed, nil, s.now()); writeErr != nil {
			s.log.Errorf("回写 IM 开通状态失败: user_id=%d, status=%d, err=%v", user.ID, domain.IMStatusFailed, writeErr)
		}
		return err
	}

	syncedAt := s.now()
	if err := s.users.UpdateIMStatus(provisionCtx, user.ID, domain.IMStatusReady, &syncedAt, syncedAt); err != nil {
		s.log.Errorf("回写 IM 开通状态失败: user_id=%d, status=%d, err=%v", user.ID, domain.IMStatusReady, err)
	}
	return nil
}

// IMToken 为指定用户与平台兑换 IM token，命中缓存时直接返回。
func (s *Service) IMToken(ctx context.Context, userID int64, platform int8) (*types.IMTokenResp, error) {
	if s.im == nil || !s.im.Enabled() {
		return nil, errcode.ErrDependency.WithMessage("IM 未启用")
	}
	if platform < 1 || platform == 10 || platform > 11 {
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
