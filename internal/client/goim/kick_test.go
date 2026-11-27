package goim

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestForceLogoutSendsDeviceIDs(t *testing.T) {
	var body forceLogoutRequest
	var token string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case apiPrefix + "/auth/get_admin_token":
			writeResponse(w, 0, "ok", map[string]any{"token": "admin-token", "expireTimeSeconds": 300})
		case apiPrefix + "/auth/force_logout":
			token = r.Header.Get(headerToken)
			_ = json.NewDecoder(r.Body).Decode(&body)
			writeResponse(w, 0, "ok", nil)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, AdminUserID: "admin", AdminSecret: "secret"})
	require.NoError(t, c.ForceLogout(context.Background(), " 42 ", 5, []string{"device-a"}))

	require.Equal(t, "42", body.UserID)
	require.Equal(t, int32(5), body.PlatformID)
	require.Equal(t, []string{"device-a"}, body.DeviceIDs)
	require.Equal(t, "admin-token", token)
}

func TestForceLogoutDisabled(t *testing.T) {
	c := New(Config{})
	require.ErrorIs(t, c.ForceLogout(context.Background(), "42", 5, nil), ErrDisabled)
}

func TestForceLogoutRejectsEmptyUserID(t *testing.T) {
	c := New(Config{BaseURL: "http://example.invalid", AdminUserID: "admin", AdminSecret: "secret"})
	require.Error(t, c.ForceLogout(context.Background(), "  ", 5, nil))
}
