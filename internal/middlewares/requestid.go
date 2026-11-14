package middlewares

import (
	"strings"

	"github.com/gin-gonic/gin"

	"goim-business/internal/pkg/idgen"
)

// RequestID 读取 X-Request-ID（兼容 SDK 的 operationID），无则生成 UUIDv7，并回写响应头。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if rid == "" {
			rid = strings.TrimSpace(c.GetHeader("operationID"))
		}
		if rid == "" || len(rid) > 128 {
			rid = idgen.MustNew()
		}
		c.Set(CtxRequestID, rid)
		c.Header("X-Request-ID", rid)
		c.Next()
	}
}
