package middlewares

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"goim-business/internal/config"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/pkg/ratelimit"
)

type rateLimitEnv struct {
	engine *gin.Engine
	mr     *miniredis.Miniredis
	calls  int
}

// newRateLimitEnv 构造真实 limiter（miniredis）+ 登录限流中间件 + 计数 handler 的 gin engine。
func newRateLimitEnv(t *testing.T, cfg config.RateLimitConfig) *rateLimitEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	env := &rateLimitEnv{engine: gin.New(), mr: mr}
	// 与服务端约定一致：不信任任何代理，避免 X-Forwarded-For 干扰 ClientIP 断言。
	require.NoError(t, env.engine.SetTrustedProxies(nil))

	limiter := ratelimit.New(rdb, "test")
	middleware := NewRateLimit(limiter, cfg, zap.NewNop())
	env.engine.POST("/login", middleware.Login(), func(c *gin.Context) {
		env.calls++
		c.JSON(http.StatusOK, gin.H{"code": errcode.CodeOK, "msg": "success"})
	})
	return env
}

func (e *rateLimitEnv) post(t *testing.T, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	e.engine.ServeHTTP(w, req)
	return w
}

type rateLimitBody struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func decodeRateLimitBody(t *testing.T, w *httptest.ResponseRecorder) rateLimitBody {
	t.Helper()
	var body rateLimitBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func TestRateLimitDisabledAllowsAll(t *testing.T) {
	env := newRateLimitEnv(t, config.RateLimitConfig{Enabled: false, LoginPerMinute: 1})

	for i := 0; i < 5; i++ {
		w := env.post(t, "1.2.3.4:1000")
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, errcode.CodeOK, decodeRateLimitBody(t, w).Code)
	}
	require.Equal(t, 5, env.calls, "关闭限流后每次请求都应进入 handler")
}

func TestRateLimitBlocksOverLimit(t *testing.T) {
	const limit = 2
	env := newRateLimitEnv(t, config.RateLimitConfig{Enabled: true, LoginPerMinute: limit})
	addr := "1.2.3.4:1000"

	for i := 0; i < limit; i++ {
		w := env.post(t, addr)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, errcode.CodeOK, decodeRateLimitBody(t, w).Code)
	}
	require.Equal(t, limit, env.calls)

	w := env.post(t, addr)
	require.Equal(t, http.StatusOK, w.Code, "限流响应 HTTP 状态仍为 200")
	require.Equal(t, errcode.CodeRateLimited, decodeRateLimitBody(t, w).Code)
	require.NotEmpty(t, w.Header().Get("Retry-After"))
	require.Equal(t, limit, env.calls, "被限流的请求不应进入后续 handler")
}

func TestRateLimitFailsClosedWhenRedisUnavailable(t *testing.T) {
	env := newRateLimitEnv(t, config.RateLimitConfig{Enabled: true, LoginPerMinute: 5})
	env.mr.Close()

	w := env.post(t, "1.2.3.4:1000")
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, errcode.CodeDependency, decodeRateLimitBody(t, w).Code)
	require.Equal(t, 0, env.calls, "Redis 不可用时必须失败关闭")
}

func TestRateLimitIsolatesClientsByIP(t *testing.T) {
	env := newRateLimitEnv(t, config.RateLimitConfig{Enabled: true, LoginPerMinute: 1})
	first, other := "1.2.3.4:1000", "5.6.7.8:1000"

	require.Equal(t, errcode.CodeOK, decodeRateLimitBody(t, env.post(t, first)).Code)
	require.Equal(t, errcode.CodeOK, decodeRateLimitBody(t, env.post(t, other)).Code)
	require.Equal(t, 2, env.calls)

	firstLimited := env.post(t, first)
	require.Equal(t, errcode.CodeRateLimited, decodeRateLimitBody(t, firstLimited).Code)
	otherLimited := env.post(t, other)
	require.Equal(t, errcode.CodeRateLimited, decodeRateLimitBody(t, otherLimited).Code)
	require.Equal(t, 2, env.calls, "不同 IP 共享 scope 但计数相互独立")
}
