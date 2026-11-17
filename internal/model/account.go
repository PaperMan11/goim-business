// Package model 定义账号相关的 MySQL 持久化记录。
package model

import (
	"time"
)

// 用户状态。
const (
	UserStatusNormal   int8 = 1
	UserStatusFrozen   int8 = 2
	UserStatusDeleting int8 = 3
	UserStatusDeleted  int8 = 4
)

// IM 账号开通状态。
const (
	IMStatusPending int8 = 0
	IMStatusReady   int8 = 1
	IMStatusFailed  int8 = 2
)

// User 是业务账号，ID 为自增主键。
type User struct {
	ID          int64      `gorm:"column:id;primaryKey;autoIncrement"`
	LoginName   string     `gorm:"column:login_name;size:64"`
	Nickname    string     `gorm:"column:nickname;size:64"`
	AvatarKey   string     `gorm:"column:avatar_key;size:255"`
	Bio         string     `gorm:"column:bio;size:255"`
	Status      int8       `gorm:"column:status"`
	IMStatus    int8       `gorm:"column:im_status"`
	IMSyncedAt  *time.Time `gorm:"column:im_synced_at"`
	LastLoginAt *time.Time `gorm:"column:last_login_at"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
}

func (User) TableName() string { return "account_users" }

// Credential 是账号密码凭证。
type Credential struct {
	UserID       int64     `gorm:"column:user_id;primaryKey"`
	PasswordHash string    `gorm:"column:password_hash;size:255"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (Credential) TableName() string { return "account_credentials" }

// Session 是一个登录会话；刷新时旧行标记 rotated_at，新行继承 family_id。
type Session struct {
	ID          int64      `gorm:"column:id;primaryKey;autoIncrement"`
	UserID      int64      `gorm:"column:user_id"`
	FamilyID    string     `gorm:"column:family_id;size:36"`
	RefreshHash string     `gorm:"column:refresh_hash;size:64"`
	RotatedFrom *int64     `gorm:"column:rotated_from"`
	Platform    int8       `gorm:"column:platform"`
	DeviceID    string     `gorm:"column:device_id;size:64"`
	UserAgent   string     `gorm:"column:user_agent;size:255"`
	IP          string     `gorm:"column:ip;size:45"`
	ExpiresAt   time.Time  `gorm:"column:expires_at"`
	RevokedAt   *time.Time `gorm:"column:revoked_at"`
	RotatedAt   *time.Time `gorm:"column:rotated_at"`
	LastSeenAt  *time.Time `gorm:"column:last_seen_at"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
}

func (Session) TableName() string { return "account_sessions" }

// Device 是用户设备登记记录。
type Device struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement"`
	UserID       int64     `gorm:"column:user_id"`
	DeviceID     string    `gorm:"column:device_id;size:64"`
	Platform     int8      `gorm:"column:platform"`
	AppVersion   string    `gorm:"column:app_version;size:32"`
	PushToken    string    `gorm:"column:push_token;size:255"`
	LastActiveAt time.Time `gorm:"column:last_active_at"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (Device) TableName() string { return "account_devices" }
