package api

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestOpenAPIAccountContract 是账号域最小 OpenAPI 契约的守护测试：
// 一旦 paths、schemas 或鉴权声明与实现脱节，直接失败。
func TestOpenAPIAccountContract(t *testing.T) {
	raw, err := os.ReadFile("openapi.yaml")
	require.NoError(t, err)

	var spec map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &spec))

	require.Equal(t, "3.0.3", spec["openapi"])
	require.Equal(t, "goim-business API", mustMap(t, spec["info"])["title"])
	require.Equal(t, "1.0.0", mustMap(t, spec["info"])["version"])

	servers, ok := spec["servers"].([]any)
	require.True(t, ok, "servers 必须是数组")
	require.Len(t, servers, 1)
	require.Equal(t, "/api/v1", mustMap(t, servers[0])["url"])

	paths := mustMap(t, spec["paths"])
	require.Len(t, paths, 25, "只允许描述 25 条路径：账号 7、IM 3、资料 3、设备 2、配置 2、文件 8")
	for _, path := range []string{
		"/account/register",
		"/account/login",
		"/account/refresh",
		"/account/logout",
		"/account/sessions",
		"/account/sessions/revoke",
		"/account/password",
		"/im/token",
		"/im/status",
		"/im/retry",
		"/profile/me",
		"/profile/batch",
		"/profile/users/{userId}",
		"/devices",
		"/devices/{deviceId}",
		"/client-config",
		"/client-config/schema",
		"/files/presign",
		"/files/multipart/presign",
		"/files/{fileId}/complete",
		"/files/{fileId}/multipart/complete",
		"/files/{fileId}/multipart/parts",
		"/files/{fileId}/multipart/abort",
		"/files/{fileId}/url",
		"/files/{fileId}",
	} {
		require.Contains(t, paths, path)
	}

	components := mustMap(t, spec["components"])

	schemas := mustMap(t, components["schemas"])
	for _, name := range []string{
		"RegisterRequest",
		"LoginRequest",
		"RefreshRequest",
		"RevokeSessionRequest",
		"ChangePasswordRequest",
		"AuthResponse",
		"UserView",
		"TokenView",
		"SessionView",
		"SessionViewList",
		"IMTokenResponse",
		"IMStatusResponse",
		"ProfileView",
		"PublicProfileView",
		"UpdateProfileRequest",
		"BatchProfilesRequest",
		"DeviceView",
		"ClientConfigResponse",
		"ClientConfigSchemaResponse",
		"FilePresignRequest",
		"FilePresignResponse",
		"FileView",
		"FileURLResponse",
		"MultipartPresignRequest",
		"MultipartPartsRequest",
		"MultipartPresignResponse",
		"MultipartPartPresign",
		"MultipartCompleteRequest",
		"MultipartCompletedPart",
		"ErrorResponse",
	} {
		require.Contains(t, schemas, name)
	}

	securitySchemes := mustMap(t, components["securitySchemes"])
	bearer, ok := securitySchemes["bearerAuth"].(map[string]any)
	require.True(t, ok, "必须声明 bearerAuth")
	require.Equal(t, "http", bearer["type"])
	require.Equal(t, "bearer", bearer["scheme"])
	require.Equal(t, "JWT", bearer["bearerFormat"])

	authed := []struct {
		path   string
		method string
	}{
		{"/account/logout", "post"},
		{"/account/sessions", "get"},
		{"/account/sessions/revoke", "post"},
		{"/account/password", "put"},
		{"/im/token", "post"},
		{"/im/status", "get"},
		{"/im/retry", "post"},
		{"/profile/me", "get"},
		{"/profile/me", "put"},
		{"/profile/batch", "post"},
		{"/profile/users/{userId}", "get"},
		{"/devices", "get"},
		{"/devices/{deviceId}", "delete"},
		{"/files/presign", "post"},
		{"/files/multipart/presign", "post"},
		{"/files/{fileId}/complete", "post"},
		{"/files/{fileId}/multipart/complete", "post"},
		{"/files/{fileId}/multipart/parts", "post"},
		{"/files/{fileId}/multipart/abort", "post"},
		{"/files/{fileId}/url", "get"},
		{"/files/{fileId}", "delete"},
	}
	for _, route := range authed {
		op := mustMap(t, mustMap(t, paths[route.path])[route.method])
		security, ok := op["security"].([]any)
		require.True(t, ok, "%s %s 必须声明 security", route.method, route.path)
		require.Len(t, security, 1)
		require.Contains(t, mustMap(t, security[0]), "bearerAuth",
			"%s %s 必须使用 bearerAuth", route.method, route.path)
	}

	// 客户端配置是公开接口，但必须声明 ETag 与 304。
	configOp := mustMap(t, mustMap(t, paths["/client-config"])["get"])
	require.NotContains(t, configOp, "security", "/client-config 不应要求鉴权")
	require.Contains(t, mustMap(t, configOp["responses"]), "304", "/client-config 必须声明 304")
}

func mustMap(t *testing.T, value any) map[string]any {
	t.Helper()
	m, ok := value.(map[string]any)
	require.True(t, ok, "期望 YAML 映射，实际为 %T", value)
	return m
}
