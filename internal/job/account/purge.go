// Package account 负责账号生命周期的异步维护：冷却期到期后物理清除注销账号。
package account

import (
	"context"
	"fmt"
	"time"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/pkg/logger"
	"goim-business/internal/service/ports"
)

const (
	// purgeJobName 是注销账号清除任务的唯一名称，同时作为分布式锁键后缀。
	purgeJobName = "account_purge"
	// purgeJobTimeout 必须小于 cron 锁的 TTL（见 app.NewCronApp 的 cronLockTTL）。
	purgeJobTimeout = 4 * time.Minute
	// defaultBatchSize 是未配置批量大小时的兜底值。
	defaultBatchSize = 50
)

// PurgeResult 是一轮清除的统计结果。
type PurgeResult struct {
	Scanned int
	Purged  int
	Failed  int
}

// userPurger 是清除流程需要的用户仓储能力。
type userPurger interface {
	ListDueForPurge(ctx context.Context, now time.Time, limit int) ([]*domain.User, error)
	MarkPurged(ctx context.Context, userID int64, loginName string, at time.Time) (bool, error)
}

// credentialPurger 是清除流程需要的凭证仓储能力。
type credentialPurger interface {
	DeleteByUser(ctx context.Context, userID int64) error
}

// sessionPurger 是清除流程需要的会话仓储能力。
type sessionPurger interface {
	DeleteByUser(ctx context.Context, userID int64) (int64, error)
}

// devicePurger 是清除流程需要的设备仓储能力。
type devicePurger interface {
	DeleteByUser(ctx context.Context, userID int64) (int64, error)
}

// filePurger 是清除流程需要的文件仓储能力。
type filePurger interface {
	MarkDeletedByUser(ctx context.Context, userID int64, at time.Time) (int64, error)
}

// Purger 物理清除冷却期已到的注销账号：清空资料、删除凭证/会话/设备，并把文件置为删除。
type Purger struct {
	users     userPurger
	creds     credentialPurger
	sessions  sessionPurger
	devices   devicePurger
	files     filePurger
	tx        ports.TxManager
	batchSize int
	now       func() time.Time
	log       logger.Logger
}

// NewPurger 构造注销账号清除器。
func NewPurger(
	users userPurger,
	creds credentialPurger,
	sessions sessionPurger,
	devices devicePurger,
	files filePurger,
	tx ports.TxManager,
	batchSize int,
	now func() time.Time,
	log logger.Logger,
) *Purger {
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	if now == nil {
		now = time.Now
	}
	return &Purger{
		users:     users,
		creds:     creds,
		sessions:  sessions,
		devices:   devices,
		files:     files,
		tx:        tx,
		batchSize: batchSize,
		now:       now,
		log:       log,
	}
}

// RunOnce 扫描一批到期账号并逐个清除；单条失败只记日志，下一轮重试。
func (p *Purger) RunOnce(ctx context.Context) PurgeResult {
	result := PurgeResult{}
	users, err := p.users.ListDueForPurge(ctx, p.now(), p.batchSize)
	if err != nil {
		p.log.Errorf("扫描到期注销账号失败: %v", err)
		return result
	}
	result.Scanned = len(users)
	for _, user := range users {
		if ctx.Err() != nil {
			break
		}
		purged, err := p.purge(ctx, user)
		if err != nil {
			result.Failed++
			p.log.Warnf("清除注销账号失败: user_id=%d, err=%v", user.ID, err)
			continue
		}
		if purged {
			result.Purged++
		}
	}
	if result.Scanned > 0 {
		p.log.Infof("注销账号清除完成: scanned=%d, purged=%d, failed=%d",
			result.Scanned, result.Purged, result.Failed)
	}
	return result
}

// purge 在单个事务内清除账号数据；MarkPurged 的条件更新保证多实例下只执行一次。
// 返回是否由本次真正清除；false 表示已被其他实例处理。
func (p *Purger) purge(ctx context.Context, user *domain.User) (bool, error) {
	var purged bool
	err := p.tx.WithTx(ctx, func(ctx context.Context) error {
		now := p.now()
		// 用 ~deleted_<id> 释放原登录名，同时保留审计线索；
		// "~" 不在登录名允许字符（a-z0-9_）内，避免与真实用户注册的 deleted_<id> 撞唯一键。
		ok, err := p.users.MarkPurged(ctx, user.ID, fmt.Sprintf("~deleted_%d", user.ID), now)
		if err != nil {
			return err
		}
		if !ok {
			// 已被其他实例处理，无需重复删除数据。
			return nil
		}
		if err := p.creds.DeleteByUser(ctx, user.ID); err != nil {
			return err
		}
		if _, err := p.sessions.DeleteByUser(ctx, user.ID); err != nil {
			return err
		}
		if _, err := p.devices.DeleteByUser(ctx, user.ID); err != nil {
			return err
		}
		// 文件只置为删除，对象由文件清理任务回收。
		if _, err := p.files.MarkDeletedByUser(ctx, user.ID, now); err != nil {
			return err
		}
		purged = true
		return nil
	})
	return purged, err
}

// PurgeJob 把 Purger 的一轮清除包装为定时任务。
type PurgeJob struct {
	purger   *Purger
	interval time.Duration
}

// NewPurgeJob 构造注销账号清除定时任务。
func NewPurgeJob(purger *Purger, interval time.Duration) *PurgeJob {
	if interval <= 0 {
		interval = time.Hour
	}
	return &PurgeJob{purger: purger, interval: interval}
}

// Name 返回任务名。
func (j *PurgeJob) Name() string { return purgeJobName }

// Spec 返回调度表达式；间隔由配置 account.purge_interval 决定。
func (j *PurgeJob) Spec() string { return "@every " + j.interval.String() }

// Timeout 返回单次执行超时。
func (j *PurgeJob) Timeout() time.Duration { return purgeJobTimeout }

// Run 执行一轮到期账号清除。
func (j *PurgeJob) Run(ctx context.Context) error {
	j.purger.RunOnce(ctx)
	return nil
}
