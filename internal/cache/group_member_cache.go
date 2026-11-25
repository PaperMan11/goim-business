package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// GroupMemberCache 使用 Redis 缓存按群维度的成员用户 ID 列表。
type GroupMemberCache struct {
	store redisStore
}

// NewGroupMemberCache 构造缓存；prefix 用于多环境共享 Redis 时的键隔离。
func NewGroupMemberCache(rdb redis.UniversalClient, prefix string) *GroupMemberCache {
	return &GroupMemberCache{store: newRedisStore(rdb, prefix)}
}

func (c *GroupMemberCache) key(groupID string) string {
	return c.store.key("file", "gm", groupID)
}

// Get 读取缓存的群成员；未命中或内容损坏时 ok=false。
func (c *GroupMemberCache) Get(ctx context.Context, groupID string) ([]string, bool, error) {
	var members []string
	ok, err := c.store.getJSON(ctx, c.key(groupID), &members)
	if err != nil || !ok {
		return nil, false, err
	}
	return members, true, nil
}

// Set 写入群成员列表，ttl 由调用方按成员变更容忍度传入。
func (c *GroupMemberCache) Set(ctx context.Context, groupID string, members []string, ttl time.Duration) error {
	return c.store.setJSON(ctx, c.key(groupID), members, ttl)
}

// Delete 清除指定群的成员缓存，供成员变更时主动失效。
func (c *GroupMemberCache) Delete(ctx context.Context, groupID string) error {
	return c.store.del(ctx, c.key(groupID))
}
