// Package types 定义跨层的请求与响应契约：请求结构同时作为 HTTP 入参与 service 入参，
// 响应结构只用于序列化，不暴露领域实体。
package types

import "time"

// RegisterRequest 是注册请求。
type RegisterRequest struct {
	LoginName string `json:"loginName" binding:"required,min=3,max=64"`
	Password  string `json:"password" binding:"required,max=64"`
	Nickname  string `json:"nickname" binding:"max=64"`

	DeviceID   string `json:"-"`
	AppVersion string `json:"-"`
	UserAgent  string `json:"-"`
	IP         string `json:"-"`
	Platform   int8   `json:"-"`
}

// LoginRequest 是登录请求。
type LoginRequest struct {
	LoginName string `json:"loginName" binding:"required,max=64"`
	Password  string `json:"password" binding:"required,max=64"`

	DeviceID   string `json:"-"`
	AppVersion string `json:"-"`
	UserAgent  string `json:"-"`
	IP         string `json:"-"`
	Platform   int8   `json:"-"`
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

// IMTokenResp 是 IM token 兑换的响应。
type IMTokenResp struct {
	UserID            string `json:"userId"`
	Token             string `json:"token"`
	ExpireTimeSeconds int64  `json:"expireTimeSeconds"`
	APIBaseURL        string `json:"apiBaseUrl"`
	WSURL             string `json:"wsUrl"`
	PlatformID        int8   `json:"platformId"`
}

// IMStatusResp 是 IM 账号开通状态的响应。
type IMStatusResp struct {
	UserID   string     `json:"userId"`
	Status   int8       `json:"status"`
	SyncedAt *time.Time `json:"syncedAt"`
}

// ProfileView 是当前用户的完整资料。
type ProfileView struct {
	UserID    string `json:"userId"`
	LoginName string `json:"loginName"`
	Nickname  string `json:"nickname"`
	AvatarKey string `json:"avatarKey"`
	Bio       string `json:"bio"`
}

// PublicProfileView 是他人可见的公开资料。
type PublicProfileView struct {
	UserID    string `json:"userId"`
	Nickname  string `json:"nickname"`
	AvatarKey string `json:"avatarKey"`
}

// UpdateProfileRequest 是更新资料请求；字段为 nil 表示不修改。
type UpdateProfileRequest struct {
	Nickname  *string `json:"nickname" binding:"omitempty,max=64"`
	AvatarKey *string `json:"avatarKey" binding:"omitempty,max=255"`
	Bio       *string `json:"bio" binding:"omitempty,max=255"`
}

// BatchProfilesRequest 是批量获取公开资料的请求。
type BatchProfilesRequest struct {
	UserIDs []string `json:"userIds" binding:"required,min=1,max=100,dive,required"`
}

// DeviceView 是对外暴露的设备信息，不含推送 token。
type DeviceView struct {
	DeviceID     string    `json:"deviceId"`
	Platform     int8      `json:"platform"`
	AppVersion   string    `json:"appVersion"`
	LastActiveAt time.Time `json:"lastActiveAt"`
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
