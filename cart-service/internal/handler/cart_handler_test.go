package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/handler"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/service"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ─── Mock ─────────────────────────────────────────────────────────────────────

type mockCartService struct{ mock.Mock }

func (m *mockCartService) GetCart(ctx context.Context, userID uuid.UUID) (*dto.CartResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.CartResponse), args.Error(1)
}

func (m *mockCartService) AddItem(ctx context.Context, userID uuid.UUID, req dto.AddItemRequest) (*dto.CartResponse, error) {
	args := m.Called(ctx, userID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.CartResponse), args.Error(1)
}

func (m *mockCartService) UpdateItem(ctx context.Context, userID uuid.UUID, productID int64, req dto.UpdateItemRequest) (*dto.CartResponse, error) {
	args := m.Called(ctx, userID, productID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.CartResponse), args.Error(1)
}

func (m *mockCartService) RemoveItem(ctx context.Context, userID uuid.UUID, productID int64) error {
	args := m.Called(ctx, userID, productID)
	return args.Error(0)
}

func (m *mockCartService) ClearCart(ctx context.Context, userID uuid.UUID) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// newCartRouter wires a CartHandler with a fake auth middleware that injects the given userID.
// The handler's getUserID() does val.(uuid.UUID) — it must be a uuid.UUID, not a string.
func newCartRouter(svc service.CartService, userID uuid.UUID) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", userID); c.Next() })
	h := handler.NewCartHandler(svc)
	v1 := r.Group("/api/v1/cart")
	v1.GET("", h.GetCart)
	v1.DELETE("", h.ClearCart)
	v1.POST("/items", h.AddItem)
	v1.PUT("/items/:productId", h.UpdateItem)
	v1.DELETE("/items/:productId", h.RemoveItem)
	return r
}

// newCartRouterNoAuth wires a CartHandler without injecting a userID into context.
func newCartRouterNoAuth(svc service.CartService) *gin.Engine {
	r := gin.New()
	h := handler.NewCartHandler(svc)
	v1 := r.Group("/api/v1/cart")
	v1.GET("", h.GetCart)
	v1.DELETE("", h.ClearCart)
	v1.POST("/items", h.AddItem)
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

func sampleCart(userID uuid.UUID) *dto.CartResponse {
	return &dto.CartResponse{
		UserID: userID.String(),
		Status: "ACTIVE",
		Items: []dto.CartItemResponse{
			{ProductID: 1, ProductName: "Widget A", Quantity: 2, UnitPrice: 9.99, Subtotal: 19.98},
			{ProductID: 2, ProductName: "Widget B", Quantity: 1, UnitPrice: 4.99, Subtotal: 4.99},
		},
		Total: 24.97,
	}
}

// ─── AddItem ──────────────────────────────────────────────────────────────────

func TestAddItem_Success_Returns201(t *testing.T) {
	userID := uuid.New()
	svc := &mockCartService{}
	cart := sampleCart(userID)
	svc.On("AddItem", mock.Anything, userID, dto.AddItemRequest{ProductID: 1, Quantity: 2}).Return(cart, nil)

	w := doRequest(newCartRouter(svc, userID), http.MethodPost, "/api/v1/cart/items",
		map[string]any{"product_id": 1, "quantity": 2})

	assert.Equal(t, http.StatusCreated, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, true, body["success"])
	svc.AssertExpectations(t)
}

func TestAddItem_InvalidJSON_Returns400(t *testing.T) {
	userID := uuid.New()
	svc := &mockCartService{}
	r := newCartRouter(svc, userID)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cart/items", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "INVALID_BODY", body["error"].(map[string]any)["code"])
	svc.AssertNotCalled(t, "AddItem")
}

func TestAddItem_MissingProductID_Returns400(t *testing.T) {
	userID := uuid.New()
	svc := &mockCartService{}

	w := doRequest(newCartRouter(svc, userID), http.MethodPost, "/api/v1/cart/items",
		map[string]any{"quantity": 2})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "VALIDATION_ERROR", body["error"].(map[string]any)["code"])
	svc.AssertNotCalled(t, "AddItem")
}

func TestAddItem_ZeroQuantity_Returns400(t *testing.T) {
	userID := uuid.New()
	svc := &mockCartService{}

	w := doRequest(newCartRouter(svc, userID), http.MethodPost, "/api/v1/cart/items",
		map[string]any{"product_id": 1, "quantity": 0})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "VALIDATION_ERROR", body["error"].(map[string]any)["code"])
	svc.AssertNotCalled(t, "AddItem")
}

func TestAddItem_ProductNotFound_Returns404(t *testing.T) {
	userID := uuid.New()
	svc := &mockCartService{}
	svc.On("AddItem", mock.Anything, userID, mock.Anything).Return(nil, service.ErrProductNotFound)

	w := doRequest(newCartRouter(svc, userID), http.MethodPost, "/api/v1/cart/items",
		map[string]any{"product_id": 99, "quantity": 1})

	assert.Equal(t, http.StatusNotFound, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "NOT_FOUND", body["error"].(map[string]any)["code"])
}

func TestAddItem_ServiceUnavailable_Returns503(t *testing.T) {
	userID := uuid.New()
	svc := &mockCartService{}
	svc.On("AddItem", mock.Anything, userID, mock.Anything).Return(nil, service.ErrProductServiceUnavailable)

	w := doRequest(newCartRouter(svc, userID), http.MethodPost, "/api/v1/cart/items",
		map[string]any{"product_id": 1, "quantity": 1})

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "SERVICE_UNAVAILABLE", body["error"].(map[string]any)["code"])
}

func TestAddItem_ConcurrentUpdate_Returns409(t *testing.T) {
	userID := uuid.New()
	svc := &mockCartService{}
	svc.On("AddItem", mock.Anything, userID, mock.Anything).Return(nil, service.ErrConcurrentUpdate)

	w := doRequest(newCartRouter(svc, userID), http.MethodPost, "/api/v1/cart/items",
		map[string]any{"product_id": 1, "quantity": 1})

	assert.Equal(t, http.StatusConflict, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "CONCURRENT_UPDATE", body["error"].(map[string]any)["code"])
}

// ─── GetCart ──────────────────────────────────────────────────────────────────

func TestGetCart_WithItems_Returns200(t *testing.T) {
	userID := uuid.New()
	svc := &mockCartService{}
	svc.On("GetCart", mock.Anything, userID).Return(sampleCart(userID), nil)

	w := doRequest(newCartRouter(svc, userID), http.MethodGet, "/api/v1/cart", nil)

	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, true, body["success"])
	data := body["data"].(map[string]any)
	items := data["items"].([]any)
	assert.Len(t, items, 2)
}

func TestGetCart_EmptyCart_Returns200(t *testing.T) {
	userID := uuid.New()
	svc := &mockCartService{}
	emptyCart := &dto.CartResponse{
		UserID: userID.String(),
		Status: "ACTIVE",
		Items:  []dto.CartItemResponse{},
		Total:  0,
	}
	svc.On("GetCart", mock.Anything, userID).Return(emptyCart, nil)

	w := doRequest(newCartRouter(svc, userID), http.MethodGet, "/api/v1/cart", nil)

	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	data := body["data"].(map[string]any)
	items := data["items"].([]any)
	assert.Empty(t, items)
}

func TestGetCart_NoAuth_Returns401(t *testing.T) {
	svc := &mockCartService{}
	w := doRequest(newCartRouterNoAuth(svc), http.MethodGet, "/api/v1/cart", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	svc.AssertNotCalled(t, "GetCart")
}

// ─── UpdateItem ───────────────────────────────────────────────────────────────

func TestUpdateItem_Success_Returns200(t *testing.T) {
	userID := uuid.New()
	svc := &mockCartService{}
	svc.On("UpdateItem", mock.Anything, userID, int64(1), dto.UpdateItemRequest{Quantity: 5}).
		Return(sampleCart(userID), nil)

	w := doRequest(newCartRouter(svc, userID), http.MethodPut, "/api/v1/cart/items/1",
		map[string]any{"quantity": 5})

	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, true, body["success"])
}

func TestUpdateItem_InvalidProductID_Returns400(t *testing.T) {
	userID := uuid.New()
	svc := &mockCartService{}

	w := doRequest(newCartRouter(svc, userID), http.MethodPut, "/api/v1/cart/items/abc",
		map[string]any{"quantity": 5})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "INVALID_PARAM", body["error"].(map[string]any)["code"])
	svc.AssertNotCalled(t, "UpdateItem")
}

func TestUpdateItem_ItemNotInCart_Returns404(t *testing.T) {
	userID := uuid.New()
	svc := &mockCartService{}
	svc.On("UpdateItem", mock.Anything, userID, int64(99), mock.Anything).
		Return(nil, service.ErrItemNotInCart)

	w := doRequest(newCartRouter(svc, userID), http.MethodPut, "/api/v1/cart/items/99",
		map[string]any{"quantity": 3})

	assert.Equal(t, http.StatusNotFound, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "NOT_FOUND", body["error"].(map[string]any)["code"])
}

// ─── RemoveItem ───────────────────────────────────────────────────────────────

func TestRemoveItem_Success_Returns204(t *testing.T) {
	userID := uuid.New()
	svc := &mockCartService{}
	svc.On("RemoveItem", mock.Anything, userID, int64(1)).Return(nil)

	w := doRequest(newCartRouter(svc, userID), http.MethodDelete, "/api/v1/cart/items/1", nil)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String())
}

// ─── ClearCart ────────────────────────────────────────────────────────────────

func TestClearCart_Success_Returns204(t *testing.T) {
	userID := uuid.New()
	svc := &mockCartService{}
	svc.On("ClearCart", mock.Anything, userID).Return(nil)

	w := doRequest(newCartRouter(svc, userID), http.MethodDelete, "/api/v1/cart", nil)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestClearCart_ServiceError_Returns500(t *testing.T) {
	userID := uuid.New()
	svc := &mockCartService{}
	svc.On("ClearCart", mock.Anything, userID).Return(assert.AnError)

	w := doRequest(newCartRouter(svc, userID), http.MethodDelete, "/api/v1/cart", nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "INTERNAL_ERROR", body["error"].(map[string]any)["code"])
}
