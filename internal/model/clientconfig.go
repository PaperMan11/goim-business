// Package model 定义客户端配置的 MySQL 持久化记录。
package model

import "time"

// Config 是 client_configs 表中的一行配置。
type Config struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement"`
	Platform  int8      `gorm:"column:platform"`
	Version   int       `gorm:"column:version"`
	Content   string    `gorm:"column:content"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Config) TableName() string { return "client_configs" }
