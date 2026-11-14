// Package types 定义跨层的请求与响应契约：请求结构同时作为 HTTP 入参与 service 入参，
// 响应结构只用于序列化，不暴露领域实体。
package types

import "time"

// RegisterRequest 是注册请求。
type RegisterRequest struct {
	LoginName string `json:"loginName" binding:"required,min=3,max=64"`
	Password  string `json:"password" binding:"required,max=64"`
	Nickname  string `json:"nickname" binding:"max=64"`
	DeviceID  string `json:"deviceId" binding:"required,max=64"`

	UserAgent string `json:"-"`
	IP        string `json:"-"`
	Platform  int8   `json:"-"`
}

// LoginRequest 是登录请求。
type LoginRequest struct {
	LoginName string `json:"loginName" binding:"required,max=64"`
	Password  string `json:"password" binding:"required,max=64"`
	DeviceID  string `json:"deviceId" binding:"required,max=64"`

	UserAgent string `json:"-"`
	IP        string `json:"-"`
	Platform  int8   `json:"-"`
}

// RefreshRequest 是刷新令牌请求。
type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`

	UserAgent string `json:"-"`
	IP        string `json:"-"`
}

// RevokeSessionRequest 是撤销指定会话的请求。
type RevokeSessionRequest struct {
	SessionID string `json:"sessionId" binding:"required"`
}

// ChangePasswordRequest 是修改密码请求。
type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required,max=64"`
	NewPassword string `json:"newPassword" binding:"required,min=8,max=64"`
}

// SessionView 是对外暴露的会话信息（不含任何令牌）。
type SessionView struct {
	SessionID  string     `json:"sessionId"`
	Platform   int8       `json:"platform"`
	DeviceID   string     `json:"deviceId"`
	UserAgent  string     `json:"userAgent"`
	IP         string     `json:"ip"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastSeenAt *time.Time `json:"lastSeenAt"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	Current    bool       `json:"current"`
}

// UserView 是账号基础信息。
type UserView struct {
	UserID    string `json:"userId"`
	LoginName string `json:"loginName"`
	Nickname  string `json:"nickname"`
}

// TokenView 是本次签发的访问令牌。
type TokenView struct {
	AccessToken string `json:"accessToken"`
	ExpiresIn   int64  `json:"expiresIn"`
}

// AuthResp 是注册/登录/刷新的响应。
type AuthResp struct {
	User             UserView  `json:"user"`
	Token            TokenView `json:"token"`
	RefreshToken     string    `json:"refreshToken,omitempty"`
	RefreshExpiresIn int64     `json:"refreshExpiresIn"`
	SessionID        string    `json:"sessionId"`
	Platform         int8      `json:"platform"`
}
