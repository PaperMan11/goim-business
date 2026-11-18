package file

import (
	"context"
	"time"
)

const (
	// cleanupJobName 是过期 pending 文件清理任务的唯一名称，同时作为分布式锁键后缀。
	cleanupJobName = "file_object_cleanup"
	// cleanupJobTimeout 必须小于 cron 锁的 TTL（见 app.NewCronApp 的 cronLockTTL）。
	cleanupJobTimeout = 4 * time.Minute
	// defaultCleanupInterval 是未配置扫描间隔时的兜底值。
	defaultCleanupInterval = time.Hour
)

// CleanupJob 把 Cleaner 的一轮回收包装为定时任务。
type CleanupJob struct {
	cleaner  *Cleaner
	interval time.Duration
}

// NewCleanupJob 构造过期 pending 文件清理任务。
func NewCleanupJob(cleaner *Cleaner, interval time.Duration) *CleanupJob {
	if interval <= 0 {
		interval = defaultCleanupInterval
	}
	return &CleanupJob{cleaner: cleaner, interval: interval}
}

// Name 返回任务名。
func (j *CleanupJob) Name() string { return cleanupJobName }

// Spec 返回调度表达式；间隔由配置 storage.cleanup_interval 决定。
func (j *CleanupJob) Spec() string { return "@every " + j.interval.String() }

// Timeout 返回单次执行超时。
func (j *CleanupJob) Timeout() time.Duration { return cleanupJobTimeout }

// Run 执行一轮过期 pending 文件回收。
func (j *CleanupJob) Run(ctx context.Context) error {
	j.cleaner.RunOnce(ctx)
	return nil
}
