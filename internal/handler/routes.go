// Package handler 负责 HTTP 路由注册与各业务域 handler 的装配。
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"goim-business/internal/app"
	"goim-business/internal/handler/account"
	"goim-business/internal/handler/im"
	"goim-business/internal/middlewares"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/pkg/response"
)

// InitRoutes 装配全局中间件，并按业务域注册路由。
func InitRoutes(a *app.App) *gin.Engine {
	r := gin.New()

	// gin 默认信任所有代理，X-Forwarded-For 可被客户端任意伪造；这里收敛为配置的可信网段。
	// 配置项已在 config.Validate 阶段校验，SetTrustedProxies 不会返回错误。
	_ = r.SetTrustedProxies(a.Config.Server.TrustedProxies)

	r.Use(
		middlewares.Recovery(a.Logger),
		middlewares.RequestID(),
		middlewares.AccessLog(a.Logger),
		middlewares.Metrics(),
		middlewares.CORS(a.Config.CORS.AllowedOrigins),
	)

	if a.Config.Server.MaxBodyBytes > 0 {
		r.Use(func(c *gin.Context) {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, a.Config.Server.MaxBodyBytes)
			c.Next()
		})
	}

	baseRoute := r.Group("/api/v1")

	account.InitRoutes(baseRoute, a)
	im.InitRoutes(baseRoute, a)

	r.NoRoute(func(c *gin.Context) {
		response.Abort(c, errcode.ErrNotFound.WithMessage("接口不存在"))
	})

	return r
}
