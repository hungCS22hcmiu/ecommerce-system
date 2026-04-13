package middleware

import (
	"crypto/rsa"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	jwtpkg "github.com/hungCS22hcmiu/ecommrece-system/cart-service/pkg/jwt"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/pkg/response"
)

// Auth validates the Bearer JWT token using the given RSA public key.
// No blacklist check — cart-service doesn't require it.
// Sets "userID" (uuid.UUID) and "role" (string) in the Gin context.
func Auth(publicKey *rsa.PublicKey) gin.HandlerFunc {
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

		userID, err := uuid.Parse(claims.UserID)
		if err != nil {
			response.Unauthorized(c, "invalid user ID in token")
			c.Abort()
			return
		}

		c.Set("userID", userID)
		c.Set("role", claims.Role)
		c.Next()
	}
}
