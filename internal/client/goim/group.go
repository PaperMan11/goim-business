package goim

import (
	"context"
	"errors"
	"strings"
)

// groupMemberUserIDsRequest 是 /group/get_group_member_user_id 的请求体。
type groupMemberUserIDsRequest struct {
	GroupID string `json:"groupID"`
}

// groupMemberUserIDsResponse 是群成员用户 ID 列表。
type groupMemberUserIDsResponse struct {
	UserIDs []string `json:"userIDs"`
}

// GroupMemberUserIDs 查询群成员用户 ID 列表，用于聊天文件的会话成员鉴权。
func (c *Client) GroupMemberUserIDs(ctx context.Context, groupID string) ([]string, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, errors.New("goim 群成员查询缺少 groupID")
	}
	var resp groupMemberUserIDsResponse
	if err := c.doWithAdminToken(ctx, methodGroupMemberUserIDs, "/group/get_group_member_user_id", groupMemberUserIDsRequest{GroupID: groupID}, &resp); err != nil {
		return nil, err
	}
	return resp.UserIDs, nil
}
