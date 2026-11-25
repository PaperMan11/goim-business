package cache

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisStore 是 cache 包内复用的 Redis 访问基座：统一键前缀拼装与 JSON 编解码。
// 仅通过组合使用，不对外暴露，也不强行统一各缓存的方法签名。
type redisStore struct {
	rdb    redis.UniversalClient
	prefix string
}

func newRedisStore(rdb redis.UniversalClient, prefix string) redisStore {
	return redisStore{rdb: rdb, prefix: prefix}
}

// key 按 "<prefix>:<segment>:..." 拼装缓存键。
func (s redisStore) key(segments ...string) string {
	return s.prefix + ":" + strings.Join(segments, ":")
}

// getJSON 读取并反序列化缓存值；未命中返回 ok=false。
func (s redisStore) getJSON(ctx context.Context, key string, dst any) (bool, error) {
	raw, err := s.rdb.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return false, err
	}
	return true, nil
}

// setJSON 序列化并写入缓存值。
func (s redisStore) setJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, key, raw, ttl).Err()
}

// del 删除缓存键。
func (s redisStore) del(ctx context.Context, key string) error {
	return s.rdb.Del(ctx, key).Err()
}
