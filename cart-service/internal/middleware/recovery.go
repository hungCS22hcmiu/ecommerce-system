package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Recovery catches any panic in a handler, logs it, and returns 500.
// Without this, a single panic would crash the entire service.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				correlationID, _ := c.Get("correlationID")
				slog.Error("panic recovered",
					"error", err,
					"correlationId", correlationID,
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"error": gin.H{
						"code":      "INTERNAL_ERROR",
						"message":   "An unexpected error occurred",
						"timestamp": time.Now().UTC(),
						"path":      c.Request.URL.Path,
					},
				})
			}
		}()
		c.Next()
	}
}
