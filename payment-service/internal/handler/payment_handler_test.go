package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/handler"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/model"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/repository"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/service"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ─── Mock ─────────────────────────────────────────────────────────────────────

type mockPaymentService struct{ mock.Mock }

func (m *mockPaymentService) ProcessPayment(ctx context.Context, in service.ProcessPaymentInput) (*model.Payment, error) {
	args := m.Called(ctx, in)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Payment), args.Error(1)
}

func (m *mockPaymentService) GetByID(ctx context.Context, paymentID, userID uuid.UUID, isAdmin bool) (*dto.PaymentResponse, error) {
	args := m.Called(ctx, paymentID, userID, isAdmin)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.PaymentResponse), args.Error(1)
}

func (m *mockPaymentService) GetByOrderID(ctx context.Context, orderID, userID uuid.UUID, isAdmin bool) (*dto.PaymentResponse, error) {
	args := m.Called(ctx, orderID, userID, isAdmin)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.PaymentResponse), args.Error(1)
}

func (m *mockPaymentService) ListByUser(ctx context.Context, userID uuid.UUID, page, size int) ([]dto.PaymentResponse, int64, error) {
	args := m.Called(ctx, userID, page, size)
	return args.Get(0).([]dto.PaymentResponse), args.Get(1).(int64), args.Error(2)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// newPaymentRouterNoAuth registers only CreatePayment (no auth required in production).
func newPaymentRouterNoAuth(svc service.PaymentService) *gin.Engine {
	r := gin.New()
	h := handler.NewPaymentHandler(svc)
	r.POST("/api/v1/payments", h.CreatePayment)
	return r
}

// newPaymentRouter registers all endpoints with a fake auth middleware injecting userID and role.
func newPaymentRouter(svc service.PaymentService, userID uuid.UUID, role string) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userID", userID)
		c.Set("role", role)
		c.Next()
	})
	h := handler.NewPaymentHandler(svc)
	r.POST("/api/v1/payments", h.CreatePayment)
	authed := r.Group("/api/v1/payments")
	authed.GET("", h.ListByUser)
	authed.GET("/order/:orderId", h.GetByOrderID)
	authed.GET("/:id", h.GetByID)
	return r
}

// newPaymentRouterUnauthenticated registers GET endpoints without injecting a userID.
func newPaymentRouterUnauthenticated(svc service.PaymentService) *gin.Engine {
	r := gin.New()
	h := handler.NewPaymentHandler(svc)
	authed := r.Group("/api/v1/payments")
	authed.GET("", h.ListByUser)
	authed.GET("/order/:orderId", h.GetByOrderID)
	authed.GET("/:id", h.GetByID)
	return r
}

func doRequest(router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	var buf *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewBuffer(b)
	} else {
		buf = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func parseBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var result map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	return result
}

func samplePayment(userID uuid.UUID) *model.Payment {
	return &model.Payment{
		ID:               uuid.New(),
		OrderID:          uuid.New(),
		UserID:           userID,
		Amount:           decimal.NewFromFloat(99.99),
		Currency:         "USD",
		Status:           model.PaymentStatusCompleted,
		Method:           model.PaymentMethodMockCard,
		IdempotencyKey:   uuid.NewString(),
		GatewayReference: "txn-abc123",
		CreatedAt:        time.Now(),
	}
}

func samplePaymentResponse(userID uuid.UUID) *dto.PaymentResponse {
	p := samplePayment(userID)
	r := dto.ToPaymentResponse(p)
	return &r
}

func validCreateRequest(orderID, userID uuid.UUID) map[string]any {
	return map[string]any{
		"orderId":        orderID.String(),
		"userId":         userID.String(),
		"amount":         "99.99",
		"currency":       "USD",
		"idempotencyKey": uuid.NewString(),
	}
}

// ─── CreatePayment ────────────────────────────────────────────────────────────

func TestCreatePayment_Success_Returns201(t *testing.T) {
	userID := uuid.New()
	orderID := uuid.New()
	payment := samplePayment(userID)
	svc := &mockPaymentService{}
	svc.On("ProcessPayment", mock.Anything, mock.Anything).Return(payment, nil)

	w := doRequest(newPaymentRouterNoAuth(svc), http.MethodPost, "/api/v1/payments",
		validCreateRequest(orderID, userID))

	assert.Equal(t, http.StatusCreated, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, true, body["success"])
	data := body["data"].(map[string]any)
	assert.Equal(t, "COMPLETED", data["status"])
	svc.AssertExpectations(t)
}

func TestCreatePayment_InvalidJSON_Returns400(t *testing.T) {
	svc := &mockPaymentService{}
	r := newPaymentRouterNoAuth(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "INVALID_BODY", body["error"].(map[string]any)["code"])
	svc.AssertNotCalled(t, "ProcessPayment")
}

func TestCreatePayment_MissingFields_Returns400(t *testing.T) {
	svc := &mockPaymentService{}

	w := doRequest(newPaymentRouterNoAuth(svc), http.MethodPost, "/api/v1/payments", map[string]any{})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "VALIDATION_ERROR", body["error"].(map[string]any)["code"])
	svc.AssertNotCalled(t, "ProcessPayment")
}

func TestCreatePayment_InvalidCurrency_Returns400(t *testing.T) {
	userID := uuid.New()
	svc := &mockPaymentService{}
	req := validCreateRequest(uuid.New(), userID)
	req["currency"] = "USDX" // 4 chars — violates validate:"len=3"

	w := doRequest(newPaymentRouterNoAuth(svc), http.MethodPost, "/api/v1/payments", req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "VALIDATION_ERROR", body["error"].(map[string]any)["code"])
	svc.AssertNotCalled(t, "ProcessPayment")
}

func TestCreatePayment_DuplicateIdempotencyKey_Returns409(t *testing.T) {
	userID := uuid.New()
	svc := &mockPaymentService{}
	svc.On("ProcessPayment", mock.Anything, mock.Anything).Return(nil, repository.ErrDuplicateIdempotencyKey)

	w := doRequest(newPaymentRouterNoAuth(svc), http.MethodPost, "/api/v1/payments",
		validCreateRequest(uuid.New(), userID))

	assert.Equal(t, http.StatusConflict, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "DUPLICATE_PAYMENT", body["error"].(map[string]any)["code"])
}

func TestCreatePayment_ServiceError_Returns500(t *testing.T) {
	userID := uuid.New()
	svc := &mockPaymentService{}
	svc.On("ProcessPayment", mock.Anything, mock.Anything).Return(nil, assert.AnError)

	w := doRequest(newPaymentRouterNoAuth(svc), http.MethodPost, "/api/v1/payments",
		validCreateRequest(uuid.New(), userID))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ─── GetByID ──────────────────────────────────────────────────────────────────

func TestGetByID_Success_Returns200(t *testing.T) {
	userID := uuid.New()
	resp := samplePaymentResponse(userID)
	svc := &mockPaymentService{}
	svc.On("GetByID", mock.Anything, resp.ID, userID, false).Return(resp, nil)

	w := doRequest(newPaymentRouter(svc, userID, "customer"), http.MethodGet,
		"/api/v1/payments/"+resp.ID.String(), nil)

	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, true, body["success"])
}

func TestGetByID_AdminBypass_Returns200(t *testing.T) {
	adminID := uuid.New()
	ownerID := uuid.New()
	resp := samplePaymentResponse(ownerID)
	svc := &mockPaymentService{}
	// Admin calls GetByID with isAdmin=true
	svc.On("GetByID", mock.Anything, resp.ID, adminID, true).Return(resp, nil)

	w := doRequest(newPaymentRouter(svc, adminID, "admin"), http.MethodGet,
		"/api/v1/payments/"+resp.ID.String(), nil)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetByID_InvalidUUID_Returns400(t *testing.T) {
	userID := uuid.New()
	svc := &mockPaymentService{}

	w := doRequest(newPaymentRouter(svc, userID, "customer"), http.MethodGet,
		"/api/v1/payments/not-a-uuid", nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "INVALID_PARAM", body["error"].(map[string]any)["code"])
	svc.AssertNotCalled(t, "GetByID")
}

func TestGetByID_NotFound_Returns404(t *testing.T) {
	userID := uuid.New()
	paymentID := uuid.New()
	svc := &mockPaymentService{}
	svc.On("GetByID", mock.Anything, paymentID, userID, false).Return(nil, service.ErrNotFound)

	w := doRequest(newPaymentRouter(svc, userID, "customer"), http.MethodGet,
		"/api/v1/payments/"+paymentID.String(), nil)

	assert.Equal(t, http.StatusNotFound, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "NOT_FOUND", body["error"].(map[string]any)["code"])
}

func TestGetByID_Forbidden_Returns403(t *testing.T) {
	userID := uuid.New()
	paymentID := uuid.New()
	svc := &mockPaymentService{}
	svc.On("GetByID", mock.Anything, paymentID, userID, false).Return(nil, service.ErrForbidden)

	w := doRequest(newPaymentRouter(svc, userID, "customer"), http.MethodGet,
		"/api/v1/payments/"+paymentID.String(), nil)

	assert.Equal(t, http.StatusForbidden, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "FORBIDDEN", body["error"].(map[string]any)["code"])
}

func TestGetByID_MissingAuth_Returns401(t *testing.T) {
	svc := &mockPaymentService{}
	w := doRequest(newPaymentRouterUnauthenticated(svc), http.MethodGet,
		"/api/v1/payments/"+uuid.NewString(), nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	svc.AssertNotCalled(t, "GetByID")
}

// ─── GetByOrderID ─────────────────────────────────────────────────────────────

func TestGetByOrderID_Success_Returns200(t *testing.T) {
	userID := uuid.New()
	orderID := uuid.New()
	resp := samplePaymentResponse(userID)
	svc := &mockPaymentService{}
	svc.On("GetByOrderID", mock.Anything, orderID, userID, false).Return(resp, nil)

	w := doRequest(newPaymentRouter(svc, userID, "customer"), http.MethodGet,
		"/api/v1/payments/order/"+orderID.String(), nil)

	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, true, body["success"])
}

func TestGetByOrderID_InvalidUUID_Returns400(t *testing.T) {
	userID := uuid.New()
	svc := &mockPaymentService{}

	w := doRequest(newPaymentRouter(svc, userID, "customer"), http.MethodGet,
		"/api/v1/payments/order/bad-uuid", nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "INVALID_PARAM", body["error"].(map[string]any)["code"])
}

func TestGetByOrderID_NotFound_Returns404(t *testing.T) {
	userID := uuid.New()
	orderID := uuid.New()
	svc := &mockPaymentService{}
	svc.On("GetByOrderID", mock.Anything, orderID, userID, false).Return(nil, service.ErrNotFound)

	w := doRequest(newPaymentRouter(svc, userID, "customer"), http.MethodGet,
		"/api/v1/payments/order/"+orderID.String(), nil)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ─── ListByUser ───────────────────────────────────────────────────────────────

func TestListByUser_DefaultPagination_Returns200(t *testing.T) {
	userID := uuid.New()
	svc := &mockPaymentService{}
	resp := samplePaymentResponse(userID)
	svc.On("ListByUser", mock.Anything, userID, 1, 20).
		Return([]dto.PaymentResponse{*resp}, int64(1), nil)

	w := doRequest(newPaymentRouter(svc, userID, "customer"), http.MethodGet,
		"/api/v1/payments", nil)

	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, true, body["success"])
	meta := body["meta"].(map[string]any)
	assert.Equal(t, float64(1), meta["page"])
	assert.Equal(t, float64(20), meta["size"])
	assert.Equal(t, float64(1), meta["totalElements"])
	assert.Equal(t, float64(1), meta["totalPages"])
	svc.AssertExpectations(t)
}

func TestListByUser_CustomPagination_Returns200(t *testing.T) {
	userID := uuid.New()
	svc := &mockPaymentService{}
	payments := make([]dto.PaymentResponse, 5)
	for i := range payments {
		payments[i] = *samplePaymentResponse(userID)
	}
	svc.On("ListByUser", mock.Anything, userID, 2, 5).Return(payments, int64(12), nil)

	w := doRequest(newPaymentRouter(svc, userID, "customer"), http.MethodGet,
		"/api/v1/payments?page=2&size=5", nil)

	assert.Equal(t, http.StatusOK, w.Code)
	meta := parseBody(t, w)["meta"].(map[string]any)
	assert.Equal(t, float64(2), meta["page"])
	assert.Equal(t, float64(5), meta["size"])
	assert.Equal(t, float64(12), meta["totalElements"])
	assert.Equal(t, float64(3), meta["totalPages"]) // ceil(12/5)=3
}

func TestListByUser_EmptyList_Returns200(t *testing.T) {
	userID := uuid.New()
	svc := &mockPaymentService{}
	svc.On("ListByUser", mock.Anything, userID, 1, 20).Return([]dto.PaymentResponse{}, int64(0), nil)

	w := doRequest(newPaymentRouter(svc, userID, "customer"), http.MethodGet,
		"/api/v1/payments", nil)

	assert.Equal(t, http.StatusOK, w.Code)
	meta := parseBody(t, w)["meta"].(map[string]any)
	assert.Equal(t, float64(0), meta["totalElements"])
	assert.Equal(t, float64(0), meta["totalPages"])
}

func TestListByUser_MissingAuth_Returns401(t *testing.T) {
	svc := &mockPaymentService{}
	w := doRequest(newPaymentRouterUnauthenticated(svc), http.MethodGet, "/api/v1/payments", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	svc.AssertNotCalled(t, "ListByUser")
}
