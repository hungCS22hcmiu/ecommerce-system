package response

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ─── Envelope types ──────────────────────────────────────────────────────────

type successEnvelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
}

type paginatedEnvelope struct {
	Success bool           `json:"success"`
	Data    interface{}    `json:"data"`
	Meta    PaginationMeta `json:"meta"`
}

type errorEnvelope struct {
	Success bool        `json:"success"`
	Error   ErrorDetail `json:"error"`
}

type PaginationMeta struct {
	Page          int   `json:"page"`
	Size          int   `json:"size"`
	TotalElements int64 `json:"totalElements"`
	TotalPages    int   `json:"totalPages"`
}

type ErrorDetail struct {
	Code      string      `json:"code"`
	Message   string      `json:"message"`
	Timestamp time.Time   `json:"timestamp"`
	Path      string      `json:"path"`
	Details   interface{} `json:"details,omitempty"`
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, successEnvelope{Success: true, Data: data})
}

func Created(c *gin.Context, data interface{}, location string) {
	if location != "" {
		c.Header("Location", location)
	}
	c.JSON(http.StatusCreated, successEnvelope{Success: true, Data: data})
}

func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

func Paginated(c *gin.Context, data interface{}, meta PaginationMeta) {
	c.JSON(http.StatusOK, paginatedEnvelope{Success: true, Data: data, Meta: meta})
}

func Error(c *gin.Context, statusCode int, code, message string, details interface{}) {
	c.JSON(statusCode, errorEnvelope{
		Success: false,
		Error: ErrorDetail{
			Code:      code,
			Message:   message,
			Timestamp: time.Now().UTC(),
			Path:      c.Request.URL.Path,
			Details:   details,
		},
	})
}

func BadRequest(c *gin.Context, code, message string, details interface{}) {
	Error(c, http.StatusBadRequest, code, message, details)
}

func Unauthorized(c *gin.Context, message string) {
	Error(c, http.StatusUnauthorized, "UNAUTHORIZED", message, nil)
}

func Forbidden(c *gin.Context, message string) {
	Error(c, http.StatusForbidden, "FORBIDDEN", message, nil)
}

func NotFound(c *gin.Context, resource string) {
	Error(c, http.StatusNotFound, "NOT_FOUND", resource+" not found", nil)
}

func Conflict(c *gin.Context, code, message string) {
	Error(c, http.StatusConflict, code, message, nil)
}

func InternalError(c *gin.Context) {
	Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
}

func TooManyRequests(c *gin.Context) {
	Error(c, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "Too many requests. Please try again later.", nil)
}
