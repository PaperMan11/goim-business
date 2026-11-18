// Package config 负责加载分层配置（base.yaml → {env}.yaml → 环境变量）并做启动期强校验。
package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	filedomain "goim-business/internal/domain/file"

	"github.com/spf13/viper"
)

const envPrefix = "GOIM_BIZ"

// Config 是服务的完整配置。
type Config struct {
	Env       string          `mapstructure:"env"`
	Server    ServerConfig    `mapstructure:"server"`
	Log       LogConfig       `mapstructure:"log"`
	MySQL     MySQLConfig     `mapstructure:"mysql"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Auth      AuthConfig      `mapstructure:"auth"`
	RateLimit RateLimitConfig `mapstructure:"ratelimit"`
	CORS      CORSConfig      `mapstructure:"cors"`
	IM        IMConfig        `mapstructure:"im"`
	Storage   StorageConfig   `mapstructure:"storage"`
}

type ServerConfig struct {
	HTTPAddr        string        `mapstructure:"http_addr"`
	ProbeAddr       string        `mapstructure:"probe_addr"`
	MetricsAddr     string        `mapstructure:"metrics_addr"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	IdleTimeout     time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	MaxBodyBytes    int64         `mapstructure:"max_body_bytes"`
	// TrustedProxies 是反向代理的可信网段（IP 或 CIDR）；为空表示不信任任何 X-Forwarded-For。
	// 从右到左匹配，第一个匹配的网段为可信网段。
	TrustedProxies []string `mapstructure:"trusted_proxies"`
}

type LogConfig struct {
	Level   string `mapstructure:"level"`
	Format  string `mapstructure:"format"`
	Service string `mapstructure:"service"`
}

type MySQLConfig struct {
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
	SlowThreshold   time.Duration `mapstructure:"slow_threshold"`
}

type RedisConfig struct {
	Addr      string `mapstructure:"addr"`
	Password  string `mapstructure:"password"`
	DB        int    `mapstructure:"db"`
	KeyPrefix string `mapstructure:"key_prefix"`
}

type AuthConfig struct {
	JWTSecret              string        `mapstructure:"jwt_secret"`
	Issuer                 string        `mapstructure:"issuer"`
	Audience               string        `mapstructure:"audience"`
	AccessTTL              time.Duration `mapstructure:"access_ttl"`
	RefreshTTL             time.Duration `mapstructure:"refresh_ttl"`
	MaxSessionsPerUser     int           `mapstructure:"max_sessions_per_user"`
	MaxSessionsPerPlatform int           `mapstructure:"max_sessions_per_platform"`
	LoginLockThreshold     int64         `mapstructure:"login_lock_threshold"`
	LoginLockWindow        time.Duration `mapstructure:"login_lock_window"`
}

type RateLimitConfig struct {
	Enabled          bool  `mapstructure:"enabled"`
	LoginPerMinute   int64 `mapstructure:"login_per_minute"`
	RegisterPerHour  int64 `mapstructure:"register_per_hour"`
	IMTokenPerMinute int64 `mapstructure:"im_token_per_minute"`
}

type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

// IMConfig 是 goim im-api 客户端配置；base_url 为空表示未接入 IM。
type IMConfig struct {
	BaseURL           string        `mapstructure:"base_url"`
	APIBaseURL        string        `mapstructure:"api_base_url"`
	WSURL             string        `mapstructure:"ws_url"`
	AdminUserID       string        `mapstructure:"admin_user_id"`
	AdminSecret       string        `mapstructure:"admin_secret"`
	Timeout           time.Duration `mapstructure:"timeout"`
	TokenRefreshAhead time.Duration `mapstructure:"token_refresh_ahead"`
	// OutboxScanInterval 是 IM 开通补偿 worker 的扫描间隔。
	OutboxScanInterval time.Duration `mapstructure:"outbox_scan_interval"`
	// OutboxBatchSize 是单次扫描的最大任务条数。
	OutboxBatchSize int `mapstructure:"outbox_batch_size"`
}

// StorageConfig 是 RustFS / S3 兼容对象存储的配置；endpoint 与 bucket 同时为空表示未启用。
type StorageConfig struct {
	Endpoint       string        `mapstructure:"endpoint"`
	PublicEndpoint string        `mapstructure:"public_endpoint"`
	AccessKey      string        `mapstructure:"access_key"`
	SecretKey      string        `mapstructure:"secret_key"`
	Region         string        `mapstructure:"region"`
	Bucket         string        `mapstructure:"bucket"`
	UsePathStyle   bool          `mapstructure:"use_path_style"`
	PresignExpire  time.Duration `mapstructure:"presign_expire"`
	// MultipartPresignExpire 是分片上传签名有效期；大文件弱网需要更长窗口。
	MultipartPresignExpire time.Duration `mapstructure:"multipart_presign_expire"`
}

// Enabled 表示对象存储已配置。
func (c StorageConfig) Enabled() bool {
	return c.Endpoint != "" || c.Bucket != ""
}

// PublicObjectURL 返回公开命名空间下对象的稳定访问地址；非 public/ 前缀、未启用存储或 key 为空时返回空。
// 头像等公开资源使用该地址直接访问，附件等私有资源仍走预签名 GET。
func (c StorageConfig) PublicObjectURL(objectKey string) string {
	if !c.Enabled() || objectKey == "" || !filedomain.IsPublicObjectKey(objectKey) {
		return ""
	}
	base := c.PublicEndpoint
	if base == "" {
		base = c.Endpoint
	}
	key := strings.TrimLeft(strings.TrimSpace(objectKey), "/")
	return strings.TrimRight(base, "/") + "/" + c.Bucket + "/" + key
}

// Load 按 env 加载配置：base.yaml → {env}.yaml → GOIM_BIZ_* 环境变量。
func Load(dir, env string) (*Config, error) {
	if dir == "" {
		dir = "etc"
	}
	if env == "" {
		env = "dev"
	}

	v := viper.New()
	v.SetConfigType("yaml")
	v.AddConfigPath(dir)

	v.SetConfigName("base")
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取基础配置失败: %w", err)
	}

	if env != "base" {
		v.SetConfigName(env)
		if err := v.MergeInConfig(); err != nil {
			var notFound viper.ConfigFileNotFoundError
			if !asConfigNotFound(err, &notFound) {
				return nil, fmt.Errorf("读取环境配置 %s 失败: %w", env, err)
			}
		}
	}

	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "__"))
	v.AutomaticEnv()

	cfg := new(Config)
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	cfg.Env = env
	cfg.Server.TrustedProxies = normalizeProxyList(cfg.Server.TrustedProxies)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func asConfigNotFound(err error, target *viper.ConfigFileNotFoundError) bool {
	if e, ok := err.(viper.ConfigFileNotFoundError); ok {
		*target = e
		return true
	}
	return false
}

// normalizeProxyList 兼容环境变量注入：GOIM_BIZ_SERVER__TRUSTED_PROXIES 只能给字符串，
// 这里按逗号拆分，同时保留 YAML 列表写法。
func normalizeProxyList(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		for _, part := range strings.Split(item, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// Validate 启动期强校验：缺项直接退出，不允许带病启动。
func (c *Config) Validate() error {
	switch c.Env {
	case "dev", "staging", "prod":
	default:
		return fmt.Errorf("不支持的环境: %s", c.Env)
	}
	if c.Server.HTTPAddr == "" || c.Server.ProbeAddr == "" || c.Server.MetricsAddr == "" {
		return fmt.Errorf("server.http_addr/probe_addr/metrics_addr 不能为空")
	}
	if c.Server.MaxBodyBytes <= 0 {
		return fmt.Errorf("server.max_body_bytes 必须大于 0")
	}
	for _, proxy := range c.Server.TrustedProxies {
		if net.ParseIP(proxy) != nil {
			continue
		}
		if _, _, err := net.ParseCIDR(proxy); err != nil {
			return fmt.Errorf("server.trusted_proxies 含非法 IP/CIDR: %s", proxy)
		}
	}
	if c.MySQL.DSN == "" {
		return fmt.Errorf("mysql.dsn 未配置（请通过环境变量 GOIM_BIZ_MYSQL__DSN 注入）")
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("redis.addr 未配置（请通过环境变量 GOIM_BIZ_REDIS__ADDR 注入）")
	}
	if c.Redis.KeyPrefix == "" {
		return fmt.Errorf("redis.key_prefix 不能为空")
	}
	if len(c.Auth.JWTSecret) < 16 {
		return fmt.Errorf("auth.jwt_secret 长度不足 16（请通过环境变量 GOIM_BIZ_AUTH__JWT_SECRET 注入）")
	}
	if c.Auth.Issuer == "" || c.Auth.Audience == "" {
		return fmt.Errorf("auth.issuer/audience 不能为空")
	}
	if c.Auth.AccessTTL <= 0 || c.Auth.RefreshTTL <= 0 {
		return fmt.Errorf("auth.access_ttl/refresh_ttl 必须大于 0")
	}
	if c.Auth.RefreshTTL <= c.Auth.AccessTTL {
		return fmt.Errorf("auth.refresh_ttl 必须大于 access_ttl")
	}
	if c.Auth.MaxSessionsPerUser <= 0 || c.Auth.MaxSessionsPerPlatform <= 0 {
		return fmt.Errorf("auth.max_sessions_per_user/max_sessions_per_platform 必须大于 0")
	}
	if c.Auth.MaxSessionsPerPlatform > c.Auth.MaxSessionsPerUser {
		return fmt.Errorf("auth.max_sessions_per_platform 不能大于 max_sessions_per_user")
	}
	if c.Auth.LoginLockThreshold <= 0 || c.Auth.LoginLockWindow <= 0 {
		return fmt.Errorf("auth.login_lock_threshold/login_lock_window 必须大于 0")
	}
	if c.Storage.Enabled() {
		if err := validateStorage(c.Storage); err != nil {
			return err
		}
	}
	if c.IM.BaseURL != "" {
		u, err := url.Parse(c.IM.BaseURL)
		if err != nil {
			return fmt.Errorf("im.base_url 非法: %w", err)
		}
		if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("im.base_url 必须是带 host 的 http/https 地址: %s", c.IM.BaseURL)
		}
		if c.IM.AdminUserID == "" || c.IM.AdminSecret == "" {
			return fmt.Errorf("im.base_url 已配置时 im.admin_user_id/admin_secret 必填（secret 请通过环境变量 GOIM_BIZ_IM__ADMIN_SECRET 注入）")
		}
		if c.IM.OutboxScanInterval <= 0 {
			return fmt.Errorf("im.outbox_scan_interval 必须大于 0")
		}
		if c.IM.OutboxBatchSize <= 0 {
			return fmt.Errorf("im.outbox_batch_size 必须大于 0")
		}
	}
	return nil
}

// validateStorage 校验对象存储配置：endpoint 与 bucket 必须同时存在，密钥通过环境变量注入。
func validateStorage(s StorageConfig) error {
	if s.Endpoint == "" || s.Bucket == "" {
		return fmt.Errorf("storage.endpoint/storage.bucket 必须同时配置")
	}
	u, err := url.Parse(s.Endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("storage.endpoint 必须是带 host 的 http/https 地址: %s", s.Endpoint)
	}
	if s.PublicEndpoint != "" {
		pu, err := url.Parse(s.PublicEndpoint)
		if err != nil || (pu.Scheme != "http" && pu.Scheme != "https") || pu.Host == "" {
			return fmt.Errorf("storage.public_endpoint 必须是带 host 的 http/https 地址: %s", s.PublicEndpoint)
		}
	}
	if s.Region == "" {
		return fmt.Errorf("storage.region 不能为空")
	}
	if s.AccessKey == "" || s.SecretKey == "" {
		return fmt.Errorf("storage.access_key/secret_key 必填（secret 请通过 GOIM_BIZ_STORAGE__SECRET_KEY 注入）")
	}
	if s.PresignExpire <= 0 {
		return fmt.Errorf("storage.presign_expire 必须大于 0")
	}
	if s.MultipartPresignExpire <= 0 {
		return fmt.Errorf("storage.multipart_presign_expire 必须大于 0")
	}
	return nil
}
