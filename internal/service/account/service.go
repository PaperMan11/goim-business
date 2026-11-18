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
	// DeletionCooldown 是账号注销后到物理清除之间的冷却期。
	DeletionCooldown time.Duration
	// AvatarURL 把头像对象 key 解析为公开访问 URL；为空时仅接受已是 http(s) 的地址。
	AvatarURL func(objectKey string) string
}

// IMProvisioner 是账号域消费的 IM 同步能力；实现位于 internal/job/im，账号域只依赖该接口。
type IMProvisioner interface {
	Provision(ctx context.Context, user *domain.User) error
	SyncProfile(ctx context.Context, user *domain.User) error
	// Purge 把 IM 侧资料置为注销占位；IM 侧没有投影时同样视为成功。
	Purge(ctx context.Context, user *domain.User) error
	// Settle 按任务 ID 落定本次同步结果，避免并发请求交叉落定。
	Settle(ctx context.Context, entryID int64, cause error)
}

// Service 是账号域服务。
type Service struct {
	users    domain.UserRepository
	creds    domain.CredentialRepository
	sessions domain.SessionRepository
	devices  domain.DeviceRepository
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
	devices domain.DeviceRepository,
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
	// 兜底默认冷却期 15 天；正常由配置校验保证大于 0。
	if cfg.DeletionCooldown <= 0 {
		cfg.DeletionCooldown = 15 * 24 * time.Hour
	}
	return &Service{
		users:    users,
		creds:    creds,
		sessions: sessions,
		devices:  devices,
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
