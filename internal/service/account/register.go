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
	return s.authResp(user, pair), nil
}
