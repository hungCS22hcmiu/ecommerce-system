package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/repository"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/service"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/pkg/response"
)

var validate = validator.New()

type PaymentHandler struct {
	svc service.PaymentService
}

func NewPaymentHandler(svc service.PaymentService) *PaymentHandler {
	return &PaymentHandler{svc: svc}
}

func (h *PaymentHandler) getUserID(c *gin.Context) (uuid.UUID, bool) {
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

func (h *PaymentHandler) isAdmin(c *gin.Context) bool {
	role, _ := c.Get("role")
	return role == "admin"
}

func (h *PaymentHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound), errors.Is(err, repository.ErrNotFound):
		response.NotFound(c, "payment")
	case errors.Is(err, service.ErrForbidden):
		response.Forbidden(c, "access denied")
	case errors.Is(err, repository.ErrDuplicateIdempotencyKey):
		response.Conflict(c, "DUPLICATE_PAYMENT", "a payment for this order already exists")
	case errors.Is(err, context.DeadlineExceeded):
		response.Error(c, http.StatusGatewayTimeout, "GATEWAY_TIMEOUT", "payment gateway timed out", nil)
	default:
		response.InternalError(c)
	}
}

// CreatePayment handles POST /api/v1/payments (internal, no auth required).
func (h *PaymentHandler) CreatePayment(c *gin.Context) {
	var req dto.ProcessPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "INVALID_BODY", "invalid request body", nil)
		return
	}
	if err := validate.Struct(req); err != nil {
		response.BadRequest(c, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	in := service.ProcessPaymentInput{
		OrderID:        req.OrderID,
		UserID:         req.UserID,
		Amount:         req.Amount,
		Currency:       req.Currency,
		IdempotencyKey: req.IdempotencyKey,
	}
	payment, err := h.svc.ProcessPayment(c.Request.Context(), in)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Created(c, dto.ToPaymentResponse(payment), "")
}

// GetByID handles GET /api/v1/payments/:id (auth required).
func (h *PaymentHandler) GetByID(c *gin.Context) {
	userID, ok := h.getUserID(c)
	if !ok {
		return
	}
	paymentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "INVALID_PARAM", "id must be a valid UUID", nil)
		return
	}
	resp, err := h.svc.GetByID(c.Request.Context(), paymentID, userID, h.isAdmin(c))
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, resp)
}

// GetByOrderID handles GET /api/v1/payments/order/:orderId (auth required).
func (h *PaymentHandler) GetByOrderID(c *gin.Context) {
	userID, ok := h.getUserID(c)
	if !ok {
		return
	}
	orderID, err := uuid.Parse(c.Param("orderId"))
	if err != nil {
		response.BadRequest(c, "INVALID_PARAM", "orderId must be a valid UUID", nil)
		return
	}
	resp, err := h.svc.GetByOrderID(c.Request.Context(), orderID, userID, h.isAdmin(c))
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, resp)
}

// ListByUser handles GET /api/v1/payments (auth required, paginated).
func (h *PaymentHandler) ListByUser(c *gin.Context) {
	userID, ok := h.getUserID(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	payments, total, err := h.svc.ListByUser(c.Request.Context(), userID, page, size)
	if err != nil {
		h.handleError(c, err)
		return
	}

	totalPages := int(total) / size
	if int(total)%size != 0 {
		totalPages++
	}
	response.Paginated(c, payments, response.PaginationMeta{
		Page:          page,
		Size:          size,
		TotalElements: total,
		TotalPages:    totalPages,
	})
}
