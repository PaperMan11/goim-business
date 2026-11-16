package account

import (
	"context"
)

// enqueueIMOutbox 在业务事务内写入 IM 同步任务；IM 未启用时跳过。
// action 由调用方指定（provision_im / profile_sync），保证任务与业务数据同事务提交。
func (s *Service) enqueueIMOutbox(ctx context.Context, userID int64, action string) error {
	if s.outbox == nil || s.im == nil || !s.im.Enabled() {
		return nil
	}
	return s.outbox.Enqueue(ctx, userID, action, "")
}
