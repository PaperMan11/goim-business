package account

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/service/ports"
	"goim-business/internal/types"
)

// fakeOutbox 是内存版 outbox 仓储；ListDue 不做时间过滤，测试直接控制到期与状态。
type fakeOutbox struct {
	mu      sync.Mutex
	seq     int64
	entries []ports.OutboxEntry
}

func newFakeOutbox() *fakeOutbox {
	return &fakeOutbox{}
}

func (f *fakeOutbox) Enqueue(_ context.Context, userID int64, action, payload string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	f.entries = append(f.entries, ports.OutboxEntry{
		ID:          f.seq,
		UserID:      userID,
		Action:      action,
		Payload:     payload,
		Status:      ports.OutboxStatusPending,
		NextRetryAt: time.Now(),
	})
	return f.seq, nil
}

func (f *fakeOutbox) GetByID(_ context.Context, id int64) (*ports.OutboxEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.entries {
		if f.entries[i].ID == id {
			clone := f.entries[i]
			return &clone, nil
		}
	}
	return nil, nil
}

func (f *fakeOutbox) ListDue(_ context.Context, limit int) ([]ports.OutboxEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ports.OutboxEntry, 0, len(f.entries))
	for _, entry := range f.entries {
		if entry.Status == ports.OutboxStatusDone {
			continue
		}
		out = append(out, entry)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (f *fakeOutbox) MarkDone(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.entries {
		if f.entries[i].ID == id && f.entries[i].Status != ports.OutboxStatusDone {
			f.entries[i].Status = ports.OutboxStatusDone
		}
	}
	return nil
}

func (f *fakeOutbox) MarkRetry(_ context.Context, id int64, attempts int, lastError string, nextRetryAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.entries {
		if f.entries[i].ID == id && f.entries[i].Status != ports.OutboxStatusDone {
			f.entries[i].Status = ports.OutboxStatusFailed
			f.entries[i].Attempts = attempts
			f.entries[i].LastError = lastError
			f.entries[i].NextRetryAt = nextRetryAt
		}
	}
	return nil
}

func (f *fakeOutbox) latest(userID int64) (ports.OutboxEntry, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.entries) - 1; i >= 0; i-- {
		if f.entries[i].UserID == userID {
			return f.entries[i], true
		}
	}
	return ports.OutboxEntry{}, false
}

func (f *fakeOutbox) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.entries)
}

func TestSettleTargetsExactOutboxEntry(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	id1, err := env.outbox.Enqueue(ctx, 1, ports.OutboxActionProfileSync, "")
	require.NoError(t, err)
	id2, err := env.outbox.Enqueue(ctx, 1, ports.OutboxActionProfileSync, "")
	require.NoError(t, err)

	// 第一次同步失败只应影响自己的任务，不能落定到后入队的任务上。
	env.imSync.Settle(ctx, id1, errors.New("im 不可用"))
	e1, err := env.outbox.GetByID(ctx, id1)
	require.NoError(t, err)
	require.Equal(t, ports.OutboxStatusFailed, e1.Status)
	require.Equal(t, 1, e1.Attempts)
	e2, err := env.outbox.GetByID(ctx, id2)
	require.NoError(t, err)
	require.Equal(t, ports.OutboxStatusPending, e2.Status)
	require.Zero(t, e2.Attempts)

	env.imSync.Settle(ctx, id2, nil)
	e2, err = env.outbox.GetByID(ctx, id2)
	require.NoError(t, err)
	require.Equal(t, ports.OutboxStatusDone, e2.Status)

	// 已完成任务不能被迟到的失败重新打开。
	require.NoError(t, env.outbox.MarkRetry(ctx, id2, 3, "late failure", time.Now()))
	e2, err = env.outbox.GetByID(ctx, id2)
	require.NoError(t, err)
	require.Equal(t, ports.OutboxStatusDone, e2.Status)
}

func TestRegisterIMOutboxMarkedDone(t *testing.T) {
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

	entry, ok := env.outbox.latest(userID)
	require.True(t, ok, "注册后应写入 outbox")
	require.Equal(t, ports.OutboxActionProvisionIM, entry.Action)
	require.Equal(t, ports.OutboxStatusDone, entry.Status)
	require.Equal(t, 0, entry.Attempts)

	stored, err := env.users.GetByID(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, domain.IMStatusReady, stored.IMStatus)
	require.Equal(t, 1, env.provisioner.callCount())
}

func TestRegisterIMOutboxEntersRetry(t *testing.T) {
	env := newTestEnv(t, nil)
	env.provisioner.setEnabled(true)
	env.provisioner.setErr(errors.New("im 不可用"))
	ctx := context.Background()

	nowBefore := env.svc.now()
	resp, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  testDevice,
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	userID := idOf(t, resp.User.UserID)

	entry, ok := env.outbox.latest(userID)
	require.True(t, ok)
	require.Equal(t, ports.OutboxStatusFailed, entry.Status)
	require.Equal(t, 1, entry.Attempts)
	require.NotEmpty(t, entry.LastError)
	require.True(t, entry.NextRetryAt.After(nowBefore), "重试时间应晚于当前时间")

	stored, err := env.users.GetByID(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, domain.IMStatusFailed, stored.IMStatus)
}

func TestProcessIMOutboxRecoversFailed(t *testing.T) {
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

	entry, ok := env.outbox.latest(userID)
	require.True(t, ok)
	require.Equal(t, ports.OutboxStatusFailed, entry.Status)

	// goim 恢复后，下一轮补偿应把失败任务推进到完成。
	env.provisioner.setErr(nil)
	processed := env.imSync.RunDue(ctx)
	require.Equal(t, 1, processed)

	entry, ok = env.outbox.latest(userID)
	require.True(t, ok)
	require.Equal(t, ports.OutboxStatusDone, entry.Status)
	require.Equal(t, 2, env.provisioner.callCount())

	stored, err := env.users.GetByID(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, domain.IMStatusReady, stored.IMStatus)
	require.NotNil(t, stored.IMSyncedAt)
}

func TestProcessIMOutboxSkipsReadyUser(t *testing.T) {
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

	// 模拟同步路径已把用户写为 Ready，但 outbox 未落定（例如标记失败）。
	at := env.svc.now()
	require.NoError(t, env.users.UpdateIMStatus(ctx, userID, domain.IMStatusReady, &at, at))
	env.provisioner.setErr(nil)
	processed := env.imSync.RunDue(ctx)
	require.Equal(t, 1, processed)
	require.Equal(t, 1, env.provisioner.callCount(), "已 Ready 的用户不应再次调用 goim")

	entry, ok := env.outbox.latest(userID)
	require.True(t, ok)
	require.Equal(t, ports.OutboxStatusDone, entry.Status)
}

func TestProcessIMOutboxDisabled(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	_, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  testDevice,
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	require.Equal(t, 0, env.outbox.count(), "IM 未启用时不应写入 outbox")

	require.Equal(t, 0, env.imSync.RunDue(ctx))
	require.Equal(t, 0, env.provisioner.callCount())
}
