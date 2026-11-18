package account

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	domain "goim-business/internal/domain/account"
)

type fakeUserPurger struct {
	users       map[int64]*domain.User
	markCalled  int
	rejectPurge bool
}

func (f *fakeUserPurger) ListDueForPurge(_ context.Context, now time.Time, limit int) ([]*domain.User, error) {
	out := make([]*domain.User, 0, len(f.users))
	for _, user := range f.users {
		if user.Status != domain.StatusDeleting || user.PurgeAt == nil || user.PurgeAt.After(now) {
			continue
		}
		clone := *user
		out = append(out, &clone)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PurgeAt.Before(*out[j].PurgeAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeUserPurger) MarkPurged(_ context.Context, userID int64, loginName string, at time.Time) (bool, error) {
	f.markCalled++
	user, ok := f.users[userID]
	if !ok || f.rejectPurge || user.Status != domain.StatusDeleting {
		return false, nil
	}
	user.Status = domain.StatusDeleted
	user.LoginName = loginName
	user.Nickname = ""
	user.AvatarKey = ""
	user.Bio = ""
	user.UpdatedAt = at
	return true, nil
}

type fakeCredPurger struct{ deleted []int64 }

func (f *fakeCredPurger) DeleteByUser(_ context.Context, userID int64) error {
	f.deleted = append(f.deleted, userID)
	return nil
}

type fakeSessionPurger struct{ deleted []int64 }

func (f *fakeSessionPurger) DeleteByUser(_ context.Context, userID int64) (int64, error) {
	f.deleted = append(f.deleted, userID)
	return 1, nil
}

type fakeDevicePurger struct{ deleted []int64 }

func (f *fakeDevicePurger) DeleteByUser(_ context.Context, userID int64) (int64, error) {
	f.deleted = append(f.deleted, userID)
	return 1, nil
}

type fakeFilePurger struct{ marked []int64 }

func (f *fakeFilePurger) MarkDeletedByUser(_ context.Context, userID int64, _ time.Time) (int64, error) {
	f.marked = append(f.marked, userID)
	return 2, nil
}

type noopTx struct{}

func (noopTx) WithTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func TestPurgerClearsDueAccount(t *testing.T) {
	now := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	due := now.Add(-time.Hour)
	users := &fakeUserPurger{users: map[int64]*domain.User{
		1: {
			ID: 1, LoginName: "alice", Nickname: "Alice", AvatarKey: "public/avatar/1/a.png",
			Bio: "hi", Status: domain.StatusDeleting, PurgeAt: &due,
		},
		2: {ID: 2, LoginName: "bob", Status: domain.StatusNormal},
	}}
	creds := &fakeCredPurger{}
	sessions := &fakeSessionPurger{}
	devices := &fakeDevicePurger{}
	files := &fakeFilePurger{}

	purger := NewPurger(users, creds, sessions, devices, files, noopTx{}, 50, func() time.Time { return now }, zap.NewNop().Sugar())
	result := purger.RunOnce(context.Background())
	require.Equal(t, PurgeResult{Scanned: 1, Purged: 1}, result)

	stored := users.users[1]
	require.Equal(t, domain.StatusDeleted, stored.Status)
	require.Equal(t, "~deleted_1", stored.LoginName)
	require.Empty(t, stored.Nickname)
	require.Empty(t, stored.AvatarKey)
	require.Empty(t, stored.Bio)
	require.Equal(t, []int64{1}, creds.deleted)
	require.Equal(t, []int64{1}, sessions.deleted)
	require.Equal(t, []int64{1}, devices.deleted)
	require.Equal(t, []int64{1}, files.marked)

	// 再次执行：账号已是 deleted，不会重复清除。
	again := purger.RunOnce(context.Background())
	require.Equal(t, PurgeResult{Scanned: 0}, again)
	require.Len(t, creds.deleted, 1, "重复执行不应再次删除凭证")
}

func TestPurgerSkipsAlreadyHandledAccount(t *testing.T) {
	now := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	due := now.Add(-time.Hour)
	users := &fakeUserPurger{
		users: map[int64]*domain.User{
			1: {ID: 1, LoginName: "alice", Status: domain.StatusDeleting, PurgeAt: &due},
		},
		rejectPurge: true,
	}
	creds := &fakeCredPurger{}
	purger := NewPurger(users, creds, &fakeSessionPurger{}, &fakeDevicePurger{}, &fakeFilePurger{}, noopTx{}, 50, func() time.Time { return now }, zap.NewNop().Sugar())

	result := purger.RunOnce(context.Background())
	require.Equal(t, PurgeResult{Scanned: 1}, result, "条件更新失败时不应计为已清除")
	require.Empty(t, creds.deleted)
}
