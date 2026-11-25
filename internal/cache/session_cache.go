// Package cache 封装业务侧使用的 Redis 缓存：会话活跃标记、登录失败计数、IM token 与群成员。
package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// SessionCache 使用 Redis 保存会话活跃标记与登录失败次数。
type SessionCache struct {
	store redisStore
}

// NewSessionCache 构造缓存；prefix 用于多环境共享 Redis 时的键隔离。
func NewSessionCache(rdb redis.UniversalClient, prefix string) *SessionCache {
	return &SessionCache{store: newRedisStore(rdb, prefix)}
}

func (c *SessionCache) activeKey(sid string) string {
	return c.store.key("sess", "active", sid)
}

func (c *SessionCache) failureKey(loginName string) string {
	return c.store.key("login", "fail", loginName)
}

// Activate 标记会话活跃；TTL 与会话过期时间对齐，支持“踢下线”即时生效。
func (c *SessionCache) Activate(ctx context.Context, sid, userID string, ttl time.Duration) error {
	return c.store.rdb.Set(ctx, c.activeKey(sid), userID, ttl).Err()
}

// Deactivate 清除会话活跃标记。
func (c *SessionCache) Deactivate(ctx context.Context, sid string) error {
	return c.store.del(ctx, c.activeKey(sid))
}

// IsActive 判断会话是否仍然活跃。
func (c *SessionCache) IsActive(ctx context.Context, sid string) (bool, error) {
	n, err := c.store.rdb.Exists(ctx, c.activeKey(sid)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// LoginFailureCount 返回当前登录失败次数。
func (c *SessionCache) LoginFailureCount(ctx context.Context, loginName string) (int64, error) {
	n, err := c.store.rdb.Get(ctx, c.failureKey(loginName)).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return n, nil
}

// IncrLoginFailure 增加失败次数；首次计数时设置窗口 TTL。
func (c *SessionCache) IncrLoginFailure(ctx context.Context, loginName string, window time.Duration) (int64, error) {
	key := c.failureKey(loginName)
	n, err := c.store.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if n == 1 {
		if err := c.store.rdb.Expire(ctx, key, window).Err(); err != nil {
			return n, err
		}
	}
	return n, nil
}

// ClearLoginFailures 登录成功后清除失败计数。
func (c *SessionCache) ClearLoginFailures(ctx context.Context, loginName string) error {
	return c.store.del(ctx, c.failureKey(loginName))
}
