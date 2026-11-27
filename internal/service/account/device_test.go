package account

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goim-business/internal/pkg/errcode"
	"goim-business/internal/types"
)

func TestDevicesOnlyReturnsOwn(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	alice, err := env.svc.Register(ctx, types.RegisterRequest{LoginName: "alice", Password: testPassword, DeviceID: "alice-web", Platform: testPlatform})
	require.NoError(t, err)
	_, err = env.svc.Login(ctx, types.LoginRequest{LoginName: "alice", Password: testPassword, DeviceID: "alice-phone", Platform: testPlatform})
	require.NoError(t, err)
	_, err = env.svc.Register(ctx, types.RegisterRequest{LoginName: "bob", Password: testPassword, DeviceID: "bob-web", Platform: testPlatform})
	require.NoError(t, err)

	views, err := env.svc.Devices(ctx, idOf(t, alice.User.UserID))
	require.NoError(t, err)
	require.Len(t, views, 2)
	deviceIDs := []string{views[0].DeviceID, views[1].DeviceID}
	require.ElementsMatch(t, []string{"alice-web", "alice-phone"}, deviceIDs)
}

func TestRemoveDeviceRevokesSessions(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	first, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  "shared-device",
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	userID := idOf(t, first.User.UserID)

	second, err := env.svc.Login(ctx, types.LoginRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  "shared-device",
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	require.Equal(t, 1, env.devices.count())

	require.NoError(t, env.svc.RemoveDevice(ctx, userID, "shared-device"))
	require.Equal(t, 0, env.devices.count())
	require.False(t, env.cache.isActive(first.SessionID))
	require.False(t, env.cache.isActive(second.SessionID))

	sid := idOf(t, first.SessionID)
	sess, ok := env.sessions.get(sid)
	require.True(t, ok)
	require.NotNil(t, sess.RevokedAt)
}

func TestRemoveDeviceNotFound(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	requireCode(t, env.svc.RemoveDevice(ctx, 18, "missing-device"), errcode.CodeNotFound)
}

func TestRemoveDeviceKicksIMDevice(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	first, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  "shared-device",
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	userID := idOf(t, first.User.UserID)
	env.imTokens.seed(strconv.FormatInt(userID, 10), testPlatform, "im-token", time.Now().Add(time.Hour))

	require.NoError(t, env.svc.RemoveDevice(ctx, userID, "shared-device"))

	calls := env.provisioner.snapshotKicks()
	require.NotEmpty(t, calls)
	for _, call := range calls {
		require.Equal(t, strconv.FormatInt(userID, 10), call.userID)
		require.Equal(t, testPlatform, call.platform)
		require.Equal(t, "shared-device", call.deviceID)
	}
	_, ok := env.imTokens.entry(strconv.FormatInt(userID, 10), testPlatform)
	require.False(t, ok, "撤销设备后应清除该平台的 IM token 缓存")
}

func TestRemoveDeviceIgnoresKickFailure(t *testing.T) {
	env := newTestEnv(t, nil)
	env.provisioner.setKickErr(errors.New("goim unavailable"))
	ctx := context.Background()

	first, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  "shared-device",
		Platform:  testPlatform,
	})
	require.NoError(t, err)

	require.NoError(t, env.svc.RemoveDevice(ctx, idOf(t, first.User.UserID), "shared-device"))
	require.NotEmpty(t, env.provisioner.snapshotKicks(), "IM 踢出失败不应阻止设备移除")
}

func TestRevokeSessionKicksDeviceWithSiblingSession(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	first, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  "shared-device",
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	// 同一设备再登录一次，模拟多标签页/同设备重登：撤销其中一条会话时，
	// 另一条仍有效，但设备级踢出仍必须执行，否则被撤销的会话还能继续发消息。
	second, err := env.svc.Login(ctx, types.LoginRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  "shared-device",
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	require.True(t, env.cache.isActive(first.SessionID))
	require.True(t, env.cache.isActive(second.SessionID))

	require.NoError(t, env.svc.RevokeSession(ctx, idOf(t, first.User.UserID), idOf(t, second.SessionID)))

	calls := env.provisioner.snapshotKicks()
	require.Len(t, calls, 1)
	require.Equal(t, "shared-device", calls[0].deviceID)
	require.Equal(t, testPlatform, calls[0].platform)
}

func TestLoginTouchesDevice(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	first, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName:  "alice",
		Password:   testPassword,
		DeviceID:   "shared-device",
		AppVersion: "1.0.0",
		Platform:   testPlatform,
	})
	require.NoError(t, err)
	require.Equal(t, 1, env.devices.count())

	_, err = env.svc.Login(ctx, types.LoginRequest{
		LoginName:  "alice",
		Password:   testPassword,
		DeviceID:   "shared-device",
		AppVersion: "1.1.0",
		Platform:   testPlatform,
	})
	require.NoError(t, err)

	stored, ok := env.devices.get(idOf(t, first.User.UserID), "shared-device")
	require.True(t, ok)
	require.Equal(t, "1.1.0", stored.AppVersion)
	require.Equal(t, 1, env.devices.count())
}
