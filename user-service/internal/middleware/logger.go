package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Logger logs every request as structured JSON.
// It also injects a correlation ID into the context so downstream
// handlers and services can include it in their own log lines.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Reuse the correlation ID from Nginx if present, else generate one
		correlationID := c.GetHeader("X-Correlation-ID")
		if correlationID == "" {
			correlationID = uuid.NewString()
		}
		c.Set("correlationID", correlationID)
		c.Header("X-Correlation-ID", correlationID)

		c.Next() // Process request

		slog.Info("request",
			"correlationId", correlationID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latencyMs", time.Since(start).Milliseconds(),
			"clientIP", c.ClientIP(),
			"userAgent", c.Request.UserAgent(),
		)
	}
}
