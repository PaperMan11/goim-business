package clientconfig

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"goim-business/internal/app"
	"goim-business/internal/pkg/httpx"
	"goim-business/internal/pkg/response"
)

// Config GET /api/v1/client-config?platform=5
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
		c.Header("Cache-Control", "no-cache")
		if etagMatches(c.GetHeader("If-None-Match"), etag) {
			c.Set(response.CodeKey, 0)
			c.Status(http.StatusNotModified)
			return
		}
		response.OK(c, view)
	}
}

// Schema GET /api/v1/client-config/schema
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
