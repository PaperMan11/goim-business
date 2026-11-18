package account

import (
	"context"
	"errors"
	"time"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/pkg/convert"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/pkg/idgen"
	"goim-business/internal/pkg/text"
	"goim-business/internal/pkg/token"
)

// createSession 生成 refresh token 并落库（sid 由数据库自增分配），同时执行会话上限检查。
func (s *Service) createSession(
	ctx context.Context,
	userID int64,
	meta sessionMeta,
	familyID string,
	rotatedFrom *int64,
	now time.Time,
) (*domain.Session, TokenPair, []int64, error) {
	refreshToken, hash, err := token.NewOpaque()
	if err != nil {
		s.log.Errorf("生成 refresh token 失败: %v", err)
		return nil, TokenPair{}, nil, errcode.ErrInternal
	}
	if familyID == "" {
		familyID, err = idgen.NewWithPrefix(idgen.PrefixAccount)
		if err != nil {
			s.log.Errorf("生成刷新家族 ID 失败: %v", err)
			return nil, TokenPair{}, nil, errcode.ErrInternal
		}
	}

	sess := &domain.Session{
		UserID:      userID,
		FamilyID:    familyID,
		RefreshHash: hash,
		RotatedFrom: rotatedFrom,
		Platform:    meta.Platform,
		DeviceID:    meta.DeviceID,
		UserAgent:   text.TruncateRunes(meta.UserAgent, 255),
		IP:          text.TruncateRunes(meta.IP, 45),
		ExpiresAt:   now.Add(s.cfg.RefreshTTL),
		CreatedAt:   now,
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return nil, TokenPair{}, nil, s.dbErr("写入会话", err)
	}

	accessToken, accessExp, err := s.tokens.Issue(
		convert.ToString(userID),
		convert.ToString(sess.ID),
		meta.DeviceID,
		int(meta.Platform),
		[]string{"user"},
	)
	if err != nil {
		s.log.Errorf("签发 access token 失败: %v", err)
		return nil, TokenPair{}, nil, errcode.ErrInternal
	}

	kicked, err := s.enforceSessionLimits(ctx, userID, sess.ID, meta.Platform, now)
	if err != nil {
		return nil, TokenPair{}, nil, err
	}

	return sess, TokenPair{
		AccessToken:      accessToken,
		AccessExpiresAt:  accessExp,
		RefreshToken:     refreshToken,
		RefreshExpiresAt: sess.ExpiresAt,
		SessionID:        sess.ID,
		Platform:         meta.Platform,
	}, kicked, nil
}

// enforceSessionLimits 超出每用户/每平台上限时踢出最旧会话（当前会话除外）。
func (s *Service) enforceSessionLimits(ctx context.Context, userID, currentSID int64, platform int8, now time.Time) ([]int64, error) {
	active, err := s.sessions.ListActive(ctx, userID, now)
	if err != nil {
		return nil, s.dbErr("查询活跃会话", err)
	}

	kicked := domain.SelectKicked(active, currentSID, platform, s.cfg.MaxSessionsPerUser, s.cfg.MaxSessionsPerPlatform)
	for _, sid := range kicked {
		if err := s.sessions.Revoke(ctx, sid, now); err != nil {
			return nil, s.dbErr("踢出超额会话", err)
		}
	}
	return kicked, nil
}

// activateSession 写入新会话的活跃标记，并清理被踢会话的标记；失败按依赖不可用处理（安全优先）。
func (s *Service) activateSession(ctx context.Context, sess *domain.Session, kicked []int64) error {
	for _, sid := range kicked {
		if err := s.deactivateSession(ctx, sid); err != nil {
			return err
		}
	}
	ttl := sess.ExpiresAt.Sub(s.now())
	if ttl <= 0 {
		return errcode.ErrInternal
	}
	if err := s.state.Activate(ctx, convert.ToString(sess.ID), convert.ToString(sess.UserID), ttl); err != nil {
		s.log.Errorf("写入会话活跃标记失败: sid=%d, err=%v", sess.ID, err)
		return errcode.ErrDependency
	}
	// 激活后复核会话是否已被并发注销撤销：若注销的 RevokeAll 恰好落在会话提交与激活之间，
	// 这里会把刚写入的活跃标记删掉并终止请求，避免返回一个已被撤销的登录态。
	fresh, err := s.sessions.GetByID(ctx, sess.ID)
	if err != nil {
		_ = s.state.Deactivate(ctx, convert.ToString(sess.ID))
		if errors.Is(err, domain.ErrNotFound) {
			return errcode.ErrUnauthenticated
		}
		return s.dbErr("复核会话状态", err)
	}
	if !fresh.IsActive(s.now()) {
		_ = s.state.Deactivate(ctx, convert.ToString(sess.ID))
		return errcode.ErrUnauthenticated.WithMessage("登录态已失效，请重新登录")
	}
	return nil
}
