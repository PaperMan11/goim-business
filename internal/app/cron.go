package app

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"goim-business/internal/client/goim"
	"goim-business/internal/config"
	imjob "goim-business/internal/job/im"
	"goim-business/internal/pkg/lock"
	redislock "goim-business/internal/pkg/lock/redis"
	"goim-business/internal/pkg/timex"
	"goim-business/internal/repository"
	accountrepo "goim-business/internal/repository/account"
	outboxrepo "goim-business/internal/repository/outbox"
)

// cronLockTTL 是 cron 互斥锁的持有时间；所有定时任务的 Timeout 都必须小于该值。
const cronLockTTL = 5 * time.Minute

// CronApp 是定时任务进程的精简容器：只装配 MySQL、Redis（分布式锁）、goim 客户端与 outbox worker，
// 不构建会话缓存、令牌签发、限流与 HTTP handler 等 API 专用组件。
type CronApp struct {
	Config *config.Config
	Logger *zap.Logger

	SQLDB *sql.DB
	Redis *redis.Client

	IM     *goim.Client
	Worker *imjob.Worker
	Locker lock.Locker
	// LockPrefix 是 cron 锁键前缀，调度器会拼接任务名得到最终键。
	LockPrefix string
}

// NewCronApp 装配定时任务进程所需的最小依赖集合。
func NewCronApp(ctx context.Context, cfg *config.Config, log *zap.Logger) (*CronApp, error) {
	db, sqlDB, err := repository.NewDB(ctx, cfg.MySQL, log)
	if err != nil {
		return nil, fmt.Errorf("连接 MySQL 失败: %w", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		_ = sqlDB.Close()
		return nil, fmt.Errorf("连接 Redis 失败: %w", err)
	}

	im := goim.New(goim.Config{
		BaseURL:           cfg.IM.BaseURL,
		AdminUserID:       cfg.IM.AdminUserID,
		AdminSecret:       cfg.IM.AdminSecret,
		Timeout:           cfg.IM.Timeout,
		TokenRefreshAhead: cfg.IM.TokenRefreshAhead,
	})

	return &CronApp{
		Config: cfg,
		Logger: log,
		SQLDB:  sqlDB,
		Redis:  rdb,
		IM:     im,
		Worker: imjob.NewWorker(accountrepo.NewUserRepo(db), imClient{client: im}, outboxrepo.New(db), cfg.IM.OutboxBatchSize, timex.Now, log.Sugar(),
			imjob.WithAvatarURL(func(objectKey string) string {
				return cfg.Storage.PublicObjectURL(objectKey)
			})),
		Locker:     redislock.NewRedisLocker(rdb, lock.WithDefaultTTL(cronLockTTL), lock.WithRetryStrategy(lock.NoRetry())),
		LockPrefix: fmt.Sprintf("%s:%s:cron", cfg.Redis.KeyPrefix, cfg.Env),
	}, nil
}

// Close 释放 MySQL 与 Redis 连接。
func (c *CronApp) Close() {
	if c.Redis != nil {
		if err := c.Redis.Close(); err != nil {
			c.Logger.Warn("关闭 Redis 失败", zap.Error(err))
		}
	}
	if c.SQLDB != nil {
		if err := c.SQLDB.Close(); err != nil {
			c.Logger.Warn("关闭 MySQL 失败", zap.Error(err))
		}
	}
}
