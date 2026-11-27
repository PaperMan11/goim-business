package goim

import (
	"context"
	"errors"
	"strings"
)

// forceLogoutRequest 是 /auth/force_logout 的请求体；deviceIDs 为空时按平台踢。
type forceLogoutRequest struct {
	UserID     string   `json:"userID"`
	PlatformID int32    `json:"platformID"`
	DeviceIDs  []string `json:"deviceIDs,omitempty"`
}

// ForceLogout 踢出指定用户设备：断开匹配的 WS 连接并按平台失效 IM token。
// 需要 admin token；业务侧的会话撤销不依赖该调用成功。
func (c *Client) ForceLogout(ctx context.Context, userID string, platformID int32, deviceIDs []string) error {
	if !c.Enabled() {
		return ErrDisabled
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return errors.New("goim 强制下线缺少 userID")
	}
	return c.doWithAdminToken(ctx, methodForceLogout, "/auth/force_logout", forceLogoutRequest{
		UserID:     userID,
		PlatformID: platformID,
		DeviceIDs:  deviceIDs,
	}, nil)
}
