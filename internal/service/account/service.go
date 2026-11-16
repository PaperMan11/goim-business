// Package account 实现账号域核心链路：注册、登录、刷新轮换、登出、会话管理与改密。
package account

import (
	"context"
	"time"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/pkg/logger"
	"goim-business/internal/pkg/timex"
	"goim-business/internal/pkg/token"
	"goim-business/internal/service/ports"
)

// Config 是账号域的策略参数。
type Config struct {
	AccessTTL              time.Duration
	RefreshTTL             time.Duration
	MaxSessionsPerUser     int
	MaxSessionsPerPlatform int
	LoginLockThreshold     int64
	LoginLockWindow        time.Duration
	IMAPIBaseURL           string
	IMWSURL                string
}

// IMProvisioner 是账号域消费的 IM 同步能力；实现位于 internal/job/im，账号域只依赖该接口。
type IMProvisioner interface {
	Provision(ctx context.Context, user *domain.User) error
	SyncProfile(ctx context.Context, user *domain.User) error
	Settle(ctx context.Context, userID int64, action string, cause error)
}

// Service 是账号域服务。
type Service struct {
	users    domain.UserRepository
	creds    domain.CredentialRepository
	sessions domain.SessionRepository
	tx       ports.TxManager
	state    ports.SessionStateCache
	throttle ports.LoginThrottle
	im       ports.IMClient
	imTokens ports.IMTokenCache
	outbox   ports.OutboxRepository
	imSync   IMProvisioner
	tokens   *token.Issuer
	cfg      Config
	log      logger.Logger

	now func() time.Time
}

// NewService 构造账号服务。
func NewService(
	users domain.UserRepository,
	creds domain.CredentialRepository,
	sessions domain.SessionRepository,
	tx ports.TxManager,
	state ports.SessionStateCache,
	throttle ports.LoginThrottle,
	im ports.IMClient,
	imTokens ports.IMTokenCache,
	outbox ports.OutboxRepository,
	imSync IMProvisioner,
	tokens *token.Issuer,
	cfg Config,
	log logger.Logger,
) *Service {
	return &Service{
		users:    users,
		creds:    creds,
		sessions: sessions,
		tx:       tx,
		state:    state,
		throttle: throttle,
		im:       im,
		imTokens: imTokens,
		outbox:   outbox,
		imSync:   imSync,
		tokens:   tokens,
		cfg:      cfg,
		log:      log,
		now:      timex.Now,
	}
}

type sessionMeta struct {
	Platform  int8
	DeviceID  string
	UserAgent string
	IP        string
}
