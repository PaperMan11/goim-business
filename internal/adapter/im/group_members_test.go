package im

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"goim-business/internal/cache"
	"goim-business/internal/client/goim"
)

// newGroupMemberServer 返回计数群成员查询次数的 im-api 测试服务。
func newGroupMemberServer(t *testing.T, calls *int32, members []string, code int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/get_admin_token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "msg": "ok",
				"data": map[string]any{"token": "admin-token", "expireTimeSeconds": 300},
			})
		case "/api/v1/group/get_group_member_user_id":
			atomic.AddInt32(calls, 1)
			if code != 0 {
				_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "msg": "failed"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "msg": "ok",
				"data": map[string]any{"userIDs": members},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestGroupMembers(t *testing.T, srvURL string) *GroupMembers {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewGroupMembers(
		goim.New(goim.Config{BaseURL: srvURL, AdminUserID: "admin", AdminSecret: "secret"}),
		cache.NewGroupMemberCache(rdb, "test"),
	)
}

func TestGroupMembersCachesMembers(t *testing.T) {
	var calls int32
	srv := newGroupMemberServer(t, &calls, []string{"42", "99"}, 0)
	dir := newTestGroupMembers(t, srv.URL)

	members, err := dir.MemberUserIDs(context.Background(), "1001")
	require.NoError(t, err)
	require.Equal(t, []string{"42", "99"}, members)
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))

	members, err = dir.MemberUserIDs(context.Background(), "1001")
	require.NoError(t, err)
	require.Equal(t, []string{"42", "99"}, members)
	require.Equal(t, int32(1), atomic.LoadInt32(&calls), "缓存命中时不应重复调用 goim")
}

func TestGroupMembersDoesNotCacheFailure(t *testing.T) {
	var calls int32
	srv := newGroupMemberServer(t, &calls, nil, 5000)
	dir := newTestGroupMembers(t, srv.URL)

	_, err := dir.MemberUserIDs(context.Background(), "1001")
	require.Error(t, err)
	_, err = dir.MemberUserIDs(context.Background(), "1001")
	require.Error(t, err)
	require.Equal(t, int32(2), atomic.LoadInt32(&calls), "失败结果不写缓存，下次需重新查询")
}
