package clientconfig

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"goim-business/internal/app"
	"goim-business/internal/pkg/httpx"
	"goim-business/internal/pkg/response"
)

// Config GET /biz-api/v1/client-config?platform=5
func Config(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.Query("platform")
		if strings.TrimSpace(raw) == "" {
			raw = c.GetHeader("X-Client-Platform")
		}
		platform, err := httpx.PlatformFromValue(raw)
		if err != nil {
			response.Abort(c, err)
			return
		}

		view, etag, err := a.ClientConfigService.Get(c.Request.Context(), platform)
		if err != nil {
			response.Abort(c, err)
			return
		}
		c.Header("ETag", etag)
		// no-cache 要求客户端每次带 If-None-Match 校验，命中后才可复用本地缓存。
		c.Header("Cache-Control", "no-cache")
		if etagMatches(c.GetHeader("If-None-Match"), etag) {
			// 304 不带响应体，仅保留 ETag 与 Cache-Control；客户端继续使用本地配置。
			// 显式写入业务码，保证访问日志把该请求记录为成功而不是缺失 code。
			c.Set(response.CodeKey, 0)
			c.Status(http.StatusNotModified)
			return
		}
		response.OK(c, view)
	}
}

// Schema GET /biz-api/v1/client-config/schema
func Schema(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		response.OK(c, a.ClientConfigService.Schema())
	}
}

// etagMatches 判断 If-None-Match 是否命中当前 ETag，兼容 W/ 前缀与多值。
func etagMatches(header, etag string) bool {
	for _, part := range strings.Split(header, ",") {
		candidate := strings.TrimSpace(part)
		if candidate == "*" || candidate == etag || strings.TrimPrefix(candidate, "W/") == etag {
			return true
		}
	}
	return false
}
