package cache

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// IMTokenCache 使用 Redis 缓存按 (userID, platform) 维度的 IM token。
type IMTokenCache struct {
	store redisStore
}

// NewIMTokenCache 构造缓存；prefix 用于多环境共享 Redis 时的键隔离。
func NewIMTokenCache(rdb redis.UniversalClient, prefix string) *IMTokenCache {
	return &IMTokenCache{store: newRedisStore(rdb, prefix)}
}

// imTokenValue 是缓存值的 JSON 结构，expireAt 为 Unix 秒。
type imTokenValue struct {
	Token    string `json:"token"`
	ExpireAt int64  `json:"expireAt"`
}

func (c *IMTokenCache) key(userID string, platform int8) string {
	return c.store.key("im", "token", userID, strconv.Itoa(int(platform)))
}

// Get 读取缓存的 IM token；未命中返回 ok=false。
func (c *IMTokenCache) Get(ctx context.Context, userID string, platform int8) (string, time.Time, bool, error) {
	var value imTokenValue
	ok, err := c.store.getJSON(ctx, c.key(userID, platform), &value)
	if err != nil || !ok {
		return "", time.Time{}, false, err
	}
	return value.Token, time.Unix(value.ExpireAt, 0), true, nil
}

// Set 写入 IM token，ttl 由调用方按剩余有效期扣减后传入。
func (c *IMTokenCache) Set(ctx context.Context, userID string, platform int8, token string, expireAt time.Time, ttl time.Duration) error {
	return c.store.setJSON(ctx, c.key(userID, platform), imTokenValue{Token: token, ExpireAt: expireAt.Unix()}, ttl)
}

// Delete 清除指定平台的 IM token 缓存。
func (c *IMTokenCache) Delete(ctx context.Context, userID string, platform int8) error {
	return c.store.del(ctx, c.key(userID, platform))
}
