package account

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	domain "goim-business/internal/domain/account"
	imjob "goim-business/internal/job/im"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/pkg/token"
	"goim-business/internal/service/ports"
	"goim-business/internal/types"
)

const (
	testPassword    = "Passw0rd"
	testDevice      = "device-1"
	testPlatform    = int8(5)
	platformWindows = int8(3)
)

// 测试时钟：每次取值前进 1 秒，保证会话创建时间有序。
type testClock struct {
	mu   sync.Mutex
	base time.Time
	n    int
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	return c.base.Add(time.Duration(c.n) * time.Second)
}

type testEnv struct {
	svc         *Service
	users       *fakeUsers
	creds       *fakeCreds
	sessions    *fakeSessions
	cache       *fakeCache
	provisioner *fakeProvisioner
	imTokens    *fakeIMTokenCache
	outbox      *fakeOutbox
	imSync      *imjob.Worker
}

func newTestEnv(t *testing.T, mutate func(*Config)) *testEnv {
	t.Helper()

	users := newFakeUsers()
	creds := newFakeCreds()
	sessions := newFakeSessions()
	cache := newFakeCache()
	provisioner := newFakeProvisioner(false)
	imTokens := newFakeIMTokenCache()
	outbox := newFakeOutbox()

	cfg := Config{
		AccessTTL:              15 * time.Minute,
		RefreshTTL:             30 * 24 * time.Hour,
		MaxSessionsPerUser:     5,
		MaxSessionsPerPlatform: 3,
		LoginLockThreshold:     10,
		LoginLockWindow:        15 * time.Minute,
		IMOutboxBatchSize:      20,
	}
	if mutate != nil {
		mutate(&cfg)
	}

	issuer := token.NewIssuer("test-secret-0123456789", "goim-business", "goim-business-api", cfg.AccessTTL)
	clock := &testClock{base: time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)}
	imSync := imjob.NewWorker(users, provisioner, outbox, cfg.IMOutboxBatchSize, clock.Now, zap.NewNop().Sugar())
	svc := NewService(users, creds, sessions, fakeTx{}, cache, cache, provisioner, imTokens, outbox, imSync, issuer, cfg, zap.NewNop().Sugar())

	svc.now = clock.Now

	return &testEnv{
		svc:         svc,
		users:       users,
		creds:       creds,
		sessions:    sessions,
		cache:       cache,
		provisioner: provisioner,
		imTokens:    imTokens,
		outbox:      outbox,
		imSync:      imSync,
	}
}

func requireCode(t *testing.T, err error, code int) {
	t.Helper()
	require.Error(t, err)
	var bizErr *errcode.Error
	require.ErrorAs(t, err, &bizErr)
	require.Equal(t, code, bizErr.Code, "unexpected message: %s", bizErr.Message)
}

func idOf(t *testing.T, raw string) int64 {
	t.Helper()
	v, err := strconv.ParseInt(raw, 10, 64)
	require.NoError(t, err)
	return v
}

func TestRegisterAndLogin(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	first, err := env.svc.Register(ctx, types.RegisterRequest{
		LoginName: " Alice ",
		Password:  testPassword,
		DeviceID:  testDevice,
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	require.Equal(t, "alice", first.User.LoginName)
	require.Equal(t, "alice", first.User.Nickname)
	stored, err := env.users.GetByID(ctx, idOf(t, first.User.UserID))
	require.NoError(t, err)
	require.Equal(t, domain.StatusNormal, stored.Status)
	require.NotEmpty(t, first.Token.AccessToken)
	require.NotEmpty(t, first.RefreshToken)
	require.True(t, env.cache.isActive(first.SessionID))

	second, err := env.svc.Login(ctx, types.LoginRequest{
		LoginName: "alice",
		Password:  testPassword,
		DeviceID:  "device-2",
		Platform:  testPlatform,
	})
	require.NoError(t, err)
	require.Equal(t, first.User.UserID, second.User.UserID)
	require.NotEqual(t, first.SessionID, second.SessionID)
	require.Len(t, env.sessions.all(), 2)
}

func TestRegisterDuplicateLoginName(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()
	in := types.RegisterRequest{LoginName: "alice", Password: testPassword, DeviceID: testDevice, Platform: testPlatform}

	_, err := env.svc.Register(ctx, in)
	require.NoError(t, err)
	_, err = env.svc.Register(ctx, in)
	requireCode(t, err, errcode.CodeAccountExists)
}

func TestRegisterValidation(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	_, err := env.svc.Register(ctx, types.RegisterRequest{LoginName: "alice", Password: "short", DeviceID: testDevice, Platform: testPlatform})
	requireCode(t, err, errcode.CodeInvalidParam)

	_, err = env.svc.Register(ctx, types.RegisterRequest{LoginName: "A", Password: testPassword, DeviceID: testDevice, Platform: testPlatform})
	requireCode(t, err, errcode.CodeInvalidParam)
}

func TestLoginWrongPasswordAndLock(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	_, err := env.svc.Register(ctx, types.RegisterRequest{LoginName: "alice", Password: testPassword, DeviceID: testDevice, Platform: testPlatform})
	require.NoError(t, err)

	for i := 0; i < 10; i++ {
		_, err = env.svc.Login(ctx, types.LoginRequest{LoginName: "alice", Password: "WrongPass1", DeviceID: testDevice, Platform: testPlatform})
		requireCode(t, err, errcode.CodeInvalidCredential)
	}
	require.Equal(t, int64(10), env.cache.failureCount("alice"))

	// 达到阈值后，即使密码正确也会被锁定。
	_, err = env.svc.Login(ctx, types.LoginRequest{LoginName: "alice", Password: testPassword, DeviceID: testDevice, Platform: testPlatform})
	requireCode(t, err, errcode.CodeRateLimited)

	// 解锁后可以正常登录，且失败计数被清除。
	env.cache.clearFailures("alice")
	_, err = env.svc.Login(ctx, types.LoginRequest{LoginName: "alice", Password: testPassword, DeviceID: testDevice, Platform: testPlatform})
	require.NoError(t, err)
	require.Equal(t, int64(0), env.cache.failureCount("alice"))
}

func TestLoginUnknownAccount(t *testing.T) {
	env := newTestEnv(t, nil)
	_, err := env.svc.Login(context.Background(), types.LoginRequest{LoginName: "nobody", Password: testPassword, DeviceID: testDevice, Platform: testPlatform})
	requireCode(t, err, errcode.CodeInvalidCredential)
	require.Equal(t, int64(1), env.cache.failureCount("nobody"))
}

func TestRefreshRotationAndReplay(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	first, err := env.svc.Register(ctx, types.RegisterRequest{LoginName: "alice", Password: testPassword, DeviceID: testDevice, Platform: testPlatform})
	require.NoError(t, err)

	rotated, err := env.svc.Refresh(ctx, types.RefreshRequest{RefreshToken: first.RefreshToken})
	require.NoError(t, err)
	require.NotEqual(t, first.RefreshToken, rotated.RefreshToken)
	require.NotEqual(t, first.SessionID, rotated.SessionID)
	require.True(t, env.cache.isActive(rotated.SessionID))

	old, ok := env.sessions.get(idOf(t, first.SessionID))
	require.True(t, ok)
	require.NotNil(t, old.RotatedAt)

	// 重放已轮换的旧令牌：整族撤销并返回 2005。
	_, err = env.svc.Refresh(ctx, types.RefreshRequest{RefreshToken: first.RefreshToken})
	requireCode(t, err, errcode.CodeRefreshReplay)
	require.False(t, env.cache.isActive(rotated.SessionID))
	require.False(t, env.cache.isActive(first.SessionID))

	newSess, ok := env.sessions.get(idOf(t, rotated.SessionID))
	require.True(t, ok)
	require.NotNil(t, newSess.RevokedAt)

	// 家族被撤销后，新令牌也不可用。
	_, err = env.svc.Refresh(ctx, types.RefreshRequest{RefreshToken: rotated.RefreshToken})
	requireCode(t, err, errcode.CodeUnauthenticated)
}

func TestRefreshUnknownToken(t *testing.T) {
	env := newTestEnv(t, nil)
	_, err := env.svc.Refresh(context.Background(), types.RefreshRequest{RefreshToken: "unknown"})
	requireCode(t, err, errcode.CodeUnauthenticated)
}

func TestSessionLimitKicksOldest(t *testing.T) {
	env := newTestEnv(t, func(cfg *Config) {
		cfg.MaxSessionsPerUser = 5
		cfg.MaxSessionsPerPlatform = 3
	})
	ctx := context.Background()

	first, err := env.svc.Register(ctx, types.RegisterRequest{LoginName: "alice", Password: testPassword, DeviceID: "device-1", Platform: testPlatform})
	require.NoError(t, err)

	for i := 2; i <= 4; i++ {
		_, err = env.svc.Login(ctx, types.LoginRequest{
			LoginName: "alice",
			Password:  testPassword,
			DeviceID:  fmt.Sprintf("device-%d", i),
			Platform:  testPlatform,
		})
		require.NoError(t, err)
	}

	require.Equal(t, 3, env.sessions.activeCount())
	firstSess, ok := env.sessions.get(idOf(t, first.SessionID))
	require.True(t, ok)
	require.NotNil(t, firstSess.RevokedAt)
	require.False(t, env.cache.isActive(first.SessionID))
}

func TestMultiPlatformSessionsIndependent(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	web, err := env.svc.Register(ctx, types.RegisterRequest{LoginName: "alice", Password: testPassword, DeviceID: "web-1", Platform: testPlatform})
	require.NoError(t, err)

	for i := 1; i <= 3; i++ {
		_, err = env.svc.Login(ctx, types.LoginRequest{
			LoginName: "alice",
			Password:  testPassword,
			DeviceID:  fmt.Sprintf("win-%d", i),
			Platform:  platformWindows,
		})
		require.NoError(t, err)
	}

	// 每平台 3 个、每用户 5 个：Web 1 个 + Windows 3 个均未超限，互不影响。
	require.Equal(t, 4, env.sessions.activeCount())
	require.True(t, env.cache.isActive(web.SessionID))
}

func TestLogoutRevokesCurrentSession(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	first, err := env.svc.Register(ctx, types.RegisterRequest{LoginName: "alice", Password: testPassword, DeviceID: testDevice, Platform: testPlatform})
	require.NoError(t, err)

	require.NoError(t, env.svc.Logout(ctx, idOf(t, first.User.UserID), idOf(t, first.SessionID)))

	sess, ok := env.sessions.get(idOf(t, first.SessionID))
	require.True(t, ok)
	require.NotNil(t, sess.RevokedAt)
	require.False(t, env.cache.isActive(first.SessionID))

	views, err := env.svc.Sessions(ctx, idOf(t, first.User.UserID), idOf(t, first.SessionID))
	require.NoError(t, err)
	require.Empty(t, views)
}

func TestSessionsMarkCurrent(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	first, err := env.svc.Register(ctx, types.RegisterRequest{LoginName: "alice", Password: testPassword, DeviceID: "device-1", Platform: testPlatform})
	require.NoError(t, err)
	second, err := env.svc.Login(ctx, types.LoginRequest{LoginName: "alice", Password: testPassword, DeviceID: "device-2", Platform: testPlatform})
	require.NoError(t, err)

	views, err := env.svc.Sessions(ctx, idOf(t, first.User.UserID), idOf(t, second.SessionID))
	require.NoError(t, err)
	require.Len(t, views, 2)
	require.True(t, views[0].Current)
	require.False(t, views[1].Current)
}

func TestChangePasswordRevokesOtherSessions(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	first, err := env.svc.Register(ctx, types.RegisterRequest{LoginName: "alice", Password: testPassword, DeviceID: "device-1", Platform: testPlatform})
	require.NoError(t, err)
	second, err := env.svc.Login(ctx, types.LoginRequest{LoginName: "alice", Password: testPassword, DeviceID: "device-2", Platform: testPlatform})
	require.NoError(t, err)

	require.NoError(t, env.svc.ChangePassword(ctx, idOf(t, first.User.UserID), idOf(t, second.SessionID), types.ChangePasswordRequest{OldPassword: testPassword, NewPassword: "NewPassw0rd"}))

	firstSess, ok := env.sessions.get(idOf(t, first.SessionID))
	require.True(t, ok)
	require.NotNil(t, firstSess.RevokedAt)
	require.False(t, env.cache.isActive(first.SessionID))
	require.True(t, env.cache.isActive(second.SessionID))

	_, err = env.svc.Login(ctx, types.LoginRequest{LoginName: "alice", Password: testPassword, DeviceID: "device-3", Platform: testPlatform})
	requireCode(t, err, errcode.CodeInvalidCredential)

	_, err = env.svc.Login(ctx, types.LoginRequest{LoginName: "alice", Password: "NewPassw0rd", DeviceID: "device-3", Platform: testPlatform})
	require.NoError(t, err)
}

func TestRevokeSessionRejectsForeignSession(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	first, err := env.svc.Register(ctx, types.RegisterRequest{LoginName: "alice", Password: testPassword, DeviceID: "device-1", Platform: testPlatform})
	require.NoError(t, err)
	second, err := env.svc.Register(ctx, types.RegisterRequest{LoginName: "bob", Password: testPassword, DeviceID: "device-2", Platform: testPlatform})
	require.NoError(t, err)

	err = env.svc.RevokeSession(ctx, idOf(t, first.User.UserID), idOf(t, second.SessionID))
	requireCode(t, err, errcode.CodeNotFound)
}

// ---- fakes ----

type fakeTx struct{}

func (fakeTx) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type fakeUsers struct {
	mu   sync.Mutex
	seq  int64
	byID map[int64]*domain.User
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{byID: make(map[int64]*domain.User)}
}

func (f *fakeUsers) Create(_ context.Context, user *domain.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.byID {
		if existing.LoginName == user.LoginName {
			return domain.ErrDuplicate
		}
	}
	f.seq++
	user.ID = f.seq
	clone := *user
	f.byID[user.ID] = &clone
	return nil
}

func (f *fakeUsers) GetByLoginName(_ context.Context, loginName string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, user := range f.byID {
		if user.LoginName == loginName {
			clone := *user
			return &clone, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeUsers) GetByID(_ context.Context, id int64) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	user, ok := f.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	clone := *user
	return &clone, nil
}

func (f *fakeUsers) TouchLastLogin(_ context.Context, id int64, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	user, ok := f.byID[id]
	if !ok {
		return domain.ErrNotFound
	}
	user.LastLoginAt = &at
	return nil
}

func (f *fakeUsers) UpdateIMStatus(_ context.Context, id int64, status int8, syncedAt *time.Time, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	user, ok := f.byID[id]
	if !ok {
		return domain.ErrNotFound
	}
	user.IMStatus = status
	if syncedAt == nil {
		user.IMSyncedAt = nil
	} else {
		value := *syncedAt
		user.IMSyncedAt = &value
	}
	user.UpdatedAt = at
	return nil
}

type fakeProvisioner struct {
	mu           sync.Mutex
	enabled      bool
	calls        int
	lastUserID   string
	lastNickname string
	err          error

	token          *ports.IMToken
	tokenErr       error
	tokenCalls     int
	lastTokenUser  string
	lastTokenPlatf int
}

func newFakeProvisioner(enabled bool) *fakeProvisioner {
	return &fakeProvisioner{enabled: enabled}
}

func (f *fakeProvisioner) Enabled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.enabled
}

func (f *fakeProvisioner) RegisterUser(_ context.Context, userID, nickname string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastUserID = userID
	f.lastNickname = nickname
	return f.err
}

func (f *fakeProvisioner) UserToken(_ context.Context, userID string, platformID int) (*ports.IMToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokenCalls++
	f.lastTokenUser = userID
	f.lastTokenPlatf = platformID
	return f.token, f.tokenErr
}

func (f *fakeProvisioner) setToken(token *ports.IMToken, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.token = token
	f.tokenErr = err
}

func (f *fakeProvisioner) tokenCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tokenCalls
}

func (f *fakeProvisioner) setEnabled(enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enabled = enabled
}

func (f *fakeProvisioner) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeProvisioner) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeProvisioner) last() (calls int, userID, nickname string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, f.lastUserID, f.lastNickname
}

type fakeIMTokenEntry struct {
	token    string
	expireAt time.Time
	ttl      time.Duration
}

type fakeIMTokenCache struct {
	mu      sync.Mutex
	entries map[string]fakeIMTokenEntry
}

func newFakeIMTokenCache() *fakeIMTokenCache {
	return &fakeIMTokenCache{entries: make(map[string]fakeIMTokenEntry)}
}

func imTokenKey(userID string, platform int8) string {
	return fmt.Sprintf("%s:%d", userID, platform)
}

func (f *fakeIMTokenCache) Get(_ context.Context, userID string, platform int8) (string, time.Time, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.entries[imTokenKey(userID, platform)]
	if !ok {
		return "", time.Time{}, false, nil
	}
	return entry.token, entry.expireAt, true, nil
}

func (f *fakeIMTokenCache) Set(_ context.Context, userID string, platform int8, token string, expireAt time.Time, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries[imTokenKey(userID, platform)] = fakeIMTokenEntry{token: token, expireAt: expireAt, ttl: ttl}
	return nil
}

func (f *fakeIMTokenCache) seed(userID string, platform int8, token string, expireAt time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries[imTokenKey(userID, platform)] = fakeIMTokenEntry{token: token, expireAt: expireAt}
}

func (f *fakeIMTokenCache) entry(userID string, platform int8) (fakeIMTokenEntry, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.entries[imTokenKey(userID, platform)]
	return entry, ok
}

type fakeCreds struct {
	mu   sync.Mutex
	byID map[int64]*domain.Credential
}

func newFakeCreds() *fakeCreds {
	return &fakeCreds{byID: make(map[int64]*domain.Credential)}
}

func (f *fakeCreds) Create(_ context.Context, cred *domain.Credential) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byID[cred.UserID]; ok {
		return domain.ErrDuplicate
	}
	clone := *cred
	f.byID[cred.UserID] = &clone
	return nil
}

func (f *fakeCreds) GetByUserID(_ context.Context, userID int64) (*domain.Credential, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cred, ok := f.byID[userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	clone := *cred
	return &clone, nil
}

func (f *fakeCreds) UpdatePasswordHash(_ context.Context, userID int64, hash string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cred, ok := f.byID[userID]
	if !ok {
		return domain.ErrNotFound
	}
	cred.PasswordHash = hash
	cred.UpdatedAt = at
	return nil
}

type fakeSessions struct {
	mu   sync.Mutex
	seq  int64
	list []*domain.Session
}

func newFakeSessions() *fakeSessions {
	return &fakeSessions{}
}

func (f *fakeSessions) Create(_ context.Context, session *domain.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.list {
		if existing.RefreshHash == session.RefreshHash {
			return domain.ErrDuplicate
		}
	}
	f.seq++
	session.ID = f.seq
	clone := *session
	f.list = append(f.list, &clone)
	return nil
}

func (f *fakeSessions) GetByRefreshHash(_ context.Context, hash string) (*domain.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, session := range f.list {
		if session.RefreshHash == hash {
			clone := *session
			return &clone, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeSessions) GetByID(_ context.Context, id int64) (*domain.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, session := range f.list {
		if session.ID == id {
			clone := *session
			return &clone, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeSessions) MarkRotated(_ context.Context, id int64, at time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, session := range f.list {
		if session.ID != id {
			continue
		}
		if session.RotatedAt != nil || session.RevokedAt != nil {
			return false, nil
		}
		value := at
		session.RotatedAt = &value
		return true, nil
	}
	return false, domain.ErrNotFound
}

func (f *fakeSessions) Revoke(_ context.Context, id int64, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, session := range f.list {
		if session.ID == id && session.RevokedAt == nil {
			value := at
			session.RevokedAt = &value
		}
	}
	return nil
}

func (f *fakeSessions) RevokeAll(_ context.Context, userID int64, at time.Time, exceptSID int64) ([]int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var sids []int64
	for _, session := range f.list {
		if session.UserID != userID || session.RevokedAt != nil || session.RotatedAt != nil || session.ID == exceptSID {
			continue
		}
		value := at
		session.RevokedAt = &value
		sids = append(sids, session.ID)
	}
	return sids, nil
}

func (f *fakeSessions) RevokeFamily(_ context.Context, familyID string, at time.Time) ([]int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var sids []int64
	for _, session := range f.list {
		if session.FamilyID != familyID || session.RevokedAt != nil {
			continue
		}
		value := at
		session.RevokedAt = &value
		sids = append(sids, session.ID)
	}
	return sids, nil
}

func (f *fakeSessions) ListActive(_ context.Context, userID int64, at time.Time) ([]*domain.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var active []*domain.Session
	for _, session := range f.list {
		if session.UserID != userID || session.RevokedAt != nil || session.RotatedAt != nil || !session.ExpiresAt.After(at) {
			continue
		}
		clone := *session
		active = append(active, &clone)
	}
	sort.Slice(active, func(i, j int) bool {
		return active[i].CreatedAt.Before(active[j].CreatedAt)
	})
	return active, nil
}

func (f *fakeSessions) all() []*domain.Session {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*domain.Session, 0, len(f.list))
	for _, session := range f.list {
		clone := *session
		out = append(out, &clone)
	}
	return out
}

func (f *fakeSessions) get(id int64) (*domain.Session, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, session := range f.list {
		if session.ID == id {
			clone := *session
			return &clone, true
		}
	}
	return nil, false
}

func (f *fakeSessions) activeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, session := range f.list {
		if session.RevokedAt == nil && session.RotatedAt == nil {
			count++
		}
	}
	return count
}

type fakeCache struct {
	mu       sync.Mutex
	active   map[string]bool
	failures map[string]int64
}

func newFakeCache() *fakeCache {
	return &fakeCache{active: make(map[string]bool), failures: make(map[string]int64)}
}

func (f *fakeCache) Activate(_ context.Context, sid, _ string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active[sid] = true
	return nil
}

func (f *fakeCache) Deactivate(_ context.Context, sid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.active, sid)
	return nil
}

func (f *fakeCache) LoginFailureCount(_ context.Context, loginName string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.failures[loginName], nil
}

func (f *fakeCache) IncrLoginFailure(_ context.Context, loginName string, _ time.Duration) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures[loginName]++
	return f.failures[loginName], nil
}

func (f *fakeCache) ClearLoginFailures(_ context.Context, loginName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.failures, loginName)
	return nil
}

func (f *fakeCache) isActive(sid string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active[sid]
}

func (f *fakeCache) failureCount(loginName string) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.failures[loginName]
}

func (f *fakeCache) clearFailures(loginName string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.failures, loginName)
}
