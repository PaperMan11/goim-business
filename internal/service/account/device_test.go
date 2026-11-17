package account

import (
	"context"
	"testing"

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
