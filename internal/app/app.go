// Package app 提供进程级应用容器：作为组合根统一装配配置、连接、仓储与业务服务，
// 并负责资源释放，供 handler 层注入使用。
package app

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"goim-business/internal/cache"
	"goim-business/internal/config"
	"goim-business/internal/pkg/ratelimit"
	"goim-business/internal/pkg/token"
	"goim-business/internal/repository"
	accountrepo "goim-business/internal/repository/account"
	"goim-business/internal/service/account"
)

// App 是进程级应用容器：持有配置、基础设施连接与业务服务。
type App struct {
	Config *config.Config
	Logger *zap.Logger

	DB    *gorm.DB
	SQLDB *sql.DB
	Redis *redis.Client

	// KeyPrefix 是 Redis 键前缀（含环境隔离）。
	KeyPrefix string

	// 仓储层。
	Tx          *repository.TxManager
	Users       *accountrepo.UserRepo
	Credentials *accountrepo.CredentialRepo
	Sessions    *accountrepo.SessionRepo

	// 基础设施。
	SessionCache *cache.SessionCache
	TokenIssuer  *token.Issuer
	RateLimiter  *ratelimit.Limiter

	// 业务服务。
	AccountService *account.Service
}

// NewApp 装配数据库、Redis、仓储、缓存与业务服务。
func NewApp(ctx context.Context, cfg *config.Config, log *zap.Logger) (*App, error) {
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

	keyPrefix := fmt.Sprintf("%s:%s", cfg.Redis.KeyPrefix, cfg.Env)

	a := &App{
		Config:       cfg,
		Logger:       log,
		DB:           db,
		SQLDB:        sqlDB,
		Redis:        rdb,
		KeyPrefix:    keyPrefix,
		Tx:           repository.NewTxManager(db),
		Users:        accountrepo.NewUserRepo(db),
		Credentials:  accountrepo.NewCredentialRepo(db),
		Sessions:     accountrepo.NewSessionRepo(db),
		SessionCache: cache.NewSessionCache(rdb, keyPrefix),
		TokenIssuer: token.NewIssuer(
			cfg.Auth.JWTSecret,
			cfg.Auth.Issuer,
			cfg.Auth.Audience,
			cfg.Auth.AccessTTL,
		),
		RateLimiter: ratelimit.New(rdb, keyPrefix),
	}

	a.AccountService = account.NewService(
		a.Users,
		a.Credentials,
		a.Sessions,
		a.Tx,
		a.SessionCache,
		a.SessionCache,
		a.TokenIssuer,
		account.Config{
			AccessTTL:              cfg.Auth.AccessTTL,
			RefreshTTL:             cfg.Auth.RefreshTTL,
			MaxSessionsPerUser:     cfg.Auth.MaxSessionsPerUser,
			MaxSessionsPerPlatform: cfg.Auth.MaxSessionsPerPlatform,
			LoginLockThreshold:     cfg.Auth.LoginLockThreshold,
			LoginLockWindow:        cfg.Auth.LoginLockWindow,
		},
		log.Sugar(),
	)
	return a, nil
}

// Close 释放连接资源。
func (a *App) Close() {
	if a.Redis != nil {
		if err := a.Redis.Close(); err != nil {
			a.Logger.Warn("关闭 Redis 失败", zap.Error(err))
		}
	}
	if a.SQLDB != nil {
		if err := a.SQLDB.Close(); err != nil {
			a.Logger.Warn("关闭 MySQL 失败", zap.Error(err))
		}
	}
}
