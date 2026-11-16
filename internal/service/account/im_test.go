package account

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/service/ports"
	"goim-business/internal/types"
)

func TestRegisterProvisionIM(t *testing.T) {
	env := newTestEnv(t, nil)
	env.provisioner.setEnabled(true)
	ctx := context.Background()

	resp, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  testDevice,
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Token.AccessToken)
	require.NotEmpty(t, resp.RefreshToken)

	userID := idOf(t, resp.User.UserID)
	stored, err := env.users.GetByID(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, domain.IMStatusReady, stored.IMStatus)
	require.NotNil(t, stored.IMSyncedAt)

	calls, gotUserID, gotNickname := env.provisioner.last()
	require.Equal(t, 1, calls)
	require.Equal(t, "1", gotUserID)
	require.Equal(t, "alice", gotNickname)
}

func TestRegisterIMFailureKeepsRegistration(t *testing.T) {
	env := newTestEnv(t, nil)
	env.provisioner.setEnabled(true)
	env.provisioner.setErr(errors.New("im 不可用"))
	ctx := context.Background()

	resp, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  testDevice,
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Token.AccessToken)
	require.NotEmpty(t, resp.RefreshToken)

	stored, err := env.users.GetByID(ctx, idOf(t, resp.User.UserID))
	require.NoError(t, err)
	require.Equal(t, domain.IMStatusFailed, stored.IMStatus)
	require.Nil(t, stored.IMSyncedAt)
}

func TestRegisterIMDisabled(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	resp, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  testDevice,
		Platform:  testPlatform,
	})
	require.NoError(t, err)

	stored, err := env.users.GetByID(ctx, idOf(t, resp.User.UserID))
	require.NoError(t, err)
	require.Equal(t, domain.IMStatusPending, stored.IMStatus)
	require.Nil(t, stored.IMSyncedAt)
	require.Equal(t, 0, env.provisioner.callCount())
}

func TestRetryIMProvisionSucceeds(t *testing.T) {
	env := newTestEnv(t, nil)
	env.provisioner.setEnabled(true)
	env.provisioner.setErr(errors.New("im 不可用"))
	ctx := context.Background()

	resp, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  testDevice,
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	userID := idOf(t, resp.User.UserID)

	stored, err := env.users.GetByID(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, domain.IMStatusFailed, stored.IMStatus)

	env.provisioner.setErr(nil)
	require.NoError(t, env.svc.RetryIMProvision(ctx, userID))

	stored, err = env.users.GetByID(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, domain.IMStatusReady, stored.IMStatus)
	require.NotNil(t, stored.IMSyncedAt)
}

func TestRetryIMProvisionErrors(t *testing.T) {
	ctx := context.Background()

	env := newTestEnv(t, nil)
	requireCode(t, env.svc.RetryIMProvision(ctx, 999), errcode.CodeNotFound)

	env.provisioner.setEnabled(false)
	resp, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  testDevice,
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	requireCode(t, env.svc.RetryIMProvision(ctx, idOf(t, resp.User.UserID)), errcode.CodeDependency)
}

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
