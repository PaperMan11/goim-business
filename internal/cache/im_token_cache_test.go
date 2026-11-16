package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newTestIMTokenCache(t *testing.T) (*IMTokenCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewIMTokenCache(rdb, "test"), mr
}

func TestIMTokenCacheSetGet(t *testing.T) {
	c, _ := newTestIMTokenCache(t)
	ctx := context.Background()
	expireAt := time.Unix(1_800_000_000, 0)

	require.NoError(t, c.Set(ctx, "user-1", 5, "token-1", expireAt, 10*time.Minute))

	token, gotExpireAt, ok, err := c.Get(ctx, "user-1", 5)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "token-1", token)
	require.Equal(t, expireAt, gotExpireAt)
}

func TestIMTokenCacheMiss(t *testing.T) {
	c, _ := newTestIMTokenCache(t)
	ctx := context.Background()

	token, expireAt, ok, err := c.Get(ctx, "user-1", 5)
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, token)
	require.True(t, expireAt.IsZero())
}

func TestIMTokenCacheTTL(t *testing.T) {
	c, mr := newTestIMTokenCache(t)
	ctx := context.Background()

	require.NoError(t, c.Set(ctx, "user-1", 5, "token-1", time.Unix(1_800_000_000, 0), time.Minute))

	mr.FastForward(2 * time.Minute)

	_, _, ok, err := c.Get(ctx, "user-1", 5)
	require.NoError(t, err)
	require.False(t, ok)
}
