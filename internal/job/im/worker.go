// Package im 负责账号与 IM 侧的异步同步：账号开通、状态回写与 outbox 补偿。
// 与账号域服务解耦，供 API（注册后的即时开通）与 cron 进程（后台补偿）共用。
package im

import (
	"context"
	"time"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/pkg/convert"
	"goim-business/internal/pkg/logger"
	"goim-business/internal/pkg/text"
	"goim-business/internal/service/ports"
)

const (
	// maxBackoff 是补偿任务重试退避的上限。
	maxBackoff = time.Hour
	// errorLimit 与 account_im_outbox.last_error 的列长度保持一致。
	errorLimit = 512
	// defaultBatchSize 是未配置批量大小时的兜底值。
	defaultBatchSize = 20
)

// Worker 处理 IM 账号开通与 outbox 补偿，只依赖用户仓储、IM 客户端与 outbox 仓储。
// API 通过账号服务注入使用；cron 进程可独立构造，无需装配会话、令牌、限流等组件。
type Worker struct {
	users     domain.UserRepository
	im        ports.IMClient
	outbox    domain.IMOutboxRepository
	batchSize int
	now       func() time.Time
	log       logger.Logger
}

// NewWorker 构造 IM 开通 worker。
func NewWorker(
	users domain.UserRepository,
	im ports.IMClient,
	outbox domain.IMOutboxRepository,
	batchSize int,
	now func() time.Time,
	log logger.Logger,
) *Worker {
	if now == nil {
		now = time.Now
	}
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	return &Worker{
		users:     users,
		im:        im,
		outbox:    outbox,
		batchSize: batchSize,
		now:       now,
		log:       log,
	}
}

// Provision 在 IM 侧开通用户并回写开通状态；失败只记录日志并返回错误，由调用方决定影响范围。
func (w *Worker) Provision(ctx context.Context, user *domain.User) error {
	// 客户端断开不应导致已发出的开通请求被误判失败，回写状态同样需要继续执行。
	provisionCtx := context.WithoutCancel(ctx)
	userID := convert.ToString(user.ID)

	if err := w.im.RegisterUser(provisionCtx, userID, user.Nickname); err != nil {
		w.log.Warnf("IM 开通失败: user_id=%d, err=%v", user.ID, err)
		if writeErr := w.users.UpdateIMStatus(provisionCtx, user.ID, domain.IMStatusFailed, nil, w.now()); writeErr != nil {
			w.log.Errorf("回写 IM 开通状态失败: user_id=%d, status=%d, err=%v", user.ID, domain.IMStatusFailed, writeErr)
		}
		return err
	}

	syncedAt := w.now()
	if err := w.users.UpdateIMStatus(provisionCtx, user.ID, domain.IMStatusReady, &syncedAt, syncedAt); err != nil {
		w.log.Errorf("回写 IM 开通状态失败: user_id=%d, status=%d, err=%v", user.ID, domain.IMStatusReady, err)
	}
	return nil
}

// Settle 在注册的同步开通结束后落定唯一一条 provision_im 任务：
// 成功标记完成，失败按退避策略安排重试。异步补偿由 RunDue 接管。
func (w *Worker) Settle(ctx context.Context, userID int64, provisionErr error) {
	if w.outbox == nil {
		return
	}
	entry, err := w.outbox.FindPending(ctx, userID, domain.OutboxActionProvisionIM)
	if err != nil {
		w.log.Errorf("查询 IM outbox 失败: user_id=%d, err=%v", userID, err)
		return
	}
	if entry == nil {
		return
	}
	if provisionErr == nil {
		if err := w.outbox.MarkDone(ctx, entry.ID); err != nil {
			w.log.Errorf("标记 IM outbox 完成失败: id=%d, user_id=%d, err=%v", entry.ID, userID, err)
		}
		return
	}
	w.retry(ctx, *entry, provisionErr)
}

// RunDue 扫描并补偿一批到期的 IM 开通任务，返回本轮处理的条数。
// 单条失败只记日志，不中断整批；IM 未启用时直接返回 0。
func (w *Worker) RunDue(ctx context.Context) int {
	if w.im == nil || !w.im.Enabled() || w.outbox == nil {
		return 0
	}

	entries, err := w.outbox.ListDue(ctx, w.batchSize)
	if err != nil {
		w.log.Errorf("扫描 IM outbox 失败: %v", err)
		return 0
	}
	if len(entries) == 0 {
		return 0
	}
	// 只在有积压时打印，避免空轮询刷屏。
	w.log.Infof("IM outbox 待补偿: count=%d", len(entries))

	processed := 0
	for _, entry := range entries {
		if ctx.Err() != nil {
			break
		}
		processed++
		w.processEntry(ctx, entry)
	}
	return processed
}

// processEntry 处理单条任务：已 Ready 的用户跳过 goim 调用直接标记完成。
func (w *Worker) processEntry(ctx context.Context, entry domain.OutboxEntry) {
	if entry.Action != domain.OutboxActionProvisionIM {
		w.log.Warnf("未知 IM outbox 任务: id=%d, action=%s", entry.ID, entry.Action)
		return
	}
	// 状态回写不受优雅退出时的取消影响，保证已发出的开通请求有明确结论。
	writeCtx := context.WithoutCancel(ctx)

	user, err := w.users.GetByID(writeCtx, entry.UserID)
	if err != nil {
		w.retry(writeCtx, entry, err)
		return
	}
	// 已开通成功（例如同步调用成功但标记失败）时不重复调用 goim。
	if user.IMStatus == domain.IMStatusReady {
		if err := w.outbox.MarkDone(writeCtx, entry.ID); err != nil {
			w.log.Errorf("标记 IM outbox 完成失败: id=%d, user_id=%d, err=%v", entry.ID, entry.UserID, err)
		}
		return
	}

	if err := w.Provision(writeCtx, user); err != nil {
		w.retry(writeCtx, entry, err)
		return
	}
	if err := w.outbox.MarkDone(writeCtx, entry.ID); err != nil {
		w.log.Errorf("标记 IM outbox 完成失败: id=%d, user_id=%d, err=%v", entry.ID, entry.UserID, err)
	}
}

// retry 记录一次补偿失败并安排下次执行时间。
func (w *Worker) retry(ctx context.Context, entry domain.OutboxEntry, cause error) {
	attempts := entry.Attempts + 1
	nextRetryAt := w.now().Add(backoff(attempts))
	errText := text.TruncateRunes(cause.Error(), errorLimit)
	if err := w.outbox.MarkRetry(ctx, entry.ID, attempts, errText, nextRetryAt); err != nil {
		w.log.Errorf("标记 IM outbox 重试失败: id=%d, user_id=%d, err=%v", entry.ID, entry.UserID, err)
	}
	w.log.Warnf("IM outbox 补偿失败: id=%d, user_id=%d, attempts=%d, next_retry_at=%s, err=%v",
		entry.ID, entry.UserID, attempts, nextRetryAt.Format(time.RFC3339), cause)
}

// backoff 返回第 attempts 次失败后的退避时长：2^attempts 分钟，上限 1 小时。
// attempts 从 1 起算，即首次失败等待 2 分钟、第二次 4 分钟，依次递增。
func backoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	// 2^6 分钟已超过 1 小时上限，避免继续左移造成溢出。
	if attempts >= 6 {
		return maxBackoff
	}
	delay := time.Duration(1<<attempts) * time.Minute
	if delay > maxBackoff {
		return maxBackoff
	}
	return delay
}
