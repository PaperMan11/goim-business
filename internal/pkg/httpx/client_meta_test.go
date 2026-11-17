package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func testContext(header, value string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	if value != "" {
		c.Request.Header.Set(header, value)
	}
	return c
}

func TestDeviceIDFrom(t *testing.T) {
	gin.SetMode(gin.TestMode)

	_, err := DeviceIDFrom(testContext("", ""))
	require.Error(t, err)

	_, err = DeviceIDFrom(testContext("X-Device-Id", strings.Repeat("a", 65)))
	require.Error(t, err)

	id, err := DeviceIDFrom(testContext("X-Device-Id", "  web-device-001  "))
	require.NoError(t, err)
	require.Equal(t, "web-device-001", id)
}

func TestClientVersionFrom(t *testing.T) {
	gin.SetMode(gin.TestMode)

	version, err := ClientVersionFrom(testContext("", ""))
	require.NoError(t, err)
	require.Empty(t, version)

	_, err = ClientVersionFrom(testContext("X-Client-Version", strings.Repeat("1", 33)))
	require.Error(t, err)

	version, err = ClientVersionFrom(testContext("X-Client-Version", " 1.0.0 "))
	require.NoError(t, err)
	require.Equal(t, "1.0.0", version)
}
