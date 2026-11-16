package account

import (
	"context"

	domain "goim-business/internal/domain/account"
)

// enqueueIMOutbox 在注册事务内写入 IM 开通补偿任务；IM 未启用时跳过。
func (s *Service) enqueueIMOutbox(ctx context.Context, userID int64) error {
	if s.outbox == nil || s.im == nil || !s.im.Enabled() {
		return nil
	}
	return s.outbox.Enqueue(ctx, userID, domain.OutboxActionProvisionIM, "")
}

// ProcessIMOutbox 委托 IMProvisioner 执行一轮补偿；保留该方法供注册后立即处理与测试使用。
func (s *Service) ProcessIMOutbox(ctx context.Context) int {
	return s.imSync.RunDue(ctx)
}
