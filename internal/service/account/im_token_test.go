package account

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goim-business/internal/pkg/errcode"
	"goim-business/internal/service/ports"
	"goim-business/internal/types"
)

func registerIMReadyUser(t *testing.T, env *testEnv) int64 {
	t.Helper()
	resp, err := env.svc.Register(context.Background(), types.RegisterRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  testDevice,
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	return idOf(t, resp.User.UserID)
}

func TestIMTokenCacheHitSkipsClient(t *testing.T) {
	env := newTestEnv(t, func(cfg *Config) {
		cfg.IMAPIBaseURL = "https://im.example.com/api/v1"
		cfg.IMWSURL = "wss://im.example.com/ws"
	})
	env.provisioner.setEnabled(true)
	ctx := context.Background()
	userID := registerIMReadyUser(t, env)

	expireAt := env.svc.now().Add(20 * time.Minute)
	env.imTokens.seed("1", testPlatform, "cached-token", expireAt)

	resp, err := env.svc.IMToken(ctx, userID, testPlatform)
	require.NoError(t, err)
	require.Equal(t, "cached-token", resp.Token)
	require.Equal(t, "1", resp.UserID)
	require.Equal(t, testPlatform, resp.PlatformID)
	require.Equal(t, "https://im.example.com/api/v1", resp.APIBaseURL)
	require.Equal(t, "wss://im.example.com/ws", resp.WSURL)
	require.InDelta(t, float64(20*time.Minute/time.Second), float64(resp.ExpireTimeSeconds), 2)
	require.Equal(t, 0, env.provisioner.tokenCallCount())
}

func TestIMTokenCacheMissFetchesAndWrites(t *testing.T) {
	env := newTestEnv(t, nil)
	env.provisioner.setEnabled(true)
	ctx := context.Background()
	userID := registerIMReadyUser(t, env)

	expireAt := env.svc.now().Add(40 * time.Minute)
	env.provisioner.setToken(&ports.IMToken{Value: "fresh-token", ExpireAt: expireAt}, nil)

	resp, err := env.svc.IMToken(ctx, userID, testPlatform)
	require.NoError(t, err)
	require.Equal(t, "fresh-token", resp.Token)
	require.Equal(t, 1, env.provisioner.tokenCallCount())

	entry, ok := env.imTokens.entry("1", testPlatform)
	require.True(t, ok, "未命中时应回写缓存")
	require.Equal(t, "fresh-token", entry.token)
	require.Equal(t, expireAt, entry.expireAt)
	// TTL = 剩余有效期 - 10 分钟。
	require.InDelta(t, float64(30*time.Minute/time.Second), float64(entry.ttl/time.Second), 2)
}

func TestIMTokenErrors(t *testing.T) {
	ctx := context.Background()

	env := newTestEnv(t, nil)
	_, err := env.svc.IMToken(ctx, 1, testPlatform)
	requireCode(t, err, errcode.CodeDependency)

	env.provisioner.setEnabled(true)
	_, err = env.svc.IMToken(ctx, 999, testPlatform)
	requireCode(t, err, errcode.CodeNotFound)

	_, err = env.svc.IMToken(ctx, 999, 10)
	requireCode(t, err, errcode.CodeInvalidParam)

	// IM 未启用的注册会保持 Pending，开通完成前不允许兑换 token。
	env.provisioner.setEnabled(false)
	pendingUserID := registerIMReadyUser(t, env)
	env.provisioner.setEnabled(true)
	_, err = env.svc.IMToken(ctx, pendingUserID, testPlatform)
	requireCode(t, err, errcode.CodeDependency)
}

func TestIMStatus(t *testing.T) {
	ctx := context.Background()

	env := newTestEnv(t, nil)
	env.provisioner.setEnabled(true)
	userID := registerIMReadyUser(t, env)

	resp, err := env.svc.IMStatus(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, "1", resp.UserID)
	require.Equal(t, int8(1), resp.Status)
	require.NotNil(t, resp.SyncedAt)

	_, err = env.svc.IMStatus(ctx, 999)
	requireCode(t, err, errcode.CodeNotFound)
}
