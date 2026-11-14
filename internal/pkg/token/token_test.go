package token

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret-0123456789-abcdef"

func TestIssueAndParse(t *testing.T) {
	issuer := NewIssuer(testSecret, "goim-business", "goim-business-api", 15*time.Minute)
	raw, exp, err := issuer.Issue("user-1", "session-1", "device-1", 5, []string{"user"})
	require.NoError(t, err)
	require.True(t, exp.After(time.Now()))

	claims, err := issuer.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, "user-1", claims.Subject)
	require.Equal(t, "session-1", claims.SID)
	require.Equal(t, 5, claims.PlatformID)
	require.Equal(t, "device-1", claims.DeviceID)
	require.Equal(t, []string{"user"}, claims.Roles)
}

func TestParseExpired(t *testing.T) {
	issuer := NewIssuer(testSecret, "goim-business", "goim-business-api", time.Minute)
	issuer.now = func() time.Time { return time.Now().Add(-10 * time.Minute) }
	raw, _, err := issuer.Issue("user-1", "session-1", "device-1", 5, nil)
	require.NoError(t, err)
	issuer.now = time.Now

	_, err = issuer.Parse(raw)
	require.Error(t, err)
	require.True(t, errors.Is(err, jwt.ErrTokenExpired))
}

func TestParseRejectsWrongAudience(t *testing.T) {
	issuer := NewIssuer(testSecret, "goim-business", "goim-business-api", 15*time.Minute)
	other := NewIssuer(testSecret, "goim-business", "other-api", 15*time.Minute)

	raw, _, err := issuer.Issue("user-1", "session-1", "device-1", 5, nil)
	require.NoError(t, err)
	_, err = other.Parse(raw)
	require.Error(t, err)
}

func TestParseRejectsTamperedToken(t *testing.T) {
	issuer := NewIssuer(testSecret, "goim-business", "goim-business-api", 15*time.Minute)
	raw, _, err := issuer.Issue("user-1", "session-1", "device-1", 5, nil)
	require.NoError(t, err)
	_, err = issuer.Parse(raw + "x")
	require.Error(t, err)
}
