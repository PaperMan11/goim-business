package account

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/pkg/errcode"
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
