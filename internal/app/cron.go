package app

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	imadapter "goim-business/internal/adapter/im"
	"goim-business/internal/client/goim"
	"goim-business/internal/config"
	accountjob "goim-business/internal/job/account"
	filejob "goim-business/internal/job/file"
	imjob "goim-business/internal/job/im"
	"goim-business/internal/pkg/lock"
	redislock "goim-business/internal/pkg/lock/redis"
	"goim-business/internal/pkg/storage/s3"
	"goim-business/internal/pkg/timex"
	"goim-business/internal/repository"
	accountrepo "goim-business/internal/repository/account"
	filerepo "goim-business/internal/repository/file"
	outboxrepo "goim-business/internal/repository/outbox"
)

// cronLockTTL 是 cron 互斥锁的持有时间；所有定时任务的 Timeout 都必须小于该值。
const cronLockTTL = 5 * time.Minute

// CronApp 是定时任务进程的精简容器：只装配 MySQL、Redis（分布式锁）、goim 客户端、
// outbox worker 与文件对象清理器，不构建会话缓存、令牌签发、限流与 HTTP handler 等 API 专用组件。
type CronApp struct {
	Config *config.Config
	Logger *zap.Logger

	SQLDB *sql.DB
	Redis *redis.Client
	// Storage 为 nil 表示对象存储未配置，文件清理只回收元数据。
	Storage s3.S3Client

	IM            *goim.Client
	Worker        *imjob.Worker
	FileCleaner   *filejob.Cleaner
	AccountPurger *accountjob.Purger
	Locker        lock.Locker
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

	var storageClient s3.S3Client
	if cfg.Storage.Enabled() {
		storageClient, err = s3.NewS3Client(ctx, &s3.Config{
			Endpoint:       cfg.Storage.Endpoint,
			PublicEndpoint: cfg.Storage.PublicEndpoint,
			AccessKey:      cfg.Storage.AccessKey,
			SecretKey:      cfg.Storage.SecretKey,
			Region:         cfg.Storage.Region,
			Bucket:         cfg.Storage.Bucket,
			UsePathStyle:   cfg.Storage.UsePathStyle,
			PresignExpire:  cfg.Storage.PresignExpire,
		})
		if err != nil {
			_ = rdb.Close()
			_ = sqlDB.Close()
			return nil, fmt.Errorf("初始化对象存储失败: %w", err)
		}
	}

	users := accountrepo.NewUserRepo(db)
	files := filerepo.New(db)

	// IM 操作用独立的短 TTL 锁：与 cron 任务锁分开，避免不同任务互相等待。
	imLocker := redislock.NewRedisLocker(
		rdb,
		lock.WithDefaultTTL(15*time.Second),
		lock.WithRetryStrategy(lock.ExponentialBackoff(50*time.Millisecond, 500*time.Millisecond)),
		lock.WithMaxRetries(5),
	)

	return &CronApp{
		Config:  cfg,
		Logger:  log,
		SQLDB:   sqlDB,
		Redis:   rdb,
		Storage: storageClient,
		IM:      im,
		Worker: imjob.NewWorker(users, imadapter.NewClient(im), outboxrepo.New(db), cfg.IM.OutboxBatchSize, timex.Now, log.Sugar(),
			imjob.WithAvatarURL(func(objectKey string) string {
				return cfg.Storage.PublicObjectURL(objectKey)
			}),
			imjob.WithUserLock(imLocker, imjob.UserLockKeyPrefix(fmt.Sprintf("%s:%s", cfg.Redis.KeyPrefix, cfg.Env))),
		),
		FileCleaner: filejob.NewCleaner(files, storageClient, cfg.Storage.CleanupBatchSize, timex.Now, log.Sugar()),
		AccountPurger: accountjob.NewPurger(
			users,
			accountrepo.NewCredentialRepo(db),
			accountrepo.NewSessionRepo(db),
			accountrepo.NewDeviceRepo(db),
			files,
			repository.NewTxManager(db),
			cfg.Account.PurgeBatchSize,
			timex.Now,
			log.Sugar(),
		),
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
