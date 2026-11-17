package clientconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	domain "goim-business/internal/domain/clientconfig"
)

type fakeRepo struct {
	cfg *domain.Config
	err error
}

func (f *fakeRepo) GetByPlatform(_ context.Context, _ int8) (*domain.Config, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.cfg, nil
}

func newService(repo domain.Repository) *Service {
	return NewService(repo, Config{IMAPIBaseURL: "https://im.example.com/api/v1", IMWSURL: "wss://im.example.com/ws"}, zap.NewNop().Sugar())
}

func TestGetUsesStoredConfig(t *testing.T) {
	repo := &fakeRepo{cfg: &domain.Config{
		Platform:      5,
		Version:       7,
		Features:      map[string]bool{"voiceMessage": true},
		Limits:        map[string]int64{"maxUploadBytes": 1024},
		MinAppVersion: map[string]string{"web": "1.2.0"},
		UpdatedAt:     time.Now(),
	}}
	svc := newService(repo)

	view, etag, err := svc.Get(context.Background(), 5)
	require.NoError(t, err)
	require.Equal(t, 7, view.ConfigVersion)
	require.True(t, view.Features["voiceMessage"])
	require.Equal(t, int64(1024), view.Limits["maxUploadBytes"])
	require.Equal(t, "1.2.0", view.MinAppVersion["web"])
	require.Equal(t, int8(5), view.IM.PlatformID)
	require.Equal(t, "https://im.example.com/api/v1", view.IM.APIBaseURL)
	require.Equal(t, `"cfg-5-7"`, etag)
}

func TestGetFallsBackToDefaults(t *testing.T) {
	svc := newService(&fakeRepo{err: domain.ErrNotFound})

	view, etag, err := svc.Get(context.Background(), 3)
	require.NoError(t, err)
	require.Equal(t, 1, view.ConfigVersion)
	require.False(t, view.Features["voiceMessage"])
	require.True(t, view.Features["fileUpload"])
	require.Equal(t, int64(104857600), view.Limits["maxUploadBytes"])
	require.Equal(t, "1.0.0", view.MinAppVersion["web"])
	require.Equal(t, `"cfg-3-1"`, etag)
}

func TestGetReturnsDependencyOnRepoError(t *testing.T) {
	svc := newService(&fakeRepo{err: errors.New("db down")})

	_, _, err := svc.Get(context.Background(), 5)
	require.Error(t, err)
}

func TestSchemaMatchesDefaults(t *testing.T) {
	svc := newService(&fakeRepo{err: domain.ErrNotFound})

	schema := svc.Schema()
	require.Len(t, schema.Items, 8)
	for _, item := range schema.Items {
		switch item.Key {
		case "features.voiceMessage":
			require.Equal(t, false, item.Default)
		case "limits.maxAvatarBytes":
			require.Equal(t, int64(5242880), item.Default)
		case "im.apiBaseUrl":
			require.Equal(t, "https://im.example.com/api/v1", item.Default)
		}
	}
}
