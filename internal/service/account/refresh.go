package account

import (
	"context"
	"errors"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/pkg/token"
	"goim-business/internal/types"
)

// Refresh 轮换 refresh token；检测到旧令牌重放时撤销整个家族。
func (s *Service) Refresh(ctx context.Context, in types.RefreshRequest) (*types.AuthResp, error) {
	if in.RefreshToken == "" {
		return nil, errcode.ErrUnauthenticated.WithMessage("缺少 refresh token")
	}

	sess, err := s.sessions.GetByRefreshHash(ctx, token.HashOpaque(in.RefreshToken))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errcode.ErrUnauthenticated
		}
		return nil, s.dbErr("查询刷新会话", err)
	}

	now := s.now()
	if sess.IsReplay() {
		// 旧令牌被再次使用：按重放处理，撤销整族。
		s.revokeFamily(ctx, sess.FamilyID, now)
		return nil, errcode.ErrRefreshReplay
	}
	if !sess.IsActive(now) {
		return nil, errcode.ErrUnauthenticated
	}

	user, err := s.users.GetByID(ctx, sess.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errcode.ErrUnauthenticated
		}
		return nil, s.dbErr("查询刷新用户", err)
	}
	if err := user.CanAuthenticate(); err != nil {
		if errors.Is(err, domain.ErrAccountFrozen) {
			return nil, errcode.ErrAccountFrozen
		}
		return nil, errcode.ErrUnauthenticated
	}

	var (
		newSess *domain.Session
		pair    TokenPair
		kicked  []int64
		replay  bool
	)
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		// 与注销的 MarkDeleting 用同一用户行锁串行化，避免刷新在注销后创建新会话。
		locked, err := s.users.GetByIDForUpdate(ctx, user.ID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return errcode.ErrUnauthenticated
			}
			return s.dbErr("锁定刷新用户", err)
		}
		if err := locked.CanAuthenticate(); err != nil {
			if errors.Is(err, domain.ErrAccountFrozen) {
				return errcode.ErrAccountFrozen
			}
			return errcode.ErrUnauthenticated
		}
		rotated, err := s.sessions.MarkRotated(ctx, sess.ID, now)
		if err != nil {
			return s.dbErr("标记会话轮换", err)
		}
		if !rotated {
			replay = true
			return nil
		}
		var createErr error
		newSess, pair, kicked, createErr = s.createSession(ctx, user.ID, sessionMeta{
			Platform:  sess.Platform,
			DeviceID:  sess.DeviceID,
			UserAgent: in.UserAgent,
			IP:        in.IP,
		}, sess.FamilyID, &sess.ID, now)
		return createErr
	})
	if err != nil {
		return nil, err
	}
	if replay {
		s.revokeFamily(ctx, sess.FamilyID, now)
		return nil, errcode.ErrRefreshReplay
	}

	if err := s.deactivateSession(ctx, sess.ID); err != nil {
		return nil, err
	}
	if err := s.activateSession(ctx, newSess, kicked); err != nil {
		return nil, err
	}
	return s.authResp(user, pair), nil
}
