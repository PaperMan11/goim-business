// Package cron 提供定时任务注册与调度能力：注册 Job 后统一按 cron 表达式调度，
// 每次触发都用 Redis 分布式锁按 Job 名互斥，保证多实例部署时同一任务只有一个在执行。
package cron

import (
	"context"
	"errors"
	"fmt"
	"time"

	robfigcron "github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"goim-business/internal/pkg/lock"
)

// Job 是一个可调度的定时任务。
type Job interface {
	// Name 是任务唯一名称，同时作为分布式锁的键后缀，不得重复。
	Name() string
	// Spec 是 cron 表达式（支持 @every 1m 这类写法）。
	Spec() string
	// Timeout 是单次执行的超时，必须小于锁的 TTL。
	Timeout() time.Duration
	Run(ctx context.Context) error
}

// Scheduler 管理一组 Job，并统一处理调度、互斥锁与优雅退出。
type Scheduler struct {
	scheduler  *robfigcron.Cron
	locker     lock.Locker
	prefix     string
	log        *zap.Logger
	jobs       []Job
	runOnStart bool
}

// NewScheduler 构造调度器；prefix 是锁键前缀（建议形如 "goim:dev:cron"）。
func NewScheduler(locker lock.Locker, prefix string, log *zap.Logger) *Scheduler {
	return &Scheduler{
		scheduler:  robfigcron.New(),
		locker:     locker,
		prefix:     prefix,
		log:        log,
		runOnStart: true,
	}
}

// Register 注册一个 Job；名称重复或表达式非法时返回错误。
func (s *Scheduler) Register(job Job) error {
	for _, registered := range s.jobs {
		if registered.Name() == job.Name() {
			return fmt.Errorf("定时任务名称重复: %s", job.Name())
		}
	}
	if _, err := s.scheduler.AddFunc(job.Spec(), func() { s.run(job) }); err != nil {
		return fmt.Errorf("注册定时任务 %s 失败: %w", job.Name(), err)
	}
	s.jobs = append(s.jobs, job)
	s.log.Info("注册定时任务", zap.String("name", job.Name()), zap.String("spec", job.Spec()))
	return nil
}

// Run 启动调度并阻塞到 ctx 取消；启动时先立即执行每个 Job 一轮，避免重启后空等首个调度点。
func (s *Scheduler) Run(ctx context.Context) {
	if s.runOnStart {
		for _, job := range s.jobs {
			s.run(job)
		}
	}
	s.scheduler.Start()

	<-ctx.Done()
	s.log.Info("cron 停止调度，等待本轮任务结束")
	<-s.scheduler.Stop().Done()
}

// run 执行单个 Job：先抢按任务名隔离的分布式锁，抢不到说明其他实例正在执行，本轮跳过。
func (s *Scheduler) run(job Job) {
	ctx, cancel := context.WithTimeout(context.Background(), job.Timeout())
	defer cancel()

	err := s.locker.ExecWithLock(ctx, s.lockKey(job.Name()), func() error {
		return job.Run(ctx)
	})
	switch {
	case errors.Is(err, lock.ErrLockNotObtained), errors.Is(err, lock.ErrRetryExhausted):
		s.log.Info("其他实例正在执行，本轮跳过", zap.String("job", job.Name()))
	case err != nil:
		s.log.Error("定时任务执行失败", zap.String("job", job.Name()), zap.Error(err))
	}
}

func (s *Scheduler) lockKey(name string) string {
	return s.prefix + ":" + name
}
