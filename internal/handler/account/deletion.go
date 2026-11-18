package account

import (
	"github.com/gin-gonic/gin"

	"goim-business/internal/app"
	"goim-business/internal/middlewares"
	"goim-business/internal/pkg/response"
)

// DeleteAccount DELETE /api/v1/account
func DeleteAccount(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := a.AccountService.DeleteAccount(c.Request.Context(), middlewares.UserIDFrom(c))
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, result)
	}
}
