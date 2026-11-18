package im

import (
	"context"
	"time"
)

// outboxJobName 是 IM 开通补偿任务的唯一名称，同时作为分布式锁键后缀。
const outboxJobName = "im_outbox"

// outboxJobTimeout 必须小于 cron 锁的 TTL（见 app.NewCronApp 的 cronLockTTL）。
const outboxJobTimeout = 4 * time.Minute

// OutboxJob 把 Worker 的一轮补偿包装为定时任务。
type OutboxJob struct {
	worker   *Worker
	interval time.Duration
}

// NewOutboxJob 构造 IM 开通补偿定时任务。
func NewOutboxJob(worker *Worker, interval time.Duration) *OutboxJob {
	if interval <= 0 {
		interval = time.Minute
	}
	return &OutboxJob{worker: worker, interval: interval}
}

// Name 返回任务名。
func (j *OutboxJob) Name() string { return outboxJobName }

// Spec 返回调度表达式；间隔由配置 im.outbox_scan_interval 决定。
func (j *OutboxJob) Spec() string {
	return "@every " + j.interval.String()
}

// Timeout 返回单次执行超时。
func (j *OutboxJob) Timeout() time.Duration { return outboxJobTimeout }

// Run 执行一轮 outbox 补偿。
func (j *OutboxJob) Run(ctx context.Context) error {
	j.worker.RunDue(ctx)
	return nil
}
