package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestFixedWindow(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	limiter := New(rdb, "test")
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		allowed, _, err := limiter.Allow(ctx, "login", "1.2.3.4", 3, time.Minute)
		require.NoError(t, err)
		require.True(t, allowed)
	}

	allowed, _, err := limiter.Allow(ctx, "login", "1.2.3.4", 3, time.Minute)
	require.NoError(t, err)
	require.False(t, allowed)

	// 不同维度互不影响。
	allowed, _, err = limiter.Allow(ctx, "login", "5.6.7.8", 3, time.Minute)
	require.NoError(t, err)
	require.True(t, allowed)

	// 窗口过期后重新计数。
	mr.FastForward(2 * time.Minute)
	allowed, _, err = limiter.Allow(ctx, "login", "1.2.3.4", 3, time.Minute)
	require.NoError(t, err)
	require.True(t, allowed)
}
