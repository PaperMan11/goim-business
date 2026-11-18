// Package im 负责账号与 IM 侧的异步同步：账号开通、资料同步、状态回写与 outbox 补偿。
// 与账号域服务解耦，供 API（注册后的即时开通）与 cron 进程（后台补偿）共用。
package im

import (
	"context"
	"errors"
	"strings"
	"time"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/pkg/convert"
	"goim-business/internal/pkg/logger"
	"goim-business/internal/pkg/text"
	"goim-business/internal/service/ports"
)

// errProfileNotReady 表示 IM 账号尚未开通，资料同步需延后重试。
var errProfileNotReady = errors.New("IM 账号尚未开通")

const (
	// maxBackoff 是补偿任务重试退避的上限。
	maxBackoff = time.Hour
	// errorLimit 与 outbox_events.last_error 的列长度保持一致。
	errorLimit = 512
	// defaultBatchSize 是未配置批量大小时的兜底值。
	defaultBatchSize = 20
)

// Worker 处理 IM 账号开通与 outbox 补偿，只依赖用户仓储、IM 客户端与 outbox 仓储。
// API 通过账号服务注入使用；cron 进程可独立构造，无需装配会话、令牌、限流等组件。
type Worker struct {
	users     domain.UserRepository
	im        ports.IMClient
	outbox    ports.OutboxRepository
	batchSize int
	now       func() time.Time
	avatarURL func(string) string
	log       logger.Logger
}

// Option 是 Worker 的可选配置。
type Option func(*Worker)

// WithAvatarURL 注入头像 key 到公开访问 URL 的解析函数。
func WithAvatarURL(resolver func(objectKey string) string) Option {
	return func(w *Worker) {
		w.avatarURL = resolver
	}
}

// NewWorker 构造 IM 开通 worker。
func NewWorker(
	users domain.UserRepository,
	im ports.IMClient,
	outbox ports.OutboxRepository,
	batchSize int,
	now func() time.Time,
	log logger.Logger,
	opts ...Option,
) *Worker {
	if now == nil {
		now = time.Now
	}
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	worker := &Worker{
		users:     users,
		im:        im,
		outbox:    outbox,
		batchSize: batchSize,
		now:       now,
		log:       log,
	}
	for _, opt := range opts {
		opt(worker)
	}
	return worker
}

// Provision 在 IM 侧开通用户并回写开通状态；失败只记录日志并返回错误，由调用方决定影响范围。
func (w *Worker) Provision(ctx context.Context, user *domain.User) error {
	// 客户端断开不应导致已发出的开通请求被误判失败，回写状态同样需要继续执行。
	provisionCtx := context.WithoutCancel(ctx)
	userID := convert.ToString(user.ID)

	registerErr := w.im.RegisterUser(provisionCtx, userID, user.Nickname, w.resolveAvatarURL(user.AvatarKey))
	// IM 侧账号已存在时注册不会更新既有资料（例如账号由其他系统预建或上次响应丢失），
	// 需补一次全量资料同步；同步失败按开通失败处理，交给 outbox 重试。
	if registerErr == nil || errors.Is(registerErr, ports.ErrIMUserAlreadyExists) {
		registerErr = w.SyncProfile(provisionCtx, user)
	}
	if registerErr != nil {
		w.log.Warnf("IM 开通失败: user_id=%d, err=%v", user.ID, registerErr)
		if writeErr := w.users.UpdateIMStatus(provisionCtx, user.ID, domain.IMStatusFailed, nil, w.now()); writeErr != nil {
			w.log.Errorf("回写 IM 开通状态失败: user_id=%d, status=%d, err=%v", user.ID, domain.IMStatusFailed, writeErr)
		}
		return registerErr
	}

	syncedAt := w.now()
	if err := w.users.UpdateIMStatus(provisionCtx, user.ID, domain.IMStatusReady, &syncedAt, syncedAt); err != nil {
		w.log.Errorf("回写 IM 开通状态失败: user_id=%d, status=%d, err=%v", user.ID, domain.IMStatusReady, err)
	}
	return nil
}

// SyncProfile 把当前用户昵称与头像同步到 IM；失败只记录日志并返回错误，由调用方决定影响范围。
func (w *Worker) SyncProfile(ctx context.Context, user *domain.User) error {
	// 客户端断开不应导致已发出的同步请求被误判失败。
	syncCtx := context.WithoutCancel(ctx)
	if err := w.im.UpdateUserInfo(syncCtx, convert.ToString(user.ID), user.Nickname, w.resolveAvatarURL(user.AvatarKey)); err != nil {
		w.log.Warnf("IM 资料同步失败: user_id=%d, err=%v", user.ID, err)
		return err
	}
	return nil
}

// resolveAvatarURL 把头像 key 解析为可直接访问的稳定 URL；已是 http(s) 地址时原样返回。
func (w *Worker) resolveAvatarURL(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if strings.HasPrefix(key, "http://") || strings.HasPrefix(key, "https://") {
		return key
	}
	if w.avatarURL == nil {
		return ""
	}
	return w.avatarURL(key)
}

// Settle 在同步调用结束后落定指定类型唯一一条 outbox 任务：
// 成功标记完成，失败按退避策略安排重试。异步补偿由 RunDue 接管。
func (w *Worker) Settle(ctx context.Context, userID int64, action string, cause error) {
	if w.outbox == nil {
		return
	}
	entry, err := w.outbox.FindPending(ctx, userID, action)
	if err != nil {
		w.log.Errorf("查询 IM outbox 失败: user_id=%d, action=%s, err=%v", userID, action, err)
		return
	}
	if entry == nil {
		return
	}
	if cause == nil {
		if err := w.outbox.MarkDone(ctx, entry.ID); err != nil {
			w.log.Errorf("标记 IM outbox 完成失败: id=%d, user_id=%d, err=%v", entry.ID, userID, err)
		}
		return
	}
	w.retry(ctx, *entry, cause)
}

// RunDue 扫描并补偿一批到期的 IM outbox 任务，返回本轮处理的条数。
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

// processEntry 按任务类型分发：provision_im 开通账号，profile_sync 同步资料。
func (w *Worker) processEntry(ctx context.Context, entry ports.OutboxEntry) {
	switch entry.Action {
	case ports.OutboxActionProvisionIM:
		w.processProvision(ctx, entry)
	case ports.OutboxActionProfileSync:
		w.processProfile(ctx, entry)
	default:
		w.log.Warnf("未知 IM outbox 任务: id=%d, action=%s", entry.ID, entry.Action)
	}
}

// processProvision 处理开通任务：已 Ready 的用户跳过 goim 调用直接标记完成。
func (w *Worker) processProvision(ctx context.Context, entry ports.OutboxEntry) {
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

// processProfile 处理资料同步任务；IM 账号未开通时按退避重试，等待开通任务完成。
func (w *Worker) processProfile(ctx context.Context, entry ports.OutboxEntry) {
	writeCtx := context.WithoutCancel(ctx)

	user, err := w.users.GetByID(writeCtx, entry.UserID)
	if err != nil {
		w.retry(writeCtx, entry, err)
		return
	}
	if user.IMStatus != domain.IMStatusReady {
		w.retry(writeCtx, entry, errProfileNotReady)
		return
	}
	if err := w.SyncProfile(writeCtx, user); err != nil {
		w.retry(writeCtx, entry, err)
		return
	}
	if err := w.outbox.MarkDone(writeCtx, entry.ID); err != nil {
		w.log.Errorf("标记 IM outbox 完成失败: id=%d, user_id=%d, err=%v", entry.ID, entry.UserID, err)
	}
}

// retry 记录一次补偿失败并安排下次执行时间。
func (w *Worker) retry(ctx context.Context, entry ports.OutboxEntry, cause error) {
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
