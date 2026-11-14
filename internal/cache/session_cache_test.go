package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newTestCache(t *testing.T) (*SessionCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewSessionCache(rdb, "test"), mr
}

func TestSessionActiveLifecycle(t *testing.T) {
	c, mr := newTestCache(t)
	ctx := context.Background()

	require.NoError(t, c.Activate(ctx, "sid-1", "user-1", time.Minute))
	active, err := c.IsActive(ctx, "sid-1")
	require.NoError(t, err)
	require.True(t, active)

	require.NoError(t, c.Deactivate(ctx, "sid-1"))
	active, err = c.IsActive(ctx, "sid-1")
	require.NoError(t, err)
	require.False(t, active)

	require.NoError(t, c.Activate(ctx, "sid-2", "user-2", time.Minute))
	mr.FastForward(2 * time.Minute)
	active, err = c.IsActive(ctx, "sid-2")
	require.NoError(t, err)
	require.False(t, active)
}

func TestLoginFailureCounter(t *testing.T) {
	c, mr := newTestCache(t)
	ctx := context.Background()

	count, err := c.IncrLoginFailure(ctx, "alice", time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	count, err = c.IncrLoginFailure(ctx, "alice", time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)

	loaded, err := c.LoginFailureCount(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, int64(2), loaded)

	require.NoError(t, c.ClearLoginFailures(ctx, "alice"))
	loaded, err = c.LoginFailureCount(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, int64(0), loaded)

	_, err = c.IncrLoginFailure(ctx, "bob", time.Minute)
	require.NoError(t, err)
	mr.FastForward(2 * time.Minute)
	loaded, err = c.LoginFailureCount(ctx, "bob")
	require.NoError(t, err)
	require.Equal(t, int64(0), loaded)
}
