package middlewares

import (
	"context"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"goim-business/internal/pkg/convert"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/pkg/response"
	"goim-business/internal/pkg/token"
)

// SessionChecker 判断会话是否仍然活跃（由 Redis 实现）。
type SessionChecker interface {
	IsActive(ctx context.Context, sid string) (bool, error)
}

// Authenticator 校验 access JWT 与 Redis 活跃标记。
type Authenticator struct {
	tokens   *token.Issuer
	sessions SessionChecker
	log      *zap.Logger
}

// NewAuthenticator 构造鉴权中间件。
func NewAuthenticator(tokens *token.Issuer, sessions SessionChecker, log *zap.Logger) *Authenticator {
	return &Authenticator{tokens: tokens, sessions: sessions, log: log}
}

// Middleware 校验 Authorization: Bearer <access token>。
func (a *Authenticator) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(header, "Bearer ") {
			response.Abort(c, errcode.ErrUnauthenticated)
			return
		}
		raw := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if raw == "" {
			response.Abort(c, errcode.ErrUnauthenticated)
			return
		}

		claims, err := a.tokens.Parse(raw)
		if err != nil {
			if errors.Is(err, jwt.ErrTokenExpired) {
				response.Abort(c, errcode.ErrAccessExpired)
				return
			}
			response.Abort(c, errcode.ErrUnauthenticated)
			return
		}

		active, err := a.sessions.IsActive(c.Request.Context(), claims.SID)
		if err != nil {
			a.log.Error("校验会话活跃状态失败", zap.String("sid", claims.SID), zap.Error(err))
			response.Abort(c, errcode.ErrDependency)
			return
		}
		if !active {
			response.Abort(c, errcode.ErrUnauthenticated.WithMessage("登录态已失效，请重新登录"))
			return
		}

		userID := convert.ToInt64(claims.Subject)
		sessionID := convert.ToInt64(claims.SID)
		if userID == 0 || sessionID == 0 {
			response.Abort(c, errcode.ErrInvalidParam)
			return
		}

		c.Set(CtxUserID, userID)
		c.Set(CtxSessionID, sessionID)
		c.Set(CtxPlatform, claims.PlatformID)
		c.Set(CtxDeviceID, claims.DeviceID)
		c.Next()
	}
}
