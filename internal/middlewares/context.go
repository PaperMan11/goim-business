// Package middlewares 提供 HTTP 横切能力：请求 ID、日志、恢复、CORS、指标、鉴权、限流。
package middlewares

import "github.com/gin-gonic/gin"

// gin.Context 中的键。
const (
	CtxRequestID = "request_id"
	CtxUserID    = "auth.user_id"
	CtxSessionID = "auth.session_id"
	CtxPlatform  = "auth.platform_id"
	CtxDeviceID  = "auth.device_id"
)

// RequestIDFrom 返回请求 ID。
func RequestIDFrom(c *gin.Context) string {
	if v, ok := c.Get(CtxRequestID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// UserIDFrom 返回已认证用户 ID；未认证时为 0。
func UserIDFrom(c *gin.Context) int64 {
	if v, ok := c.Get(CtxUserID); ok {
		if n, ok := v.(int64); ok {
			return n
		}
	}
	return 0
}

// SessionIDFrom 返回当前会话 ID；未认证时为 0。
func SessionIDFrom(c *gin.Context) int64 {
	if v, ok := c.Get(CtxSessionID); ok {
		if n, ok := v.(int64); ok {
			return n
		}
	}
	return 0
}
