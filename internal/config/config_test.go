package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadDev(t *testing.T) {
	cfg, err := LoadFile("../../etc/dev.yaml")
	require.NoError(t, err)
	require.Equal(t, "dev", cfg.Env)
	require.NotEmpty(t, cfg.MySQL.DSN)
	require.Equal(t, 15*time.Minute, cfg.Auth.AccessTTL)
	require.Equal(t, 720*time.Hour, cfg.Auth.RefreshTTL)
	require.Equal(t, 5, cfg.Auth.MaxSessionsPerUser)
}

func TestLoadFileMissing(t *testing.T) {
	_, err := LoadFile("../../etc/not-exist.yaml")
	require.Error(t, err)
}

func TestStoragePublicObjectURL(t *testing.T) {
	cfg := StorageConfig{
		Endpoint:       "http://rustfs:9000",
		PublicEndpoint: "https://files.example.com/",
		Bucket:         "goim-business",
	}
	require.Equal(t,
		"https://files.example.com/goim-business/public/avatar/123/a.png",
		cfg.PublicObjectURL("public/avatar/123/a.png"))
	require.Equal(t,
		"https://files.example.com/goim-business/public/avatar/123/a.png",
		cfg.PublicObjectURL("/public/avatar/123/a.png"))
	require.Empty(t, cfg.PublicObjectURL("attachment/123/a.pdf"))
	require.Empty(t, cfg.PublicObjectURL("avatar/123/a.png"))
	require.Empty(t, cfg.PublicObjectURL(""))
	require.Empty(t, StorageConfig{}.PublicObjectURL("public/avatar/123/a.png"))
}

func TestLoadFileEnvOverride(t *testing.T) {
	t.Setenv("GOIM_BIZ_AUTH__JWT_SECRET", "env-provided-secret-0123456789")

	cfg, err := LoadFile("../../etc/dev.yaml")
	require.NoError(t, err)
	require.Equal(t, "env-provided-secret-0123456789", cfg.Auth.JWTSecret)
}

func TestValidateRejectsMissingRequired(t *testing.T) {
	cfg := &Config{
		Env: "prod",
		Server: ServerConfig{
			HTTPAddr:     "0.0.0.0:8080",
			ProbeAddr:    "0.0.0.0:8081",
			MetricsAddr:  "0.0.0.0:9091",
			MaxBodyBytes: 1,
		},
		Redis: RedisConfig{Addr: "127.0.0.1:6379", KeyPrefix: "goim:business"},
	}
	require.Error(t, cfg.Validate())
}

func validConfig() *Config {
	return &Config{
		Env: "dev",
		Server: ServerConfig{
			HTTPAddr:     "0.0.0.0:8080",
			ProbeAddr:    "0.0.0.0:8081",
			MetricsAddr:  "0.0.0.0:9091",
			MaxBodyBytes: 1,
		},
		MySQL: MySQLConfig{DSN: "root:pass@tcp(127.0.0.1:3306)/goim_business"},
		Redis: RedisConfig{Addr: "127.0.0.1:6379", KeyPrefix: "goim:business"},
		Auth: AuthConfig{
			JWTSecret:              "0123456789abcdef",
			Issuer:                 "goim-business",
			Audience:               "goim-business-api",
			AccessTTL:              time.Minute,
			RefreshTTL:             time.Hour,
			MaxSessionsPerUser:     5,
			MaxSessionsPerPlatform: 3,
			LoginLockThreshold:     10,
			LoginLockWindow:        time.Minute,
		},
	}
}

func TestValidateIMBaseURL(t *testing.T) {
	cfg := validConfig()
	cfg.IM = IMConfig{
		BaseURL:     "im.example.com",
		AdminUserID: "admin",
		AdminSecret: "secret",
	}
	require.ErrorContains(t, cfg.Validate(), "im.base_url")
}

func TestValidateIMMissingAdminCredentials(t *testing.T) {
	cfg := validConfig()
	cfg.IM = IMConfig{BaseURL: "http://im.example.com"}
	require.ErrorContains(t, cfg.Validate(), "im.admin_user_id/admin_secret")
}
