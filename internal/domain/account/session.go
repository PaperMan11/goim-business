package account

import "time"

// Session 是一次登录会话；刷新时旧会话被轮换，新会话继承 FamilyID。
type Session struct {
	ID          int64
	UserID      int64
	FamilyID    string
	RefreshHash string
	RotatedFrom *int64
	Platform    int8
	DeviceID    string
	UserAgent   string
	IP          string
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	RotatedAt   *time.Time
	LastSeenAt  *time.Time
	CreatedAt   time.Time
}

// IsReplay 判断该会话是否已被轮换，即 refresh token 被重复使用。
func (s *Session) IsReplay() bool {
	return s.RotatedAt != nil
}

// IsActive 判断会话在给定时刻是否仍可用。
func (s *Session) IsActive(at time.Time) bool {
	return s.RevokedAt == nil && s.RotatedAt == nil && at.Before(s.ExpiresAt)
}
