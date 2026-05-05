package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/service"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/pkg/response"
)

var validate = validator.New()

type CartHandler struct {
	cartSvc service.CartService
}

func NewCartHandler(cartSvc service.CartService) *CartHandler {
	return &CartHandler{cartSvc: cartSvc}
}

// getUserID extracts the uuid.UUID stored by the auth middleware.
func (h *CartHandler) getUserID(c *gin.Context) (uuid.UUID, bool) {
	val, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "missing user context")
		return uuid.Nil, false
	}
	userID, ok := val.(uuid.UUID)
	if !ok {
		response.InternalError(c)
		return uuid.Nil, false
	}
	return userID, true
}

// parseProductID parses the :productId path param as int64.
func parseProductID(c *gin.Context) (int64, bool) {
	raw := c.Param("productId")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		response.BadRequest(c, "INVALID_PARAM", "productId must be an integer", nil)
		return 0, false
	}
	return id, true
}

// handleCartError maps service sentinel errors to HTTP responses.
func handleCartError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrProductNotFound):
		response.NotFound(c, "product")
	case errors.Is(err, service.ErrProductServiceUnavailable):
		response.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "product service unavailable", nil)
	case errors.Is(err, service.ErrItemNotInCart):
		response.NotFound(c, "cart item")
	case errors.Is(err, service.ErrConcurrentUpdate):
		response.Error(c, http.StatusConflict, "CONCURRENT_UPDATE", "concurrent update detected, please retry", nil)
	default:
		response.InternalError(c)
	}
}

// GetCart handles GET /api/v1/cart
func (h *CartHandler) GetCart(c *gin.Context) {
	userID, ok := h.getUserID(c)
	if !ok {
		return
	}
	cart, err := h.cartSvc.GetCart(c.Request.Context(), userID)
	if err != nil {
		handleCartError(c, err)
		return
	}
	response.Success(c, cart)
}

// AddItem handles POST /api/v1/cart/items
func (h *CartHandler) AddItem(c *gin.Context) {
	userID, ok := h.getUserID(c)
	if !ok {
		return
	}
	var req dto.AddItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "INVALID_BODY", "invalid request body", nil)
		return
	}
	if err := validate.Struct(req); err != nil {
		response.BadRequest(c, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	cart, err := h.cartSvc.AddItem(c.Request.Context(), userID, req)
	if err != nil {
		handleCartError(c, err)
		return
	}
	response.Created(c, cart, "")
}

// UpdateItem handles PUT /api/v1/cart/items/:productId
func (h *CartHandler) UpdateItem(c *gin.Context) {
	userID, ok := h.getUserID(c)
	if !ok {
		return
	}
	productID, ok := parseProductID(c)
	if !ok {
		return
	}
	var req dto.UpdateItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "INVALID_BODY", "invalid request body", nil)
		return
	}
	if err := validate.Struct(req); err != nil {
		response.BadRequest(c, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	cart, err := h.cartSvc.UpdateItem(c.Request.Context(), userID, productID, req)
	if err != nil {
		handleCartError(c, err)
		return
	}
	response.Success(c, cart)
}

// RemoveItem handles DELETE /api/v1/cart/items/:productId
func (h *CartHandler) RemoveItem(c *gin.Context) {
	userID, ok := h.getUserID(c)
	if !ok {
		return
	}
	productID, ok := parseProductID(c)
	if !ok {
		return
	}
	if err := h.cartSvc.RemoveItem(c.Request.Context(), userID, productID); err != nil {
		handleCartError(c, err)
		return
	}
	response.NoContent(c)
}

// ClearCart handles DELETE /api/v1/cart
func (h *CartHandler) ClearCart(c *gin.Context) {
	userID, ok := h.getUserID(c)
	if !ok {
		return
	}
	if err := h.cartSvc.ClearCart(c.Request.Context(), userID); err != nil {
		handleCartError(c, err)
		return
	}
	response.NoContent(c)
}
