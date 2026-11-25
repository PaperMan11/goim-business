// Package im 把 goim im-api 客户端适配为业务侧端口：账号域的 IM 操作与文件域的群成员查询。
package im

import (
	"context"
	"errors"

	"goim-business/internal/client/goim"
	"goim-business/internal/service/ports"
)

// Client 把 goim 客户端适配为账号域的 IM 端口。
type Client struct {
	client *goim.Client
}

// NewClient 构造 IM 端口适配器。
func NewClient(client *goim.Client) *Client {
	return &Client{client: client}
}

// Enabled 表示 IM 是否已配置。
func (c *Client) Enabled() bool { return c.client.Enabled() }

// RegisterUser 在 IM 侧开通用户；账号已存在映射为端口错误，由服务层按幂等处理。
func (c *Client) RegisterUser(ctx context.Context, userID, nickname, avatarURL string) error {
	err := c.client.RegisterUser(ctx, goim.UserInfo{UserID: userID, Nickname: nickname, AvatarURL: avatarURL})
	if errors.Is(err, goim.ErrUserAlreadyExists) {
		return ports.ErrIMUserAlreadyExists
	}
	return err
}

// UpdateUserInfo 把昵称与头像同步到 IM；用户不存在映射为端口错误，注销占位可视为完成。
func (c *Client) UpdateUserInfo(ctx context.Context, userID, nickname, avatarURL string) error {
	err := c.client.UpdateUserInfo(ctx, goim.UserInfo{UserID: userID, Nickname: nickname, AvatarURL: avatarURL})
	if errors.Is(err, goim.ErrUserNotFound) {
		return ports.ErrIMUserNotFound
	}
	return err
}

// UserToken 为指定用户与平台兑换 IM token。
func (c *Client) UserToken(ctx context.Context, userID string, platformID int) (*ports.IMToken, error) {
	tok, err := c.client.UserToken(ctx, userID, platformID)
	if err != nil {
		return nil, err
	}
	return &ports.IMToken{Value: tok.Value, ExpireAt: tok.ExpireAt}, nil
}
