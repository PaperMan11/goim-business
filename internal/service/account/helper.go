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
			continue
		}
		s.kickSessionDevice(ctx, sid)
	}
}

func (s *Service) dbErr(op string, err error) error {
	s.log.Errorf("数据库操作失败: op=%s, err=%v", op, err)
	return errcode.ErrDependency
}

// deactivateSession 清除会话活跃标记并尽力踢出对应 IM 设备。
// 清标记失败按依赖不可用处理；IM 踢出失败只记日志，不回滚业务侧撤销。
func (s *Service) deactivateSession(ctx context.Context, sid int64) error {
	if err := s.state.Deactivate(ctx, convert.ToString(sid)); err != nil {
		s.log.Errorf("清除会话活跃标记失败: sid=%d, err=%v", sid, err)
		return errcode.ErrDependency
	}
	s.kickSessionDevice(ctx, sid)
	return nil
}

// kickSessionDevice 按会话所属平台与设备断开 IM 连接，并清除该平台的 token 缓存，
// 使设备即使重连也必须重新走业务 /im/token（此时该会话已撤销，鉴权中间件会拒绝）。
// goim 的取消失效粒度是设备：同一设备的多条会话（多标签页/同设备重登）共享 deviceID，
// 无法只踢其中一条，所以撤销任一会话都必须踢该设备；同设备仍有效的会话会重新握手，
// 凭仍有效的业务会话换取新 token 后自动恢复，不会被误登出。
func (s *Service) kickSessionDevice(ctx context.Context, sid int64) {
	if s.im == nil {
		return
	}
	sess, err := s.sessions.GetByID(ctx, sid)
	if err != nil || sess == nil {
		s.log.Warnf("查询待踢设备会话失败: sid=%d, err=%v", sid, err)
		return
	}
	userID := convert.ToString(sess.UserID)
	if err := s.im.KickDevice(ctx, userID, sess.Platform, sess.DeviceID); err != nil {
		s.log.Warnf("踢出 IM 设备失败: user_id=%s, platform=%d, device_id=%s, err=%v",
			userID, sess.Platform, sess.DeviceID, err)
	}
	if err := s.imTokens.Delete(ctx, userID, sess.Platform); err != nil {
		s.log.Warnf("清除 IM token 缓存失败: user_id=%s, platform=%d, err=%v", userID, sess.Platform, err)
	}
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
