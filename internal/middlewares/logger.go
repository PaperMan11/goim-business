package middlewares

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"goim-business/internal/pkg/response"
)

// AccessLog 每个请求只打一条访问日志。
func AccessLog(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		code := 0
		if v, ok := c.Get(response.CodeKey); ok {
			if n, ok := v.(int); ok {
				code = n
			}
		}
		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}

		fields := []zap.Field{
			zap.String("request_id", RequestIDFrom(c)),
			zap.Int64("user_id", UserIDFrom(c)),
			zap.String("method", c.Request.Method),
			zap.String("route", route),
			zap.Int("status", status),
			zap.Int("error_code", code),
			zap.Int64("latency_ms", time.Since(start).Milliseconds()),
			zap.String("client_ip", c.ClientIP()),
		}
		if status >= 500 || code >= 9000 {
			log.Error("请求处理失败", fields...)
			return
		}
		log.Debug("请求完成", fields...)
	}
}
