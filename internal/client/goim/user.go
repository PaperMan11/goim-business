package goim

import "context"

// UserInfo 是需要同步到 IM 的用户资料。
type UserInfo struct {
	UserID    string
	Nickname  string
	AvatarURL string
}

// registerRequest 是 /user/user_register 的请求体。
type registerRequest struct {
	Users []registerUser `json:"users"`
}

// registerUser 与 goim 的 sdkws.UserInfo 字段一致。
type registerUser struct {
	UserID   string `json:"userID"`
	Nickname string `json:"nickname"`
	FaceURL  string `json:"faceURL"`
}

// RegisterUser 在 IM 侧开通用户，需要 admin token。
func (c *Client) RegisterUser(ctx context.Context, u UserInfo) error {
	if !c.Enabled() {
		return ErrDisabled
	}
	return c.doWithAdminToken(ctx, methodRegisterUser, "/user/user_register", registerRequest{
		Users: []registerUser{{
			UserID:   u.UserID,
			Nickname: u.Nickname,
			FaceURL:  u.AvatarURL,
		}},
	}, nil)
}
