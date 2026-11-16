package account

import (
	"context"
	"errors"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/types"
)

// Register 注册账号并直接签发令牌。
func (s *Service) Register(ctx context.Context, in types.RegisterRequest) (*types.AuthResp, error) {
	loginName, err := domain.NewLoginName(in.LoginName)
	if err != nil {
		return nil, errcode.InvalidParam(err)
	}
	nickname, err := domain.NewNickname(in.Nickname, loginName)
	if err != nil {
		return nil, errcode.InvalidParam(err)
	}
	plainPassword, err := domain.NewPassword(in.Password)
	if err != nil {
		return nil, errcode.InvalidParam(err)
	}
	deviceID, err := domain.NewDeviceID(in.DeviceID)
	if err != nil {
		return nil, errcode.InvalidParam(err)
	}

	now := s.now()
	hash, err := s.hashPassword(plainPassword)
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		LoginName: loginName,
		Nickname:  nickname,
		Status:    domain.StatusNormal,
		CreatedAt: now,
		UpdatedAt: now,
	}

	var (
		sess   *domain.Session
		pair   TokenPair
		kicked []int64
	)
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.users.Create(ctx, user); err != nil {
			if errors.Is(err, domain.ErrDuplicate) {
				return errcode.ErrAccountExists
			}
			return s.dbErr("注册写入用户", err)
		}
		cred := &domain.Credential{UserID: user.ID, PasswordHash: hash, UpdatedAt: now}
		if err := s.creds.Create(ctx, cred); err != nil {
			if errors.Is(err, domain.ErrDuplicate) {
				return errcode.ErrAccountExists
			}
			return s.dbErr("注册写入凭证", err)
		}
		// 与用户、凭证、会话同事务写入：账号创建成功即保证补偿任务不会丢。
		if err := s.enqueueIMOutbox(ctx, user.ID); err != nil {
			return s.dbErr("注册写入 IM outbox", err)
		}
		var createErr error
		sess, pair, kicked, createErr = s.createSession(ctx, user.ID, sessionMeta{
			Platform:  in.Platform,
			DeviceID:  deviceID,
			UserAgent: in.UserAgent,
			IP:        in.IP,
		}, "", nil, now)
		return createErr
	})
	if err != nil {
		return nil, err
	}
	if err := s.activateSession(ctx, sess, kicked); err != nil {
		return nil, err
	}
	// IM 开通失败不影响注册结果；未配置 IM 时跳过，用户保持 Pending。
	// 同步开通仍保留以缩短正常路径的等待，结果同时落定 outbox 供后台补偿。
	if s.im != nil && s.im.Enabled() {
		settleCtx := context.WithoutCancel(ctx)
		provisionErr := s.provisionIM(settleCtx, user)
		s.imSync.Settle(settleCtx, user.ID, provisionErr)
	}
	return s.authResp(user, pair), nil
}
