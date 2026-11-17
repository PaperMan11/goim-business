// Package clientconfig 封装 client_configs 表的 MySQL 数据访问。
package clientconfig

import (
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	domain "goim-business/internal/domain/clientconfig"
	"goim-business/internal/model"
	"goim-business/internal/repository"
)

type content struct {
	Features      map[string]bool   `json:"features"`
	Limits        map[string]int64  `json:"limits"`
	MinAppVersion map[string]string `json:"minAppVersion"`
}

// Repo 是 client_configs 表的仓储实现。
type Repo struct {
	db *gorm.DB
}

// New 构造客户端配置仓储。
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

func (r *Repo) dbFrom(ctx context.Context) *gorm.DB {
	return repository.DBFromContext(ctx, r.db)
}

// GetByPlatform 优先读取指定平台配置，缺失时回退到全平台默认（platform=0）。
func (r *Repo) GetByPlatform(ctx context.Context, platform int8) (*domain.Config, error) {
	var record model.Config
	err := r.dbFrom(ctx).
		Where("platform IN ?", []int8{0, platform}).
		Order("platform DESC").
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	var cfg content
	if record.Content != "" {
		if err := json.Unmarshal([]byte(record.Content), &cfg); err != nil {
			return nil, err
		}
	}
	return &domain.Config{
		Platform:      record.Platform,
		Version:       record.Version,
		Features:      cfg.Features,
		Limits:        cfg.Limits,
		MinAppVersion: cfg.MinAppVersion,
		UpdatedAt:     record.UpdatedAt,
	}, nil
}
