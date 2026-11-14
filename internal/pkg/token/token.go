// Package token 负责签发与校验 access JWT（HS256）。
package token

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims 是 access token 的业务载荷。
type Claims struct {
	SID        string   `json:"sid"`
	PlatformID int      `json:"pid"`
	DeviceID   string   `json:"did"`
	Roles      []string `json:"roles"`
	jwt.RegisteredClaims
}

// Issuer 签发 access token。
type Issuer struct {
	secret   []byte
	issuer   string
	audience string
	ttl      time.Duration
	now      func() time.Time
}

// NewIssuer 构造签发器。
func NewIssuer(secret, issuer, audience string, ttl time.Duration) *Issuer {
	return &Issuer{
		secret:   []byte(secret),
		issuer:   issuer,
		audience: audience,
		ttl:      ttl,
		now:      time.Now,
	}
}

// TTL 返回 access token 有效期。
func (i *Issuer) TTL() time.Duration { return i.ttl }

// Issue 签发 access token，返回令牌与过期时间。
func (i *Issuer) Issue(userID, sessionID, deviceID string, platformID int, roles []string) (string, time.Time, error) {
	now := i.now().UTC()
	exp := now.Add(i.ttl)
	claims := &Claims{
		SID:        sessionID,
		PlatformID: platformID,
		DeviceID:   deviceID,
		Roles:      roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.issuer,
			Audience:  jwt.ClaimStrings{i.audience},
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        sessionID + "-" + fmt.Sprintf("%d", now.UnixNano()),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("签发 access token 失败: %w", err)
	}
	return signed, exp, nil
}

// Parse 校验签名、issuer、audience 与过期时间。
func (i *Issuer) Parse(tokenString string) (*Claims, error) {
	claims := new(Claims)
	parsed, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		return i.secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(i.issuer),
		jwt.WithAudience(i.audience),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}
	if !parsed.Valid || claims.Subject == "" || claims.SID == "" {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}
