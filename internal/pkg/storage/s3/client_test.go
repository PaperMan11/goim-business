package s3

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testConfig() *Config {
	return &Config{
		Endpoint:       "http://127.0.0.1:9000",
		PublicEndpoint: "https://files.example.com",
		AccessKey:      "access",
		SecretKey:      "secret",
		Region:         "us-east-1",
		Bucket:         "goim-files",
		UsePathStyle:   true,
		PresignExpire:  15 * time.Minute,
	}
}

func TestNewS3ClientValidation(t *testing.T) {
	_, err := NewS3Client(context.Background(), nil)
	require.ErrorIs(t, err, ErrNilConfig)

	cfg := testConfig()
	cfg.Bucket = ""
	_, err = NewS3Client(context.Background(), cfg)
	require.ErrorIs(t, err, ErrEmptyBucket)
}

func TestNewS3ClientDefaultsPresignExpire(t *testing.T) {
	cfg := testConfig()
	cfg.PresignExpire = 0

	client, err := NewS3Client(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, time.Hour, cfg.PresignExpire)
}

func TestGenerateGetObjectPresignedURL(t *testing.T) {
	client, err := NewS3Client(context.Background(), testConfig())
	require.NoError(t, err)

	res, err := client.GenerateGetObjectPresignedURL(context.Background(), "avatar/123/abc.png", 0)
	require.NoError(t, err)
	require.NotEmpty(t, res.URL)

	u, err := url.Parse(res.URL)
	require.NoError(t, err)
	require.Equal(t, "files.example.com", u.Host)
	require.Equal(t, "/goim-files/avatar/123/abc.png", u.Path)
	require.NotEmpty(t, u.Query().Get("X-Amz-Signature"))
	require.Greater(t, res.ExpireAt, time.Now().UnixMilli())
}

func TestGeneratePutObjectPresignedURL(t *testing.T) {
	client, err := NewS3Client(context.Background(), testConfig())
	require.NoError(t, err)

	res, err := client.GeneratePutObjectPresignedURL(context.Background(), "avatar/123/abc.png", "image/png", 0)
	require.NoError(t, err)
	require.NotEmpty(t, res.URL)

	u, err := url.Parse(res.URL)
	require.NoError(t, err)
	require.Equal(t, "files.example.com", u.Host)
	require.Equal(t, "/goim-files/avatar/123/abc.png", u.Path)
	require.Contains(t, u.Query().Get("X-Amz-SignedHeaders"), "content-type")
	require.NotEmpty(t, u.Query().Get("X-Amz-Signature"))
}

func TestGeneratePostFormPresignedData(t *testing.T) {
	client, err := NewS3Client(context.Background(), testConfig())
	require.NoError(t, err)

	res, err := client.GeneratePostFormPresignedData(context.Background(), "avatar/123/abc.png", "image/png")
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(res.URL, "/goim-files"), "POST action 应指向 bucket: %s", res.URL)
	require.NotEmpty(t, res.FormData)
	require.NotEmpty(t, res.FormData["key"])
	require.Greater(t, res.ExpireAt, time.Now().UnixMilli())
}
