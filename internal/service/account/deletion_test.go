package account

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/service/ports"
	"goim-business/internal/types"
)

func TestDeleteAccountMarksDeletingAndPurgesIM(t *testing.T) {
	env := newTestEnv(t, func(cfg *Config) {
		cfg.DeletionCooldown = 15 * 24 * time.Hour
	})
	env.provisioner.setEnabled(true)
	ctx := context.Background()

	resp, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice", Password: testPassword, DeviceID: testDevice, Platform: testPlatform,
	})
	require.NoError(t, err)
	userID := idOf(t, resp.User.UserID)
	require.Equal(t, domain.IMStatusReady, mustUser(t, env, userID).IMStatus)

	// 预置 IM token 缓存，验证注销时按平台清除。
	env.imTokens.seed(resp.User.UserID, 5, "im-token", env.svc.now().Add(time.Hour))

	before := env.svc.now()
	result, err := env.svc.DeleteAccount(ctx, userID)
	require.NoError(t, err)
	require.True(t, result.PurgeAt.After(before), "purgeAt 应晚于当前时间")
	require.True(t, result.PurgeAt.After(before.Add(14*24*time.Hour)), "purgeAt 应至少接近冷却期")

	stored := mustUser(t, env, userID)
	require.Equal(t, domain.StatusDeleting, stored.Status)
	require.NotNil(t, stored.PurgeAt)
	require.True(t, stored.PurgeAt.Equal(result.PurgeAt))
	require.Equal(t, domain.IMStatusPurged, stored.IMStatus)

	// 会话全部撤销且 Redis 活跃标记清除。
	require.Equal(t, 0, env.sessions.activeCount())
	require.False(t, env.cache.isActive(resp.SessionID))

	// IM token 缓存已清除。
	_, ok := env.imTokens.entry(resp.User.UserID, 5)
	require.False(t, ok)

	// 注销同步任务已入队并完成。
	entry, ok := env.outbox.latest(userID)
	require.True(t, ok)
	require.Equal(t, ports.OutboxActionPurgeIM, entry.Action)
	require.Equal(t, ports.OutboxStatusDone, entry.Status)

	// 注销后无法登录，公开资料也不再可见。
	_, err = env.svc.Login(ctx, types.LoginRequest{LoginName: "alice", Password: testPassword, DeviceID: testDevice, Platform: testPlatform})
	requireCode(t, err, errcode.CodeInvalidCredential)

	views, err := env.svc.Profiles(ctx, []int64{userID})
	require.NoError(t, err)
	require.Empty(t, views)

	// 重复注销幂等：返回同一清除时间，且不重复入队。
	count := env.outbox.count()
	again, err := env.svc.DeleteAccount(ctx, userID)
	require.NoError(t, err)
	require.True(t, again.PurgeAt.Equal(result.PurgeAt))
	require.Equal(t, count, env.outbox.count())
}

func TestDeleteAccountWithoutIMSkipsOutbox(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	resp, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice", Password: testPassword, DeviceID: testDevice, Platform: testPlatform,
	})
	require.NoError(t, err)

	_, err = env.svc.DeleteAccount(ctx, idOf(t, resp.User.UserID))
	require.NoError(t, err)
	require.Equal(t, 0, env.outbox.count(), "IM 未启用时不应写入 outbox")
	require.Equal(t, domain.StatusDeleting, mustUser(t, env, idOf(t, resp.User.UserID)).Status)
}

func TestDeleteAccountFrozenRejected(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	resp, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice", Password: testPassword, DeviceID: testDevice, Platform: testPlatform,
	})
	require.NoError(t, err)
	userID := idOf(t, resp.User.UserID)
	env.users.mu.Lock()
	env.users.byID[userID].Status = domain.StatusFrozen
	env.users.mu.Unlock()

	_, err = env.svc.DeleteAccount(ctx, userID)
	requireCode(t, err, errcode.CodeAccountFrozen)
}

func mustUser(t *testing.T, env *testEnv, userID int64) *domain.User {
	t.Helper()
	user, err := env.users.GetByID(context.Background(), userID)
	require.NoError(t, err)
	return user
}
