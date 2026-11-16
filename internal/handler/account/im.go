package account

import (
	"github.com/gin-gonic/gin"

	"goim-business/internal/app"
	"goim-business/internal/middlewares"
	"goim-business/internal/pkg/response"
)

// IMToken POST /api/v1/im/token
func IMToken(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := a.AccountService.IMToken(c.Request.Context(), middlewares.UserIDFrom(c), middlewares.PlatformIDFrom(c))
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, result)
	}
}

// IMStatus GET /api/v1/im/status
func IMStatus(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := a.AccountService.IMStatus(c.Request.Context(), middlewares.UserIDFrom(c))
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, result)
	}
}

// IMRetry POST /api/v1/im/retry
func IMRetry(a *app.App) gin.HandlerFunc {
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
