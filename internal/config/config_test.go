package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadDev(t *testing.T) {
	cfg, err := Load("../../etc", "dev")
	require.NoError(t, err)
	require.Equal(t, "dev", cfg.Env)
	require.NotEmpty(t, cfg.MySQL.DSN)
	require.Equal(t, 15*time.Minute, cfg.Auth.AccessTTL)
	require.Equal(t, 720*time.Hour, cfg.Auth.RefreshTTL)
	require.Equal(t, 5, cfg.Auth.MaxSessionsPerUser)
}

func TestLoadEnvOverride(t *testing.T) {
	t.Setenv("GOIM_BIZ_AUTH__JWT_SECRET", "env-provided-secret-0123456789")

	cfg, err := Load("../../etc", "dev")
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
