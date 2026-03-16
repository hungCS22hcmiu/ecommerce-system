package middleware

import (
	"crypto/rsa"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/blacklist"
	jwtpkg "github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/jwt"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/response"
)

// Context keys injected by the Auth middleware.
const (
	CtxUserID      = "userID"
	CtxRole        = "role"
	CtxJTI         = "jti"
	CtxTokenExpiry = "tokenExpiry"
)

// Auth validates the Bearer JWT and checks the Redis blacklist.
// On success it injects userID, role, jti, and tokenExpiry into the Gin context.
func Auth(publicKey *rsa.PublicKey, bl blacklist.Blacklist) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			response.Unauthorized(c, "missing or malformed authorization header")
			c.Abort()
			return
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")
		claims, err := jwtpkg.ValidateToken(tokenStr, publicKey)
		if err != nil {
			response.Unauthorized(c, "invalid or expired token")
			c.Abort()
			return
		}

		blacklisted, err := bl.Contains(c.Request.Context(), claims.ID)
		if err != nil {
			response.InternalError(c)
			c.Abort()
			return
		}
		if blacklisted {
			response.Unauthorized(c, "token has been revoked")
			c.Abort()
			return
		}

		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxRole, claims.Role)
		c.Set(CtxJTI, claims.ID)
		c.Set(CtxTokenExpiry, claims.ExpiresAt.Time)
		c.Next()
	}
}
