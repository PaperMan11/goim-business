package account

import (
	"github.com/gin-gonic/gin"

	"goim-business/internal/app"
	"goim-business/internal/middlewares"
	"goim-business/internal/pkg/response"
)

// Devices GET /biz-api/v1/devices
func Devices(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		views, err := a.AccountService.Devices(c.Request.Context(), middlewares.UserIDFrom(c))
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, views)
	}
}

// RemoveDevice DELETE /biz-api/v1/devices/:deviceId
func RemoveDevice(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := a.AccountService.RemoveDevice(
			c.Request.Context(),
			middlewares.UserIDFrom(c),
			c.Param("deviceId"),
		); err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, nil)
	}
}
