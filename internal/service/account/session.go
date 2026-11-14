package account

import (
	"context"
	"errors"
	"strconv"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/types"
)

// Logout 撤销当前会话。
func (s *Service) Logout(ctx context.Context, userID, sid int64) error {
	sess, err := s.sessions.GetByID(ctx, sid)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return s.dbErr("查询登出会话", err)
	}
	if sess != nil && sess.UserID == userID {
		if err := s.sessions.Revoke(ctx, sid, s.now()); err != nil {
			return s.dbErr("撤销会话", err)
		}
	}
	if err := s.deactivateSession(ctx, sid); err != nil {
		return err
	}
	return nil
}

// Sessions 列出当前用户活跃会话。
func (s *Service) Sessions(ctx context.Context, userID, currentSID int64) ([]types.SessionView, error) {
	sessions, err := s.sessions.ListActive(ctx, userID, s.now())
	if err != nil {
		return nil, s.dbErr("查询活跃会话", err)
	}
	views := make([]types.SessionView, 0, len(sessions))
	for _, item := range sessions {
		views = append(views, types.SessionView{
			SessionID:  strconv.FormatInt(item.ID, 10),
			Platform:   item.Platform,
			DeviceID:   item.DeviceID,
			UserAgent:  item.UserAgent,
			IP:         item.IP,
			CreatedAt:  item.CreatedAt,
			LastSeenAt: item.LastSeenAt,
			ExpiresAt:  item.ExpiresAt,
			Current:    item.ID == currentSID,
		})
	}
	// sort.Slice(views, func(i, j int) bool {
	// 	return views[i].CreatedAt.After(views[j].CreatedAt)
	// })
	return views, nil
}

// RevokeSession 踢出指定会话；仅允许操作本人会话。
func (s *Service) RevokeSession(ctx context.Context, userID, targetSID int64) error {
	if targetSID == 0 {
		return errcode.ErrInvalidParam.WithMessage("sessionId 不能为空")
	}
	sess, err := s.sessions.GetByID(ctx, targetSID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return errcode.ErrNotFound
		}
		return s.dbErr("查询待踢出会话", err)
	}
	if sess.UserID != userID {
		return errcode.ErrNotFound
	}
	if sess.RevokedAt == nil {
		if err := s.sessions.Revoke(ctx, targetSID, s.now()); err != nil {
			return s.dbErr("撤销会话", err)
		}
	}
	if err := s.deactivateSession(ctx, targetSID); err != nil {
		return err
	}
	return nil
}

// ChangePassword 校验原密码后更新密码，并撤销当前会话之外的所有会话。
func (s *Service) ChangePassword(ctx context.Context, userID, currentSID int64, in types.ChangePasswordRequest) error {
	plainPassword, err := domain.NewPassword(in.NewPassword)
	if err != nil {
		return errcode.InvalidParam(err)
	}

	cred, err := s.creds.GetByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return errcode.ErrInternal
		}
		return s.dbErr("查询密码凭证", err)
	}
	ok, err := s.verifyPassword(userID, cred.PasswordHash, in.OldPassword)
	if err != nil {
		return err
	}
	if !ok {
		return errcode.ErrInvalidCredential.WithMessage("原密码错误")
	}

	hash, err := s.hashPassword(plainPassword)
	if err != nil {
		return err
	}
	now := s.now()
	if err := s.creds.UpdatePasswordHash(ctx, userID, hash, now); err != nil {
		return s.dbErr("更新密码", err)
	}

	sids, err := s.sessions.RevokeAll(ctx, userID, now, currentSID)
	if err != nil {
		return s.dbErr("撤销其他会话", err)
	}
	for _, sid := range sids {
		if err := s.deactivateSession(ctx, sid); err != nil {
			return err
		}
	}
	return nil
}
