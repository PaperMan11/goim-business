package goim

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupMemberUserIDs(t *testing.T) {
	var body groupMemberUserIDsRequest
	var token string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case apiPrefix + "/auth/get_admin_token":
			writeResponse(w, 0, "ok", map[string]any{"token": "admin-token", "expireTimeSeconds": 300})
		case apiPrefix + "/group/get_group_member_user_id":
			token = r.Header.Get(headerToken)
			_ = json.NewDecoder(r.Body).Decode(&body)
			writeResponse(w, 0, "ok", map[string]any{"userIDs": []string{"42", "99"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, AdminUserID: "admin", AdminSecret: "secret"})
	members, err := c.GroupMemberUserIDs(context.Background(), " 1001 ")
	require.NoError(t, err)
	require.Equal(t, []string{"42", "99"}, members)
	require.Equal(t, "1001", body.GroupID)
	require.Equal(t, "admin-token", token)
}

func TestGroupMemberUserIDsDisabled(t *testing.T) {
	c := New(Config{})
	_, err := c.GroupMemberUserIDs(context.Background(), "1001")
	require.ErrorIs(t, err, ErrDisabled)
}

func TestGroupMemberUserIDsRejectsEmptyGroupID(t *testing.T) {
	c := New(Config{BaseURL: "http://example.invalid", AdminUserID: "admin", AdminSecret: "secret"})
	_, err := c.GroupMemberUserIDs(context.Background(), "   ")
	require.Error(t, err)
}
