package account

import (
	"context"
	"errors"
	"strings"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/types"
)

// Login 校验账号密码并创建新会话。
func (s *Service) Login(ctx context.Context, in types.LoginRequest) (*types.AuthResp, error) {
	loginName := strings.ToLower(strings.TrimSpace(in.LoginName))
	if loginName == "" || in.Password == "" {
		return nil, errcode.ErrInvalidParam.WithMessage("账号和密码不能为空")
	}
	deviceID, err := domain.NewDeviceID(in.DeviceID)
	if err != nil {
		return nil, errcode.InvalidParam(err)
	}

	fails, err := s.throttle.LoginFailureCount(ctx, loginName)
	if err != nil {
		s.log.Errorf("读取登录失败计数失败: %v", err)
		return nil, errcode.ErrDependency
	}
	if fails >= s.cfg.LoginLockThreshold {
		return nil, errcode.ErrRateLimited.WithMessage("登录失败次数过多，请稍后重试")
	}

	user, err := s.users.GetByLoginName(ctx, loginName)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.recordLoginFailure(ctx, loginName)
			return nil, errcode.ErrInvalidCredential
		}
		return nil, s.dbErr("查询登录用户", err)
	}
	if err := user.CanAuthenticate(); err != nil {
		if errors.Is(err, domain.ErrAccountFrozen) {
			return nil, errcode.ErrAccountFrozen
		}
		return nil, errcode.ErrInvalidCredential
	}

	cred, err := s.creds.GetByUserID(ctx, user.ID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.log.Errorf("账号缺少密码凭证: user_id=%d", user.ID)
			return nil, errcode.ErrInternal
		}
		return nil, s.dbErr("查询密码凭证", err)
	}

	ok, err := s.verifyPassword(user.ID, cred.PasswordHash, in.Password)
	if err != nil {
		return nil, err
	}
	if !ok {
		s.recordLoginFailure(ctx, loginName)
		return nil, errcode.ErrInvalidCredential
	}

	if err := s.throttle.ClearLoginFailures(ctx, loginName); err != nil {
		s.log.Errorf("清除登录失败计数失败: %v", err)
		return nil, errcode.ErrDependency
	}

	now := s.now()
	var (
		sess   *domain.Session
		pair   TokenPair
		kicked []int64
	)
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.users.TouchLastLogin(ctx, user.ID, now); err != nil {
			return s.dbErr("更新最后登录时间", err)
		}
		var createErr error
		sess, pair, kicked, createErr = s.createSession(ctx, user.ID, sessionMeta{
			Platform:  in.Platform,
			DeviceID:  deviceID,
			UserAgent: in.UserAgent,
			IP:        in.IP,
		}, "", nil, now)
		if createErr != nil {
			return createErr
		}
		// 登录成功即登记设备。
		return s.touchDevice(ctx, user.ID, in.Platform, deviceID, in.AppVersion, now)
	})
	if err != nil {
		return nil, err
	}
	if err := s.activateSession(ctx, sess, kicked); err != nil {
		return nil, err
	}
	return s.authResp(user, pair), nil
}
