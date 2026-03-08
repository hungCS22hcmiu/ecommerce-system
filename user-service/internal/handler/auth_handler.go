package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/service"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/response"
)

var validate = validator.New()

type AuthHandler struct {
	authService service.AuthService
}

func NewAuthHandler(authService service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// Register handles POST /api/v1/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "INVALID_BODY", "request body is not valid JSON", nil)
		return
	}

	if err := validate.Struct(req); err != nil {
		var ve validator.ValidationErrors
		errors.As(err, &ve)
		fields := make(map[string]string, len(ve))
		for _, fe := range ve {
			fields[fe.Field()] = fe.Tag()
		}
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "validation failed", fields)
		return
	}

	user, err := h.authService.Register(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, service.ErrDuplicateEmail) {
			response.Conflict(c, "DUPLICATE_EMAIL", "email already registered")
			return
		}
		response.InternalError(c)
		return
	}

	resp := dto.UserResponse{
		ID:        user.ID.String(),
		Email:     user.Email,
		Role:      user.Role,
		FirstName: user.Profile.FirstName,
		LastName:  user.Profile.LastName,
	}
	response.Created(c, resp, "")
}

// Login handles POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "INVALID_BODY", "request body is not valid JSON", nil)
		return
	}

	if err := validate.Struct(req); err != nil {
		var ve validator.ValidationErrors
		errors.As(err, &ve)
		fields := make(map[string]string, len(ve))
		for _, fe := range ve {
			fields[fe.Field()] = fe.Tag()
		}
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "validation failed", fields)
		return
	}

	loginResp, err := h.authService.Login(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCredentials):
			response.Error(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password", nil)
		case errors.Is(err, service.ErrAccountLocked):
			response.Error(c, http.StatusForbidden, "ACCOUNT_LOCKED", "account is locked due to too many failed login attempts", nil)
		default:
			response.InternalError(c)
		}
		return
	}

	response.Success(c, loginResp)
}

// Refresh handles POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req dto.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "INVALID_BODY", "request body is not valid JSON", nil)
		return
	}

	if err := validate.Struct(req); err != nil {
		var ve validator.ValidationErrors
		errors.As(err, &ve)
		fields := make(map[string]string, len(ve))
		for _, fe := range ve {
			fields[fe.Field()] = fe.Tag()
		}
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "validation failed", fields)
		return
	}

	refreshResp, err := h.authService.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, service.ErrInvalidToken) {
			response.Error(c, http.StatusUnauthorized, "INVALID_TOKEN", "refresh token is invalid or expired", nil)
			return
		}
		response.InternalError(c)
		return
	}

	response.Success(c, refreshResp)
}
