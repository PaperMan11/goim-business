package goim

import (
	"context"
	"errors"
)

// codeUserAlreadyExists 是 goim 对“用户已注册”返回的业务码。
const codeUserAlreadyExists = 1102

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
//
// 幂等语义：goim 返回业务码 1102（用户已注册）视为开通成功并返回 nil。
// 该场景通常发生在“IM 已创建成功但响应丢失”后的重试，若按失败处理会让用户永久卡在未开通状态；
// 其余业务码或网络错误仍返回 *APIError/原始错误，供上层决定是否重试。
func (c *Client) RegisterUser(ctx context.Context, u UserInfo) error {
	if !c.Enabled() {
		return ErrDisabled
	}
	err := c.doWithAdminToken(ctx, methodRegisterUser, "/user/user_register", registerRequest{
		Users: []registerUser{{
			UserID:   u.UserID,
			Nickname: u.Nickname,
			FaceURL:  u.AvatarURL,
		}},
	}, nil)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Code == codeUserAlreadyExists {
		return nil
	}
	return err
}
