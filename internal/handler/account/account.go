// Package account 承载账号域 HTTP 接口：注册、登录、刷新、登出、会话管理与改密。
package account

import (
	"github.com/gin-gonic/gin"

	"goim-business/internal/app"
	"goim-business/internal/middlewares"
	"goim-business/internal/pkg/convert"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/pkg/httpx"
	"goim-business/internal/pkg/response"
	"goim-business/internal/types"
)

// Register POST /api/v1/account/register
func Register(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in types.RegisterRequest
		if err := httpx.BindJSON(c, &in); err != nil {
			response.Abort(c, err)
			return
		}
		platform, err := httpx.PlatformFrom(c)
		if err != nil {
			response.Abort(c, err)
			return
		}
		in.UserAgent = c.Request.UserAgent()
		in.IP = c.ClientIP()
		in.Platform = platform

		result, err := a.AccountService.Register(c.Request.Context(), in)
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, result)
	}
}

// Login POST /api/v1/account/login
func Login(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in types.LoginRequest
		if err := httpx.BindJSON(c, &in); err != nil {
			response.Abort(c, err)
			return
		}
		platform, err := httpx.PlatformFrom(c)
		if err != nil {
			response.Abort(c, err)
			return
		}
		in.UserAgent = c.Request.UserAgent()
		in.IP = c.ClientIP()
		in.Platform = platform

		result, err := a.AccountService.Login(c.Request.Context(), in)
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, result)
	}
}

// Refresh POST /api/v1/account/refresh
func Refresh(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in types.RefreshRequest
		if err := httpx.BindJSON(c, &in); err != nil {
			response.Abort(c, err)
			return
		}
		in.UserAgent = c.Request.UserAgent()
		in.IP = c.ClientIP()

		result, err := a.AccountService.Refresh(c.Request.Context(), in)
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, result)
	}
}

// Logout POST /api/v1/account/logout
func Logout(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := a.AccountService.Logout(c.Request.Context(), middlewares.UserIDFrom(c), middlewares.SessionIDFrom(c)); err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, nil)
	}
}

// Sessions GET /api/v1/account/sessions
func Sessions(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		views, err := a.AccountService.Sessions(c.Request.Context(), middlewares.UserIDFrom(c), middlewares.SessionIDFrom(c))
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, views)
	}
}

// RevokeSession POST /api/v1/account/sessions/revoke
func RevokeSession(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in types.RevokeSessionRequest
		if err := httpx.BindJSON(c, &in); err != nil {
			response.Abort(c, err)
			return
		}
		targetSID := convert.ToInt64(in.SessionID)
		if targetSID == 0 {
			response.Abort(c, errcode.ErrInvalidParam.WithMessage("sessionId 格式非法"))
			return
		}
		if err := a.AccountService.RevokeSession(c.Request.Context(), middlewares.UserIDFrom(c), targetSID); err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, nil)
	}
}

// ChangePassword PUT /api/v1/account/password
func ChangePassword(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in types.ChangePasswordRequest
		if err := httpx.BindJSON(c, &in); err != nil {
			response.Abort(c, err)
			return
		}
		if err := a.AccountService.ChangePassword(c.Request.Context(), middlewares.UserIDFrom(c), middlewares.SessionIDFrom(c), in); err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, nil)
	}
}
