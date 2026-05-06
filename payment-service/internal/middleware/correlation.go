package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type correlationKeyType struct{}

// CorrelationKey is the context key for the correlation ID.
// Using a private type avoids collisions with other packages.
var CorrelationKey = correlationKeyType{}

// Correlation reads X-Correlation-ID from the request header (set by Nginx/gateway),
// generates one if absent, and stores it in both the gin context and the request context.
// The request-context injection is needed by Kafka workers (Week 10) that operate outside gin.
func Correlation() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Correlation-ID")
		if id == "" {
			id = uuid.NewString()
		}
		c.Set("correlationID", id)
		c.Header("X-Correlation-ID", id)

		ctx := context.WithValue(c.Request.Context(), CorrelationKey, id)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}
