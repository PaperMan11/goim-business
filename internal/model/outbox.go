// Package model 定义通用持久化记录。
package model

import "time"

// Outbox 是通用 outbox 任务（事务性 outbox），不属于任何业务域。
type Outbox struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement"`
	UserID      int64     `gorm:"column:user_id"`
	Action      string    `gorm:"column:action;size:32"`
	Payload     string    `gorm:"column:payload"`
	Status      int8      `gorm:"column:status"`
	Attempts    int       `gorm:"column:attempts"`
	NextRetryAt time.Time `gorm:"column:next_retry_at"`
	LastError   string    `gorm:"column:last_error;size:512"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (Outbox) TableName() string { return "outbox_events" }
