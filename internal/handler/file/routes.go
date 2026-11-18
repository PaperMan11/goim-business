package file

import (
	"goim-business/internal/app"
	"goim-business/internal/middlewares"

	"github.com/gin-gonic/gin"
)

// InitRoutes 注册文件域路由，全部要求登录。
func InitRoutes(r *gin.RouterGroup, a *app.App) {
	authenticator := middlewares.NewAuthenticator(a.TokenIssuer, a.SessionCache, a.Logger)

	fileRoute := r.Group("/files")
	fileRoute.Use(authenticator.Middleware())
	fileRoute.POST("/presign", Presign(a))
	fileRoute.POST("/multipart/presign", MultipartPresign(a))
	fileRoute.POST("/:fileId/multipart/parts", RefreshMultipartParts(a))
	fileRoute.POST("/:fileId/complete", Complete(a))
	fileRoute.POST("/:fileId/multipart/complete", MultipartComplete(a))
	fileRoute.POST("/:fileId/multipart/abort", MultipartAbort(a))
	fileRoute.GET("/:fileId/url", URL(a))
	fileRoute.DELETE("/:fileId", Delete(a))
}
