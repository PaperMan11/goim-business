package im

import (
	"github.com/gin-gonic/gin"

	"goim-business/internal/app"
	"goim-business/internal/middlewares"
)

// InitRoutes 注册 IM 域路由：整体鉴权，兑换与重试额外按 IP 限流。
func InitRoutes(r *gin.RouterGroup, a *app.App) {
	limiter := middlewares.NewRateLimit(a.RateLimiter, a.Config.RateLimit, a.Logger)
	authenticator := middlewares.NewAuthenticator(a.TokenIssuer, a.SessionCache, a.Logger)

	imRoute := r.Group("/im")
	imRoute.Use(authenticator.Middleware())

	imRoute.POST("/token", limiter.IMToken(), Token(a))
	imRoute.GET("/status", Status(a))
	imRoute.POST("/retry", limiter.IMToken(), Retry(a))
}
