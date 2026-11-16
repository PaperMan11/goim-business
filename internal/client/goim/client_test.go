package goim

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// callRecord 汇总测试服务端收到的请求，便于在测试主 goroutine 中断言。
type callRecord struct {
	mu              sync.Mutex
	adminCalls      int
	registerCalls   int
	userTokenCalls  int
	registerTokens  []string
	registerOpIDs   []string
	registerBodies  []registerRequest
	userTokenBodies []userTokenRequest
}

func (r *callRecord) recordAdmin() {
	r.mu.Lock()
	r.adminCalls++
	r.mu.Unlock()
}

func (r *callRecord) recordRegister(req *http.Request, body registerRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registerCalls++
	r.registerTokens = append(r.registerTokens, req.Header.Get(headerToken))
	r.registerOpIDs = append(r.registerOpIDs, req.Header.Get(headerOperationID))
	r.registerBodies = append(r.registerBodies, body)
}

func (r *callRecord) recordUserToken(req *http.Request, body userTokenRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.userTokenCalls++
	r.userTokenBodies = append(r.userTokenBodies, body)
}

func (r *callRecord) counts() (int, int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.adminCalls, r.registerCalls, r.userTokenCalls
}

func (r *callRecord) registerSnapshot() ([]string, []string, []registerRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	tokens := append([]string(nil), r.registerTokens...)
	opIDs := append([]string(nil), r.registerOpIDs...)
	bodies := append([]registerRequest(nil), r.registerBodies...)
	return tokens, opIDs, bodies
}

func (r *callRecord) userTokenSnapshot() []userTokenRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]userTokenRequest(nil), r.userTokenBodies...)
}

// writeResponse 输出 im-api 的统一响应包体。
func writeResponse(w http.ResponseWriter, code int, msg string, data any) {
	resp := apiResponse{Code: code, Msg: msg}
	if data != nil {
		raw, err := json.Marshal(data)
		if err == nil {
			resp.Data = raw
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func TestRegisterUserCachesAdminToken(t *testing.T) {
	rec := new(callRecord)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case apiPrefix + "/auth/get_admin_token":
			rec.recordAdmin()
			writeResponse(w, 0, "ok", map[string]any{"token": "admin-token", "expireTimeSeconds": 300})
		case apiPrefix + "/user/user_register":
			var body registerRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			rec.recordRegister(r, body)
			writeResponse(w, 0, "ok", nil)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, AdminUserID: "admin", AdminSecret: "secret"})
	ctx := context.Background()
	require.NoError(t, c.RegisterUser(ctx, UserInfo{UserID: "1"}))
	require.NoError(t, c.RegisterUser(ctx, UserInfo{UserID: "2"}))

	adminCalls, registerCalls, _ := rec.counts()
	require.Equal(t, 1, adminCalls)
	require.Equal(t, 2, registerCalls)
}

func TestAdminTokenRefreshesAfterExpiry(t *testing.T) {
	rec := new(callRecord)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case apiPrefix + "/auth/get_admin_token":
			rec.recordAdmin()
			writeResponse(w, 0, "ok", map[string]any{"token": "admin-token", "expireTimeSeconds": 300})
		case apiPrefix + "/user/user_register":
			var body registerRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			rec.recordRegister(r, body)
			writeResponse(w, 0, "ok", nil)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	now := time.Now()
	c := New(Config{BaseURL: srv.URL, AdminUserID: "admin", AdminSecret: "secret"})
	c.now = func() time.Time { return now }

	ctx := context.Background()
	require.NoError(t, c.RegisterUser(ctx, UserInfo{UserID: "1"}))

	// 300s 有效期、提前 60s 刷新：越过 240s 后必须重新换取。
	now = now.Add(241 * time.Second)
	require.NoError(t, c.RegisterUser(ctx, UserInfo{UserID: "2"}))

	adminCalls, registerCalls, _ := rec.counts()
	require.Equal(t, 2, adminCalls)
	require.Equal(t, 2, registerCalls)
}

func TestRegisterUserSendsHeadersAndBody(t *testing.T) {
	rec := new(callRecord)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case apiPrefix + "/auth/get_admin_token":
			rec.recordAdmin()
			writeResponse(w, 0, "ok", map[string]any{"token": "admin-token", "expireTimeSeconds": 300})
		case apiPrefix + "/user/user_register":
			var body registerRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			rec.recordRegister(r, body)
			writeResponse(w, 0, "ok", nil)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, AdminUserID: "admin", AdminSecret: "secret"})
	ctx := context.Background()
	require.NoError(t, c.RegisterUser(ctx, UserInfo{
		UserID:    "1001",
		Nickname:  "alice",
		AvatarURL: "https://cdn.example.com/a.png",
	}))
	require.NoError(t, c.RegisterUser(ctx, UserInfo{UserID: "1002"}))

	tokens, opIDs, bodies := rec.registerSnapshot()
	require.Len(t, tokens, 2)
	require.Equal(t, "admin-token", tokens[0])
	require.Equal(t, "admin-token", tokens[1])

	require.Len(t, opIDs, 2)
	require.NotEmpty(t, opIDs[0])
	require.NotEmpty(t, opIDs[1])
	require.NotEqual(t, opIDs[0], opIDs[1])

	require.Len(t, bodies, 2)
	require.Len(t, bodies[0].Users, 1)
	require.Equal(t, "1001", bodies[0].Users[0].UserID)
	require.Equal(t, "alice", bodies[0].Users[0].Nickname)
	require.Equal(t, "https://cdn.example.com/a.png", bodies[0].Users[0].FaceURL)
}

func TestUserTokenParsesResponseAndPassesPlatformID(t *testing.T) {
	rec := new(callRecord)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case apiPrefix + "/auth/get_admin_token":
			rec.recordAdmin()
			writeResponse(w, 0, "ok", map[string]any{"token": "admin-token", "expireTimeSeconds": 300})
		case apiPrefix + "/auth/get_user_token":
			var body userTokenRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			rec.recordUserToken(r, body)
			writeResponse(w, 0, "ok", map[string]any{"token": "user-token-abc", "expireTimeSeconds": 7200})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, AdminUserID: "admin", AdminSecret: "secret"})
	token, err := c.UserToken(context.Background(), "42", 5)
	require.NoError(t, err)
	require.Equal(t, "user-token-abc", token.Value)
	require.WithinDuration(t, time.Now().Add(7200*time.Second), token.ExpireAt, 2*time.Second)

	bodies := rec.userTokenSnapshot()
	require.Len(t, bodies, 1)
	require.Equal(t, "42", bodies[0].UserID)
	require.Equal(t, 5, bodies[0].PlatformID)
}

func TestRegisterUserReturnsAPIError(t *testing.T) {
	rec := new(callRecord)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case apiPrefix + "/auth/get_admin_token":
			rec.recordAdmin()
			writeResponse(w, 0, "ok", map[string]any{"token": "admin-token", "expireTimeSeconds": 300})
		case apiPrefix + "/user/user_register":
			var body registerRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			rec.recordRegister(r, body)
			writeResponse(w, 1101, "用户不存在", nil)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, AdminUserID: "admin", AdminSecret: "secret"})
	err := c.RegisterUser(context.Background(), UserInfo{UserID: "missing"})

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, 1101, apiErr.Code)
	require.Equal(t, "用户不存在", apiErr.Message)
	require.Equal(t, http.StatusOK, apiErr.HTTPStatus)
}

func TestRegisterUserRetriesOnceAfterPermissionError(t *testing.T) {
	rec := new(callRecord)
	var lastToken atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case apiPrefix + "/auth/get_admin_token":
			rec.recordAdmin()
			adminCalls, _, _ := rec.counts()
			writeResponse(w, 0, "ok", map[string]any{
				"token":             fmt.Sprintf("admin-token-%d", adminCalls),
				"expireTimeSeconds": 300,
			})
		case apiPrefix + "/user/user_register":
			var body registerRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			rec.recordRegister(r, body)
			token := r.Header.Get(headerToken)
			lastToken.Store(token)
			if token == "admin-token-1" {
				writeResponse(w, 1002, "无权限", nil)
				return
			}
			writeResponse(w, 0, "ok", nil)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, AdminUserID: "admin", AdminSecret: "secret"})
	require.NoError(t, c.RegisterUser(context.Background(), UserInfo{UserID: "1"}))

	adminCalls, registerCalls, _ := rec.counts()
	require.Equal(t, 2, adminCalls)
	require.Equal(t, 2, registerCalls)
	require.Equal(t, "admin-token-2", lastToken.Load())
}

func TestRegisterUserHTTPError(t *testing.T) {
	rec := new(callRecord)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case apiPrefix + "/auth/get_admin_token":
			rec.recordAdmin()
			writeResponse(w, 0, "ok", map[string]any{"token": "admin-token", "expireTimeSeconds": 300})
		case apiPrefix + "/user/user_register":
			var body registerRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			rec.recordRegister(r, body)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal server error"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, AdminUserID: "admin", AdminSecret: "secret"})
	require.Error(t, c.RegisterUser(context.Background(), UserInfo{UserID: "1"}))
}

func TestRegisterUserTimeout(t *testing.T) {
	rec := new(callRecord)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case apiPrefix + "/auth/get_admin_token":
			rec.recordAdmin()
			writeResponse(w, 0, "ok", map[string]any{"token": "admin-token", "expireTimeSeconds": 300})
		case apiPrefix + "/user/user_register":
			var body registerRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			rec.recordRegister(r, body)
			time.Sleep(200 * time.Millisecond)
			writeResponse(w, 0, "ok", nil)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(Config{
		BaseURL:     srv.URL,
		AdminUserID: "admin",
		AdminSecret: "secret",
		Timeout:     30 * time.Millisecond,
	})
	require.Error(t, c.RegisterUser(context.Background(), UserInfo{UserID: "1"}))
}

func TestDisabledClient(t *testing.T) {
	c := New(Config{})
	require.False(t, c.Enabled())
	require.ErrorIs(t, c.RegisterUser(context.Background(), UserInfo{UserID: "1"}), ErrDisabled)

	_, err := c.UserToken(context.Background(), "1", 5)
	require.ErrorIs(t, err, ErrDisabled)
}
