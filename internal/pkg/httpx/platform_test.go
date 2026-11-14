package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPlatformFrom(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		header  string
		want    int8
		wantErr bool
	}{
		{"", PlatformWeb, false},
		{"web", PlatformWeb, false},
		{"windows", PlatformWindows, false},
		{"macos", PlatformMacOS, false},
		{"ios", PlatformIOS, false},
		{"android", PlatformAndroid, false},
		{"harmonyos", PlatformHarmonyOS, false},
		{"5", PlatformWeb, false},
		{"11", PlatformHarmonyOS, false},
		{"99", 0, true},
		{"unknown", 0, true},
	}

	for _, tc := range cases {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		if tc.header != "" {
			c.Request.Header.Set("X-Client-Platform", tc.header)
		}

		got, err := PlatformFrom(c)
		if tc.wantErr {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
}
