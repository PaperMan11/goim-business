// Package clientconfig 注册客户端配置路由。
package clientconfig

import (
	"github.com/gin-gonic/gin"

	"goim-business/internal/app"
)

// InitRoutes 注册 /client-config 相关路由；均为公开接口。
func InitRoutes(r *gin.RouterGroup, a *app.App) {
	group := r.Group("/client-config")
	group.GET("", Config(a))
	group.GET("/schema", Schema(a))
}
