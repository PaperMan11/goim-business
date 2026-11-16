package account

import (
	"github.com/gin-gonic/gin"

	"goim-business/internal/app"
	"goim-business/internal/middlewares"
)

// InitRoutes 注册账号域路由。
func InitRoutes(r *gin.RouterGroup, a *app.App) {
	limiter := middlewares.NewRateLimit(a.RateLimiter, a.Config.RateLimit, a.Logger)
	authenticator := middlewares.NewAuthenticator(a.TokenIssuer, a.SessionCache, a.Logger)

	accountRoute := r.Group("/account")

	accountRoute.POST("/register", limiter.Register(), Register(a))
	accountRoute.POST("/login", limiter.Login(), Login(a))
	accountRoute.POST("/refresh", Refresh(a))

	authed := accountRoute.Group("")
	authed.Use(authenticator.Middleware())
	authed.POST("/logout", Logout(a))
	authed.GET("/sessions", Sessions(a))
	authed.POST("/sessions/revoke", RevokeSession(a))
	authed.PUT("/password", ChangePassword(a))

	// IM 凭证属于账号域：仍挂在 /api/v1/im，但由账号 handler 统一注册。
	imRoute := r.Group("/im")
	imRoute.Use(authenticator.Middleware())
	imRoute.POST("/token", limiter.IMToken(), IMToken(a))
	imRoute.GET("/status", IMStatus(a))
	imRoute.POST("/retry", limiter.IMToken(), IMRetry(a))
}
