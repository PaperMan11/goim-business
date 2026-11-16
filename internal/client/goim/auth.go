package goim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Token 是 im-api 签发的用户 token 及到期时间。
type Token struct {
	Value    string
	ExpireAt time.Time
}

// adminTokenRequest 是 /auth/get_admin_token 的请求体。
type adminTokenRequest struct {
	Secret string `json:"secret"`
	UserID string `json:"userID"`
}

// userTokenRequest 是 /auth/get_user_token 的请求体。
type userTokenRequest struct {
	UserID     string `json:"userID"`
	PlatformID int    `json:"platformID"`
}

// UserToken 为指定用户与平台兑换 IM token，需要 admin token。
func (c *Client) UserToken(ctx context.Context, userID string, platformID int) (*Token, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	var resp tokenResponse
	if err := c.doWithAdminToken(ctx, methodUserToken, "/auth/get_user_token", userTokenRequest{
		UserID:     userID,
		PlatformID: platformID,
	}, &resp); err != nil {
		return nil, err
	}
	return &Token{
		Value:    resp.Token,
		ExpireAt: c.now().Add(durationFromSeconds(resp.ExpireTimeSeconds)),
	}, nil
}

// doWithAdminToken 携带 admin token 调用接口；业务码 1002 视为 token 失效，
// 失效缓存后重新取 token 并只重试一次。
func (c *Client) doWithAdminToken(ctx context.Context, method, path string, body, out any) error {
	token, err := c.adminTokenValue(ctx)
	if err != nil {
		return err
	}

	data, err := c.post(ctx, method, path, token, body)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Code == codeNoPermission {
			c.invalidateAdminToken(token)
			token, err = c.adminTokenValue(ctx)
			if err != nil {
				return err
			}
			data, err = c.post(ctx, method, path, token, body)
		}
		if err != nil {
			return err
		}
	}

	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("goim 响应解析失败: %w", err)
		}
	}
	return nil
}

// adminTokenValue 返回有效的 admin token，必要时在锁内换取并缓存。
func (c *Client) adminTokenValue(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 锁内二次检查：并发请求中只有一个会真正发起换取。
	if c.adminToken != "" && c.now().Before(c.adminTokenExpireAt.Add(-c.tokenRefreshAhead)) {
		return c.adminToken, nil
	}
	return c.fetchAdminToken(ctx)
}

// fetchAdminToken 调用 /auth/get_admin_token，调用方必须持有锁。
func (c *Client) fetchAdminToken(ctx context.Context) (string, error) {
	data, err := c.post(ctx, methodAdminToken, "/auth/get_admin_token", "", adminTokenRequest{
		Secret: c.adminSecret,
		UserID: c.adminUserID,
	})
	if err != nil {
		return "", err
	}

	var resp tokenResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("goim 响应解析失败: %w", err)
	}
	if resp.Token == "" {
		return "", errors.New("goim admin token 响应缺少 token")
	}

	c.adminToken = resp.Token
	c.adminTokenExpireAt = c.now().Add(durationFromSeconds(resp.ExpireTimeSeconds))
	return c.adminToken, nil
}

// invalidateAdminToken 仅清除与失效请求匹配的缓存，避免并发刷新时误删新 token。
func (c *Client) invalidateAdminToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.adminToken == token {
		c.adminToken = ""
		c.adminTokenExpireAt = time.Time{}
	}
}
