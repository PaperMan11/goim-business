package account

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/service/ports"
	"goim-business/internal/types"
)

func TestUpdateProfileSyncsIM(t *testing.T) {
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
	userID := idOf(t, resp.User.UserID)
	require.Equal(t, domain.IMStatusReady, env.mustUser(t, userID).IMStatus)

	nickname := " Alice "
	bio := " hello "
	view, err := env.svc.UpdateProfile(ctx, userID, types.UpdateProfileRequest{Nickname: &nickname, Bio: &bio})
	require.NoError(t, err)
	require.Equal(t, resp.User.UserID, view.UserID)
	require.Equal(t, "alice", view.LoginName)
	require.Equal(t, "Alice", view.Nickname)
	require.Equal(t, "hello", view.Bio)
	require.Empty(t, view.AvatarKey)

	stored := env.mustUser(t, userID)
	require.Equal(t, "Alice", stored.Nickname)
	require.Equal(t, "hello", stored.Bio)

	entry, ok := env.outbox.latest(userID)
	require.True(t, ok)
	require.Equal(t, ports.OutboxActionProfileSync, entry.Action)
	require.Equal(t, ports.OutboxStatusDone, entry.Status)

	calls, updateUser, updateNickname, updateAvatar := env.provisioner.lastUpdate()
	require.Equal(t, 1, calls)
	require.Equal(t, resp.User.UserID, updateUser)
	require.Equal(t, "Alice", updateNickname)
	require.Empty(t, updateAvatar)
}

func TestUpdateProfileSyncsAvatarURL(t *testing.T) {
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
	userID := idOf(t, resp.User.UserID)

	avatar := "https://cdn.example.com/avatars/alice.png"
	_, err = env.svc.UpdateProfile(ctx, userID, types.UpdateProfileRequest{AvatarKey: &avatar})
	require.NoError(t, err)
	_, _, _, gotAvatar := env.provisioner.lastUpdate()
	require.Equal(t, avatar, gotAvatar)

	// 对象存储 key 不是可直接访问的 URL，不应作为 faceURL 下发。
	key := "avatars/alice.png"
	_, err = env.svc.UpdateProfile(ctx, userID, types.UpdateProfileRequest{AvatarKey: &key})
	require.NoError(t, err)
	require.Equal(t, key, env.mustUser(t, userID).AvatarKey)
	_, _, _, gotAvatar = env.provisioner.lastUpdate()
	require.Empty(t, gotAvatar)
	require.Equal(t, 2, env.provisioner.updateCallCount())
}

func TestAvatarObjectKeyResolvesToPublicURL(t *testing.T) {
	env := newTestEnv(t, func(cfg *Config) {
		cfg.AvatarURL = func(key string) string {
			return "https://files.example.com/goim-business/" + key
		}
	})
	env.provisioner.setEnabled(true)
	ctx := context.Background()

	resp, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  testDevice,
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	userID := idOf(t, resp.User.UserID)

	key := "public/avatar/123/a.png"
	expected := "https://files.example.com/goim-business/" + key
	view, err := env.svc.UpdateProfile(ctx, userID, types.UpdateProfileRequest{AvatarKey: &key})
	require.NoError(t, err)
	require.Equal(t, key, view.AvatarKey)
	require.Equal(t, expected, view.AvatarURL)

	_, _, _, gotAvatar := env.provisioner.lastUpdate()
	require.Equal(t, expected, gotAvatar)

	views, err := env.svc.Profiles(ctx, []int64{userID})
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Equal(t, expected, views[0].AvatarURL)
}

func TestUpdateProfileValidation(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	resp, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  testDevice,
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	userID := idOf(t, resp.User.UserID)

	_, err = env.svc.UpdateProfile(ctx, userID, types.UpdateProfileRequest{})
	requireCode(t, err, errcode.CodeInvalidParam)

	blank := "   "
	_, err = env.svc.UpdateProfile(ctx, userID, types.UpdateProfileRequest{Nickname: &blank})
	requireCode(t, err, errcode.CodeInvalidParam)

	_, err = env.svc.UpdateProfile(ctx, 99999, types.UpdateProfileRequest{})
	requireCode(t, err, errcode.CodeInvalidParam)

	bio := "hi"
	_, err = env.svc.UpdateProfile(ctx, 99999, types.UpdateProfileRequest{Bio: &bio})
	requireCode(t, err, errcode.CodeNotFound)

	_, err = env.svc.Profile(ctx, 99999)
	requireCode(t, err, errcode.CodeNotFound)
}

func TestUpdateProfileWithoutIM(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	resp, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  testDevice,
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	userID := idOf(t, resp.User.UserID)
	require.Equal(t, 0, env.outbox.count())

	bio := "hello"
	view, err := env.svc.UpdateProfile(ctx, userID, types.UpdateProfileRequest{Bio: &bio})
	require.NoError(t, err)
	require.Equal(t, "hello", view.Bio)
	require.Equal(t, 0, env.outbox.count(), "IM 未启用时不应写入 outbox")
	require.Equal(t, 0, env.provisioner.updateCallCount())
}

func TestProfileOutboxWaitsForIMReady(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	resp, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  testDevice,
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	userID := idOf(t, resp.User.UserID)

	env.provisioner.setEnabled(true)
	_, err = env.outbox.Enqueue(ctx, userID, ports.OutboxActionProfileSync, "")
	require.NoError(t, err)

	// IM 账号尚未开通：资料同步应进入重试而不是直接调用 goim。
	require.Equal(t, 1, env.imSync.RunDue(ctx))
	entry, ok := env.outbox.latest(userID)
	require.True(t, ok)
	require.Equal(t, ports.OutboxStatusFailed, entry.Status)
	require.Equal(t, 0, env.provisioner.updateCallCount())

	at := env.svc.now()
	require.NoError(t, env.users.UpdateIMStatus(ctx, userID, domain.IMStatusReady, &at, at))

	require.Equal(t, 1, env.imSync.RunDue(ctx))
	entry, ok = env.outbox.latest(userID)
	require.True(t, ok)
	require.Equal(t, ports.OutboxStatusDone, entry.Status)
	require.Equal(t, 1, env.provisioner.updateCallCount())
}

func TestBatchProfiles(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	alice, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  "device-1",
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	bob, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "bob",
		Password:  testPassword,
		DeviceID:  "device-2",
		Platform:  testPlatform,
	})
	require.NoError(t, err)

	views, err := env.svc.Profiles(ctx, []int64{idOf(t, alice.User.UserID), idOf(t, bob.User.UserID), 99999})
	require.NoError(t, err)
	require.Len(t, views, 2, "不存在的用户应被跳过")
	require.Equal(t, alice.User.UserID, views[0].UserID)
	require.Equal(t, "alice", views[0].Nickname)
	require.Equal(t, bob.User.UserID, views[1].UserID)
	require.Equal(t, "bob", views[1].Nickname)

	views, err = env.svc.Profiles(ctx, nil)
	require.NoError(t, err)
	require.Empty(t, views)
}

func (e *testEnv) mustUser(t *testing.T, userID int64) *domain.User {
	t.Helper()
	user, err := e.users.GetByID(context.Background(), userID)
	require.NoError(t, err)
	return user
}
