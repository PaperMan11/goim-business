package account

import "time"

// Credential 是账号密码凭证。
type Credential struct {
	UserID       int64
	PasswordHash string
	UpdatedAt    time.Time
}
