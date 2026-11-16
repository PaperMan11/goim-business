package middlewares

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"goim-business/internal/config"
	"goim-business/internal/pkg/convert"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/pkg/ratelimit"
	"goim-business/internal/pkg/response"
)

// RateLimit 提供登录与注册的 IP 维度限流（失败关闭）。
type RateLimit struct {
	limiter *ratelimit.Limiter
	cfg     config.RateLimitConfig
	log     *zap.Logger
}

// NewRateLimit 构造限流中间件。
func NewRateLimit(limiter *ratelimit.Limiter, cfg config.RateLimitConfig, log *zap.Logger) *RateLimit {
	return &RateLimit{limiter: limiter, cfg: cfg, log: log}
}

// Login 登录限流：默认 5 次/分钟/IP。
func (r *RateLimit) Login() gin.HandlerFunc {
	return r.apply("login", r.cfg.LoginPerMinute, time.Minute)
}

// Register 注册限流：默认 3 次/小时/IP。
func (r *RateLimit) Register() gin.HandlerFunc {
	return r.apply("register", r.cfg.RegisterPerHour, time.Hour)
}

// IMToken IM token 兑换/重试限流：默认 30 次/分钟/IP。
func (r *RateLimit) IMToken() gin.HandlerFunc {
	return r.apply("im_token", r.cfg.IMTokenPerMinute, time.Minute)
}

func (r *RateLimit) apply(scope string, limit int64, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !r.cfg.Enabled || limit <= 0 {
			c.Next()
			return
		}
		key := c.ClientIP()
		allowed, _, err := r.limiter.Allow(c.Request.Context(), scope, key, limit, window)
		if err != nil {
			r.log.Error("限流检查失败（失败关闭）", zap.String("scope", scope), zap.Error(err))
			response.Abort(c, errcode.ErrDependency)
			return
		}
		if !allowed {
			if ttl := r.limiter.RetryAfter(c.Request.Context(), scope, key); ttl > 0 {
				c.Header("Retry-After", convert.ToString(int(ttl.Seconds()+0.5)))
			}
			response.Abort(c, errcode.ErrRateLimited)
			return
		}
		c.Next()
	}
}
