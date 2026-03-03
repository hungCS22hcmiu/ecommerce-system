package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type HealthHandler struct {
	db    *gorm.DB
	redis *redis.Client
}

func NewHealthHandler(db *gorm.DB, redis *redis.Client) *HealthHandler {
	return &HealthHandler{db: db, redis: redis}
}

// Live handles GET /health/live
// Kubernetes liveness probe — is the process alive?
// Never checks external dependencies.
func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "UP",
		"timestamp": time.Now().UTC(),
	})
}

// Ready handles GET /health/ready
// Kubernetes readiness probe — can we serve traffic?
// Checks DB and Redis connectivity.
func (h *HealthHandler) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	status := gin.H{
		"status":    "UP",
		"timestamp": time.Now().UTC(),
		"checks":    gin.H{},
	}
	httpStatus := http.StatusOK

	// Check PostgreSQL
	sqlDB, err := h.db.DB()
	if err != nil || sqlDB.PingContext(ctx) != nil {
		status["status"] = "DOWN"
		status["checks"].(gin.H)["postgres"] = "DOWN"
		httpStatus = http.StatusServiceUnavailable
	} else {
		status["checks"].(gin.H)["postgres"] = "UP"
	}

	// Check Redis
	if err := h.redis.Ping(ctx).Err(); err != nil {
		status["status"] = "DOWN"
		status["checks"].(gin.H)["redis"] = "DOWN"
		httpStatus = http.StatusServiceUnavailable
	} else {
		status["checks"].(gin.H)["redis"] = "UP"
	}

	c.JSON(httpStatus, status)
}
