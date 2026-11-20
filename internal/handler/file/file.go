// Package file 承载文件域 HTTP 接口：申请直传、完成校验、访问签名与删除。
package file

import (
	"github.com/gin-gonic/gin"

	"goim-business/internal/app"
	"goim-business/internal/middlewares"
	"goim-business/internal/pkg/httpx"
	"goim-business/internal/pkg/response"
	"goim-business/internal/types"
)

// Presign POST /biz-api/v1/files/presign
func Presign(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in types.FilePresignRequest
		if err := httpx.BindJSON(c, &in); err != nil {
			response.Abort(c, err)
			return
		}
		result, err := a.FileService.PresignUpload(c.Request.Context(), middlewares.UserIDFrom(c), in)
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, result)
	}
}

// MultipartPresign POST /biz-api/v1/files/multipart/presign
func MultipartPresign(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in types.MultipartPresignRequest
		if err := httpx.BindJSON(c, &in); err != nil {
			response.Abort(c, err)
			return
		}
		result, err := a.FileService.MultipartPresign(c.Request.Context(), middlewares.UserIDFrom(c), in)
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, result)
	}
}

// RefreshMultipartParts POST /biz-api/v1/files/:fileId/multipart/parts
func RefreshMultipartParts(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in types.MultipartPartsRequest
		if err := httpx.BindJSON(c, &in); err != nil {
			response.Abort(c, err)
			return
		}
		result, err := a.FileService.RefreshMultipartParts(c.Request.Context(), middlewares.UserIDFrom(c), c.Param("fileId"), in.PartNumbers)
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, result)
	}
}

// MultipartComplete POST /biz-api/v1/files/:fileId/multipart/complete
func MultipartComplete(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in types.MultipartCompleteRequest
		if err := httpx.BindJSON(c, &in); err != nil {
			response.Abort(c, err)
			return
		}
		view, err := a.FileService.MultipartComplete(c.Request.Context(), middlewares.UserIDFrom(c), c.Param("fileId"), in)
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, view)
	}
}

// MultipartAbort POST /biz-api/v1/files/:fileId/multipart/abort
func MultipartAbort(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := a.FileService.MultipartAbort(c.Request.Context(), middlewares.UserIDFrom(c), c.Param("fileId")); err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, nil)
	}
}

// Complete POST /biz-api/v1/files/:fileId/complete
func Complete(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		view, err := a.FileService.Complete(c.Request.Context(), middlewares.UserIDFrom(c), c.Param("fileId"))
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, view)
	}
}

// URL GET /biz-api/v1/files/:fileId/url
func URL(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := a.FileService.URL(c.Request.Context(), middlewares.UserIDFrom(c), c.Param("fileId"))
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, result)
	}
}

// Delete DELETE /biz-api/v1/files/:fileId
func Delete(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := a.FileService.Delete(c.Request.Context(), middlewares.UserIDFrom(c), c.Param("fileId")); err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, nil)
	}
}
