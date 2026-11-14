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
	require.Len(t, paths, 7, "只允许描述已实现的 7 个账号接口")
	for _, path := range []string{
		"/account/register",
		"/account/login",
		"/account/refresh",
		"/account/logout",
		"/account/sessions",
		"/account/sessions/revoke",
		"/account/password",
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

	authed := map[string]string{
		"/account/logout":          "post",
		"/account/sessions":        "get",
		"/account/sessions/revoke": "post",
		"/account/password":        "put",
	}
	for path, method := range authed {
		op := mustMap(t, mustMap(t, paths[path])[method])
		security, ok := op["security"].([]any)
		require.True(t, ok, "%s %s 必须声明 security", method, path)
		require.Len(t, security, 1)
		require.Contains(t, mustMap(t, security[0]), "bearerAuth",
			"%s %s 必须使用 bearerAuth", method, path)
	}
}

func mustMap(t *testing.T, value any) map[string]any {
	t.Helper()
	m, ok := value.(map[string]any)
	require.True(t, ok, "期望 YAML 映射，实际为 %T", value)
	return m
}
