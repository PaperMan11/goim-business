// Package im 承载 IM 凭证与开通状态接口：token 兑换、状态查询与手动重试。
package im

import (
	"github.com/gin-gonic/gin"

	"goim-business/internal/app"
	"goim-business/internal/middlewares"
	"goim-business/internal/pkg/response"
)

// Token POST /api/v1/im/token
func Token(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := a.AccountService.IMToken(c.Request.Context(), middlewares.UserIDFrom(c), middlewares.PlatformIDFrom(c))
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, result)
	}
}

// Status GET /api/v1/im/status
func Status(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := a.AccountService.IMStatus(c.Request.Context(), middlewares.UserIDFrom(c))
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, result)
	}
}

// Retry POST /api/v1/im/retry
func Retry(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := middlewares.UserIDFrom(c)
		if err := a.AccountService.RetryIMProvision(c.Request.Context(), userID); err != nil {
			response.Abort(c, err)
			return
		}
		result, err := a.AccountService.IMStatus(c.Request.Context(), userID)
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, result)
	}
}
