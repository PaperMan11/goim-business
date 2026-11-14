package account

import (
	"context"
	"time"

	"goim-business/internal/pkg/convert"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/pkg/password"
)

func (s *Service) recordLoginFailure(ctx context.Context, loginName string) {
	if _, err := s.throttle.IncrLoginFailure(ctx, loginName, s.cfg.LoginLockWindow); err != nil {
		s.log.Warnf("记录登录失败次数失败: login_name=%s, err=%v", loginName, err)
	}
}

func (s *Service) revokeFamily(ctx context.Context, familyID string, at time.Time) {
	sids, err := s.sessions.RevokeFamily(ctx, familyID, at)
	if err != nil {
		s.log.Errorf("撤销刷新家族失败: family_id=%s, err=%v", familyID, err)
		return
	}
	for _, sid := range sids {
		if err := s.state.Deactivate(ctx, convert.ToString(sid)); err != nil {
			s.log.Errorf("清除会话活跃标记失败: sid=%d, err=%v", sid, err)
		}
	}
}

func (s *Service) dbErr(op string, err error) error {
	s.log.Errorf("数据库操作失败: op=%s, err=%v", op, err)
	return errcode.ErrDependency
}

// deactivateSession 清除会话活跃标记，失败按依赖不可用处理。
func (s *Service) deactivateSession(ctx context.Context, sid int64) error {
	if err := s.state.Deactivate(ctx, convert.ToString(sid)); err != nil {
		s.log.Errorf("清除会话活跃标记失败: sid=%d, err=%v", sid, err)
		return errcode.ErrDependency
	}
	return nil
}

// hashPassword 计算密码哈希；失败按内部错误处理。
func (s *Service) hashPassword(plain string) (string, error) {
	hash, err := password.Hash(plain)
	if err != nil {
		s.log.Errorf("密码哈希失败: %v", err)
		return "", errcode.ErrInternal
	}
	return hash, nil
}

// verifyPassword 校验密码；哈希格式非法时按内部错误处理。
func (s *Service) verifyPassword(userID int64, encoded, plain string) (bool, error) {
	ok, err := password.Verify(encoded, plain)
	if err != nil {
		s.log.Errorf("密码哈希格式非法: user_id=%d, err=%v", userID, err)
		return false, errcode.ErrInternal
	}
	return ok, nil
}
