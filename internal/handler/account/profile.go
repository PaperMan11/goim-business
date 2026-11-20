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

// Profile GET /biz-api/v1/profile/me
func Profile(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		view, err := a.AccountService.Profile(c.Request.Context(), middlewares.UserIDFrom(c))
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, view)
	}
}

// UpdateProfile PUT /biz-api/v1/profile/me
func UpdateProfile(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in types.UpdateProfileRequest
		if err := httpx.BindJSON(c, &in); err != nil {
			response.Abort(c, err)
			return
		}
		view, err := a.AccountService.UpdateProfile(c.Request.Context(), middlewares.UserIDFrom(c), in)
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, view)
	}
}

// BatchProfiles POST /biz-api/v1/profile/batch
func BatchProfiles(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in types.BatchProfilesRequest
		if err := httpx.BindJSON(c, &in); err != nil {
			response.Abort(c, err)
			return
		}
		ids := make([]int64, 0, len(in.UserIDs))
		for _, raw := range in.UserIDs {
			id := convert.ToInt64(raw)
			if id <= 0 {
				response.Abort(c, errcode.ErrInvalidParam.WithMessage("userIds 包含非法用户 ID"))
				return
			}
			ids = append(ids, id)
		}
		views, err := a.AccountService.Profiles(c.Request.Context(), ids)
		if err != nil {
			response.Abort(c, err)
			return
		}
		response.OK(c, views)
	}
}

// PublicProfile GET /biz-api/v1/profile/users/:userId
func PublicProfile(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := convert.ToInt64(c.Param("userId"))
		if userID <= 0 {
			response.Abort(c, errcode.ErrInvalidParam.WithMessage("userId 格式非法"))
			return
		}
		views, err := a.AccountService.Profiles(c.Request.Context(), []int64{userID})
		if err != nil {
			response.Abort(c, err)
			return
		}
		if len(views) == 0 {
			response.Abort(c, errcode.ErrNotFound)
			return
		}
		response.OK(c, views[0])
	}
}
