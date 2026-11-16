package account

import (
	"context"
	"errors"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/pkg/convert"
	"goim-business/internal/pkg/errcode"
)

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
