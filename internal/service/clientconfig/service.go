// Package clientconfig 提供客户端启动配置：按平台读取、版本化 ETag 与配置项元数据。
package clientconfig

import (
	"context"
	"errors"
	"fmt"

	domain "goim-business/internal/domain/clientconfig"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/pkg/logger"
	"goim-business/internal/types"
)

// Config 是配置服务需要的运行期参数。
type Config struct {
	IMAPIBaseURL string
	IMWSURL      string
}

// Service 是客户端配置服务。
type Service struct {
	repo domain.Repository
	cfg  Config
	log  logger.Logger
}

// NewService 构造客户端配置服务。
func NewService(repo domain.Repository, cfg Config, log logger.Logger) *Service {
	return &Service{repo: repo, cfg: cfg, log: log}
}

// Get 返回指定平台的配置与 ETag；配置表缺失时回退代码内置默认值。
func (s *Service) Get(ctx context.Context, platform int8) (*types.ClientConfigView, string, error) {
	cfg, err := s.repo.GetByPlatform(ctx, platform)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			s.log.Errorf("读取客户端配置失败: platform=%d, err=%v", platform, err)
			return nil, "", errcode.ErrDependency
		}
		s.log.Warnf("客户端配置缺失，使用内置默认值: platform=%d", platform)
		cfg = defaultConfig()
	}

	view := &types.ClientConfigView{
		ConfigVersion: cfg.Version,
		Features:      cfg.Features,
		IM: types.ClientConfigIM{
			APIBaseURL: s.cfg.IMAPIBaseURL,
			WSURL:      s.cfg.IMWSURL,
			PlatformID: platform,
		},
		Limits:        cfg.Limits,
		MinAppVersion: cfg.MinAppVersion,
	}
	etag := fmt.Sprintf(`"cfg-%d-%d"`, platform, cfg.Version)
	return view, etag, nil
}

// Schema 返回配置项元数据；与默认值保持一致，供客户端渲染设置页与校验输入。
func (s *Service) Schema() *types.ClientConfigSchemaView {
	defaults := defaultConfig()
	return &types.ClientConfigSchemaView{Items: []types.ClientConfigItemSchema{
		{Key: "features.voiceMessage", Type: "bool", Default: defaults.Features["voiceMessage"], Scope: "all"},
		{Key: "features.fileUpload", Type: "bool", Default: defaults.Features["fileUpload"], Scope: "all"},
		{Key: "limits.maxUploadBytes", Type: "int64", Default: defaults.Limits["maxUploadBytes"], Scope: "all"},
		{Key: "limits.maxAvatarBytes", Type: "int64", Default: defaults.Limits["maxAvatarBytes"], Scope: "all"},
		{Key: "minAppVersion.web", Type: "string", Default: defaults.MinAppVersion["web"], Scope: "web"},
		{Key: "minAppVersion.desktop", Type: "string", Default: defaults.MinAppVersion["desktop"], Scope: "desktop"},
		{Key: "im.apiBaseUrl", Type: "string", Default: s.cfg.IMAPIBaseURL, Scope: "runtime"},
		{Key: "im.wsUrl", Type: "string", Default: s.cfg.IMWSURL, Scope: "runtime"},
	}}
}

func defaultConfig() *domain.Config {
	return &domain.Config{
		Version: 1,
		Features: map[string]bool{
			"voiceMessage": false,
			"fileUpload":   true,
		},
		Limits: map[string]int64{
			"maxUploadBytes": 104857600,
			"maxAvatarBytes": 5242880,
		},
		MinAppVersion: map[string]string{
			"web":     "1.0.0",
			"desktop": "1.0.0",
		},
	}
}
