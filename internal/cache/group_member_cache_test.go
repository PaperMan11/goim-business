package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newTestGroupMemberCache(t *testing.T) (*GroupMemberCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewGroupMemberCache(rdb, "test"), mr
}

func TestGroupMemberCacheGetSet(t *testing.T) {
	c, _ := newTestGroupMemberCache(t)
	ctx := context.Background()

	_, ok, err := c.Get(ctx, "1001")
	require.NoError(t, err)
	require.False(t, ok, "未写入时应视为未命中")

	require.NoError(t, c.Set(ctx, "1001", []string{"42", "99"}, time.Minute))
	members, ok, err := c.Get(ctx, "1001")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, []string{"42", "99"}, members)

	require.NoError(t, c.Delete(ctx, "1001"))
	_, ok, err = c.Get(ctx, "1001")
	require.NoError(t, err)
	require.False(t, ok, "删除后应视为未命中")
}

func TestGroupMemberCacheExpires(t *testing.T) {
	c, mr := newTestGroupMemberCache(t)

	require.NoError(t, c.Set(context.Background(), "1001", []string{"42"}, time.Minute))
	mr.FastForward(time.Minute + time.Second)

	_, ok, err := c.Get(context.Background(), "1001")
	require.NoError(t, err)
	require.False(t, ok, "过期后应视为未命中")
}

func TestGroupMemberCacheCorruptedValueMisses(t *testing.T) {
	c, mr := newTestGroupMemberCache(t)
	mr.Set(c.key("1001"), "not-json")

	_, ok, err := c.Get(context.Background(), "1001")
	require.Error(t, err)
	require.False(t, ok)
}
