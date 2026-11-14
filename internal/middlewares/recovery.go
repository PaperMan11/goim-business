package middlewares

import (
	"bytes"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"goim-business/internal/pkg/errcode"
	"goim-business/internal/pkg/response"
)

// maxStackLines 控制日志中保留的堆栈行数，只留最核心的调用信息。
const maxStackLines = 15

// shortStack 返回截断后的堆栈，避免日志过长。
func shortStack() []byte {
	stack := debug.Stack()
	lines := bytes.SplitN(stack, []byte("\n"), maxStackLines+1)
	if len(lines) > maxStackLines {
		lines = lines[:maxStackLines]
	}
	return bytes.Join(lines, []byte("\n"))
}

// Recovery 捕获 panic，记录堆栈并返回统一内部错误。
func Recovery(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("请求 panic",
					zap.Any("panic", r),
					zap.String("request_id", RequestIDFrom(c)),
					zap.String("route", c.Request.URL.Path),
					zap.ByteString("stack", shortStack()),
				)
				response.Abort(c, errcode.ErrInternal)
			}
		}()
		c.Next()
	}
}
