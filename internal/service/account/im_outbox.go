package account

import (
	"context"

	"goim-business/internal/service/ports"
)

// enqueueIMOutbox 在注册事务内写入 IM 开通补偿任务；IM 未启用时跳过。
func (s *Service) enqueueIMOutbox(ctx context.Context, userID int64) error {
	if s.outbox == nil || s.im == nil || !s.im.Enabled() {
		return nil
	}
	return s.outbox.Enqueue(ctx, userID, ports.OutboxActionProvisionIM, "")
}
