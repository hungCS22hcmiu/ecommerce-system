package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger logs every request as a structured JSON line.
// It reads the correlationID set by the Correlation middleware and the
// userID set by the Auth middleware (when present).
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		correlationID, _ := c.Get("correlationID")
		userID, _ := c.Get("userID")

		slog.Info("request",
			"service", "payment-service",
			"correlationId", correlationID,
			"userId", userID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latencyMs", time.Since(start).Milliseconds(),
		)
	}
}
