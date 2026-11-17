// Package clientconfig 提供客户端启动配置：按平台读取配置、版本化为 ETag 并支持 304。
package clientconfig

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound 表示配置表中没有任何可用配置。
var ErrNotFound = errors.New("客户端配置不存在")

// Config 是一份客户端配置；Platform=0 表示全平台默认。
type Config struct {
	Platform      int8
	Version       int
	Features      map[string]bool
	Limits        map[string]int64
	MinAppVersion map[string]string
	UpdatedAt     time.Time
}

// Repository 是客户端配置仓储端口。
type Repository interface {
	// GetByPlatform 返回指定平台配置；该平台缺少记录时回退全平台默认（platform=0）。
	GetByPlatform(ctx context.Context, platform int8) (*Config, error)
}
