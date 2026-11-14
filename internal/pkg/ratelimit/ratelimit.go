// Package ratelimit 提供基于 Redis 的固定窗口限流。
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter 对指定键做固定窗口计数。
type Limiter struct {
	rdb    redis.UniversalClient
	prefix string
}

// New 构造限流器。
func New(rdb redis.UniversalClient, prefix string) *Limiter {
	return &Limiter{rdb: rdb, prefix: prefix}
}

// Allow 返回本次请求是否放行；keys 是业务维度（例如 IP、账号）。
func (l *Limiter) Allow(ctx context.Context, scope, key string, limit int64, window time.Duration) (bool, int64, error) {
	if limit <= 0 {
		return true, 0, nil
	}
	redisKey := fmt.Sprintf("%s:rl:%s:%s", l.prefix, scope, key)

	n, err := l.rdb.Incr(ctx, redisKey).Result()
	if err != nil {
		return false, 0, err
	}
	if n == 1 {
		if err := l.rdb.Expire(ctx, redisKey, window).Err(); err != nil {
			return false, 0, err
		}
	}
	if n > limit {
		return false, 0, nil
	}
	return true, limit - n, nil
}

// RetryAfter 返回键的剩余 TTL，用于 Retry-After 响应头。
func (l *Limiter) RetryAfter(ctx context.Context, scope, key string) time.Duration {
	ttl, err := l.rdb.TTL(ctx, fmt.Sprintf("%s:rl:%s:%s", l.prefix, scope, key)).Result()
	if err != nil || ttl < 0 {
		return 0
	}
	return ttl
}
