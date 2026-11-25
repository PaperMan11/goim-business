package im

import (
	"context"
	"time"

	"goim-business/internal/cache"
	"goim-business/internal/client/goim"
)

// groupMemberCacheTTL 是群成员列表的缓存时长；短暂延迟可接受，避免逐条消息重复远程调用。
const groupMemberCacheTTL = 60 * time.Second

// GroupMembers 把 goim 群成员查询适配为文件服务的成员校验端口：
// 先读 Redis 缓存，未命中或缓存异常时回源，并且只缓存远程成功的结果。
type GroupMembers struct {
	client *goim.Client
	cache  *cache.GroupMemberCache
}

// NewGroupMembers 构造群成员查询适配器。
func NewGroupMembers(client *goim.Client, memberCache *cache.GroupMemberCache) *GroupMembers {
	return &GroupMembers{client: client, cache: memberCache}
}

// MemberUserIDs 返回群成员用户 ID；命中缓存时不再调用 goim。
func (g *GroupMembers) MemberUserIDs(ctx context.Context, groupID string) ([]string, error) {
	if members, ok, err := g.cache.Get(ctx, groupID); err == nil && ok {
		return members, nil
	}

	members, err := g.client.GroupMemberUserIDs(ctx, groupID)
	if err != nil {
		return nil, err
	}
	// 缓存写失败不影响鉴权结果，下次回源即可。
	_ = g.cache.Set(ctx, groupID, members, groupMemberCacheTTL)
	return members, nil
}
