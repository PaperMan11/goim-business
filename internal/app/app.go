// Package app 提供进程级应用容器：作为组合根统一装配配置、连接、仓储与业务服务，
// 并负责资源释放，供 handler 层注入使用。
package app

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"goim-business/internal/cache"
	"goim-business/internal/client/goim"
	"goim-business/internal/config"
	imjob "goim-business/internal/job/im"
	"goim-business/internal/pkg/ratelimit"
	"goim-business/internal/pkg/timex"
	"goim-business/internal/pkg/token"
	"goim-business/internal/repository"
	accountrepo "goim-business/internal/repository/account"
	clientconfigrepo "goim-business/internal/repository/clientconfig"
	outboxrepo "goim-business/internal/repository/outbox"
	"goim-business/internal/service/account"
	clientconfigsvc "goim-business/internal/service/clientconfig"
	"goim-business/internal/service/ports"
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
	Devices     *accountrepo.DeviceRepo
	Outbox      *outboxrepo.Repo

	// ClientConfigs 是客户端配置仓储。
	ClientConfigs *clientconfigrepo.Repo

	// 基础设施。
	SessionCache *cache.SessionCache
	IMTokenCache *cache.IMTokenCache
	TokenIssuer  *token.Issuer
	RateLimiter  *ratelimit.Limiter
	IM           *goim.Client

	// 业务服务。
	AccountService      *account.Service
	ClientConfigService *clientconfigsvc.Service
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
		Config:        cfg,
		Logger:        log,
		DB:            db,
		SQLDB:         sqlDB,
		Redis:         rdb,
		KeyPrefix:     keyPrefix,
		Tx:            repository.NewTxManager(db),
		Users:         accountrepo.NewUserRepo(db),
		Credentials:   accountrepo.NewCredentialRepo(db),
		Sessions:      accountrepo.NewSessionRepo(db),
		Devices:       accountrepo.NewDeviceRepo(db),
		Outbox:        outboxrepo.New(db),
		ClientConfigs: clientconfigrepo.New(db),
		SessionCache:  cache.NewSessionCache(rdb, keyPrefix),
		IMTokenCache:  cache.NewIMTokenCache(rdb, keyPrefix),
		TokenIssuer: token.NewIssuer(
			cfg.Auth.JWTSecret,
			cfg.Auth.Issuer,
			cfg.Auth.Audience,
			cfg.Auth.AccessTTL,
		),
		RateLimiter: ratelimit.New(rdb, keyPrefix),
		IM: goim.New(goim.Config{
			BaseURL:           cfg.IM.BaseURL,
			AdminUserID:       cfg.IM.AdminUserID,
			AdminSecret:       cfg.IM.AdminSecret,
			Timeout:           cfg.IM.Timeout,
			TokenRefreshAhead: cfg.IM.TokenRefreshAhead,
		}),
	}
	if !a.IM.Enabled() {
		log.Warn("IM 未配置，注册跳过开通")
	}

	// api_base_url 未显式配置时，按 im-api 的统一前缀从 base_url 推导。
	imAPIBaseURL := strings.TrimSpace(cfg.IM.APIBaseURL)
	if imAPIBaseURL == "" && cfg.IM.BaseURL != "" {
		imAPIBaseURL = strings.TrimRight(cfg.IM.BaseURL, "/") + "/api/v1"
	}

	a.AccountService = account.NewService(
		a.Users,
		a.Credentials,
		a.Sessions,
		a.Devices,
		a.Tx,
		a.SessionCache,
		a.SessionCache,
		imClient{client: a.IM},
		a.IMTokenCache,
		a.Outbox,
		imjob.NewWorker(a.Users, imClient{client: a.IM}, a.Outbox, cfg.IM.OutboxBatchSize, timex.Now, log.Sugar()),
		a.TokenIssuer,
		account.Config{
			AccessTTL:              cfg.Auth.AccessTTL,
			RefreshTTL:             cfg.Auth.RefreshTTL,
			MaxSessionsPerUser:     cfg.Auth.MaxSessionsPerUser,
			MaxSessionsPerPlatform: cfg.Auth.MaxSessionsPerPlatform,
			LoginLockThreshold:     cfg.Auth.LoginLockThreshold,
			LoginLockWindow:        cfg.Auth.LoginLockWindow,
			IMAPIBaseURL:           imAPIBaseURL,
			IMWSURL:                cfg.IM.WSURL,
		},
		log.Sugar(),
	)

	a.ClientConfigService = clientconfigsvc.NewService(
		a.ClientConfigs,
		clientconfigsvc.Config{IMAPIBaseURL: imAPIBaseURL, IMWSURL: cfg.IM.WSURL},
		log.Sugar(),
	)
	return a, nil
}

// imClient 把 goim 客户端适配为账号域的 IM 端口。
type imClient struct {
	client *goim.Client
}

// Enabled 表示 IM 是否已配置。
func (c imClient) Enabled() bool { return c.client.Enabled() }

// RegisterUser 在 IM 侧开通用户。
func (c imClient) RegisterUser(ctx context.Context, userID, nickname string) error {
	return c.client.RegisterUser(ctx, goim.UserInfo{UserID: userID, Nickname: nickname})
}

// UpdateUserInfo 把昵称与头像同步到 IM。
func (c imClient) UpdateUserInfo(ctx context.Context, userID, nickname, avatarURL string) error {
	return c.client.UpdateUserInfo(ctx, goim.UserInfo{UserID: userID, Nickname: nickname, AvatarURL: avatarURL})
}

// UserToken 为指定用户与平台兑换 IM token。
func (c imClient) UserToken(ctx context.Context, userID string, platformID int) (*ports.IMToken, error) {
	tok, err := c.client.UserToken(ctx, userID, platformID)
	if err != nil {
		return nil, err
	}
	return &ports.IMToken{Value: tok.Value, ExpireAt: tok.ExpireAt}, nil
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
