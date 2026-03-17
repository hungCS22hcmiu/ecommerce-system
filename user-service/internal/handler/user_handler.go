package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/middleware"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/service"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/response"
)

type UserHandler struct {
	userService service.UserService
}

func NewUserHandler(userService service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

// GetUser handles GET /api/v1/users/:id.
// Internal endpoint for service-to-service lookups — no JWT auth required.
// Security boundary: Docker internal network.
func (h *UserHandler) GetUser(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "INVALID_USER_ID", "user ID is not a valid UUID", nil)
		return
	}

	user, err := h.userService.GetUser(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			response.NotFound(c, "user")
			return
		}
		response.InternalError(c)
		return
	}

	response.Success(c, user)
}

// GetProfile handles GET /api/v1/users/profile
func (h *UserHandler) GetProfile(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	profile, err := h.userService.GetProfile(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			response.NotFound(c, "user")
			return
		}
		response.InternalError(c)
		return
	}

	response.Success(c, profile)
}

// UpdateProfile handles PUT /api/v1/users/profile
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	var req dto.UpdateProfileRequest
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

	profile, err := h.userService.UpdateProfile(c.Request.Context(), userID, req)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			response.NotFound(c, "user")
			return
		}
		response.InternalError(c)
		return
	}

	response.Success(c, profile)
}

// AddAddress handles POST /api/v1/users/addresses
func (h *UserHandler) AddAddress(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	var req dto.CreateAddressRequest
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

	addr, err := h.userService.AddAddress(c.Request.Context(), userID, req)
	if err != nil {
		response.InternalError(c)
		return
	}
	response.Created(c, addr, "")
}

// UpdateAddress handles PUT /api/v1/users/addresses/:id
func (h *UserHandler) UpdateAddress(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	addressID, ok := parseAddressID(c)
	if !ok {
		return
	}

	var req dto.UpdateAddressRequest
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

	addr, err := h.userService.UpdateAddress(c.Request.Context(), userID, addressID, req)
	if err != nil {
		handleAddressError(c, err)
		return
	}
	response.Success(c, addr)
}

// DeleteAddress handles DELETE /api/v1/users/addresses/:id
func (h *UserHandler) DeleteAddress(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	addressID, ok := parseAddressID(c)
	if !ok {
		return
	}

	if err := h.userService.DeleteAddress(c.Request.Context(), userID, addressID); err != nil {
		handleAddressError(c, err)
		return
	}
	response.NoContent(c)
}

// SetDefaultAddress handles PUT /api/v1/users/addresses/:id/default
func (h *UserHandler) SetDefaultAddress(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	addressID, ok := parseAddressID(c)
	if !ok {
		return
	}

	addr, err := h.userService.SetDefaultAddress(c.Request.Context(), userID, addressID)
	if err != nil {
		handleAddressError(c, err)
		return
	}
	response.Success(c, addr)
}

// parseUserID extracts and parses the userID UUID from the Gin context (set by Auth middleware).
// Returns false and writes an error response if the ID is missing or invalid.
func parseUserID(c *gin.Context) (uuid.UUID, bool) {
	userIDStr := c.GetString(middleware.CtxUserID)
	if userIDStr == "" {
		response.Unauthorized(c, "missing user identity")
		return uuid.Nil, false
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_USER_ID", "user ID is not a valid UUID", nil)
		return uuid.Nil, false
	}
	return userID, true
}

// parseAddressID extracts and parses the :id path parameter as a UUID.
func parseAddressID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "INVALID_ADDRESS_ID", "address ID is not a valid UUID", nil)
		return uuid.Nil, false
	}
	return id, true
}

// handleAddressError maps address service errors to HTTP responses.
func handleAddressError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrAddressNotFound):
		response.NotFound(c, "address")
	case errors.Is(err, service.ErrAddressForbidden):
		response.Forbidden(c, "address does not belong to this user")
	default:
		response.InternalError(c)
	}
}
