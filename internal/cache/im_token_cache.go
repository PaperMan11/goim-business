package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// IMTokenCache 使用 Redis 缓存按 (userID, platform) 维度的 IM token。
type IMTokenCache struct {
	rdb    redis.UniversalClient
	prefix string
}

// NewIMTokenCache 构造缓存；prefix 用于多环境共享 Redis 时的键隔离。
func NewIMTokenCache(rdb redis.UniversalClient, prefix string) *IMTokenCache {
	return &IMTokenCache{rdb: rdb, prefix: prefix}
}

// imTokenValue 是缓存值的 JSON 结构，expireAt 为 Unix 秒。
type imTokenValue struct {
	Token    string `json:"token"`
	ExpireAt int64  `json:"expireAt"`
}

func (c *IMTokenCache) key(userID string, platform int8) string {
	return fmt.Sprintf("%s:im:token:%s:%d", c.prefix, userID, platform)
}

// Get 读取缓存的 IM token；redis.Nil 视为未命中。
func (c *IMTokenCache) Get(ctx context.Context, userID string, platform int8) (string, time.Time, bool, error) {
	raw, err := c.rdb.Get(ctx, c.key(userID, platform)).Bytes()
	if err == redis.Nil {
		return "", time.Time{}, false, nil
	}
	if err != nil {
		return "", time.Time{}, false, err
	}
	var value imTokenValue
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", time.Time{}, false, err
	}
	return value.Token, time.Unix(value.ExpireAt, 0), true, nil
}

// Set 写入 IM token，ttl 由调用方按剩余有效期扣减后传入。
func (c *IMTokenCache) Set(ctx context.Context, userID string, platform int8, token string, expireAt time.Time, ttl time.Duration) error {
	raw, err := json.Marshal(imTokenValue{Token: token, ExpireAt: expireAt.Unix()})
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, c.key(userID, platform), raw, ttl).Err()
}

// Delete 清除指定平台的 IM token 缓存。
func (c *IMTokenCache) Delete(ctx context.Context, userID string, platform int8) error {
	return c.rdb.Del(ctx, c.key(userID, platform)).Err()
}
