package middleware

import (
	"strings"

	"github.com/example/goapp/internal/shared/httpx"
	"github.com/gin-gonic/gin"
)

const (
	ctxUserIDKey = "user_id"
	authHeader   = "Authorization"
	bearerPrefix = "Bearer "
)

// TokenVerifier is implemented by the auth module. Keeping it as an interface
// here avoids a circular import between middleware and auth.
type TokenVerifier interface {
	VerifyAccessToken(token string) (userID string, err error)
}

// Auth requires a valid bearer token and injects the user id into the context.
func Auth(v TokenVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader(authHeader)
		if !strings.HasPrefix(raw, bearerPrefix) {
			httpx.Error(c, httpx.Unauthorized("auth.missing_token", "missing bearer token"))
			return
		}
		token := strings.TrimPrefix(raw, bearerPrefix)
		uid, err := v.VerifyAccessToken(token)
		if err != nil {
			httpx.Error(c, httpx.Unauthorized("auth.invalid_token", "invalid or expired token"))
			return
		}
		c.Set(ctxUserIDKey, uid)
		c.Next()
	}
}

// UserID extracts the authenticated user id, panicking if Auth was not used
// before the handler.
func UserID(c *gin.Context) string {
	v, ok := c.Get(ctxUserIDKey)
	if !ok {
		return ""
	}
	uid, _ := v.(string)
	return uid
}
