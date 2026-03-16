package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/handler"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/middleware"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/service"
)

// ─── Mock user service ────────────────────────────────────────────────────────

type mockUserService struct {
	mock.Mock
}

func (m *mockUserService) GetProfile(ctx context.Context, userID uuid.UUID) (*dto.ProfileResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.ProfileResponse), args.Error(1)
}

func (m *mockUserService) UpdateProfile(ctx context.Context, userID uuid.UUID, req dto.UpdateProfileRequest) (*dto.ProfileResponse, error) {
	args := m.Called(ctx, userID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.ProfileResponse), args.Error(1)
}

func (m *mockUserService) AddAddress(ctx context.Context, userID uuid.UUID, req dto.CreateAddressRequest) (*dto.AddressResponse, error) {
	args := m.Called(ctx, userID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.AddressResponse), args.Error(1)
}

func (m *mockUserService) UpdateAddress(ctx context.Context, userID, addressID uuid.UUID, req dto.UpdateAddressRequest) (*dto.AddressResponse, error) {
	args := m.Called(ctx, userID, addressID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.AddressResponse), args.Error(1)
}

func (m *mockUserService) DeleteAddress(ctx context.Context, userID, addressID uuid.UUID) error {
	args := m.Called(ctx, userID, addressID)
	return args.Error(0)
}

func (m *mockUserService) SetDefaultAddress(ctx context.Context, userID, addressID uuid.UUID) (*dto.AddressResponse, error) {
	args := m.Called(ctx, userID, addressID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.AddressResponse), args.Error(1)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func newUserRouter(svc service.UserService, userID string) *gin.Engine {
	r := gin.New()
	h := handler.NewUserHandler(svc)
	// Simulate Auth middleware by injecting userID into context
	r.Use(func(c *gin.Context) {
		c.Set(middleware.CtxUserID, userID)
		c.Next()
	})
	r.GET("/api/v1/users/profile", h.GetProfile)
	r.PUT("/api/v1/users/profile", h.UpdateProfile)
	r.POST("/api/v1/users/addresses", h.AddAddress)
	r.PUT("/api/v1/users/addresses/:id", h.UpdateAddress)
	r.DELETE("/api/v1/users/addresses/:id", h.DeleteAddress)
	r.PUT("/api/v1/users/addresses/:id/default", h.SetDefaultAddress)
	return r
}

func doRequest(router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	var buf *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	} else {
		buf = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func sampleProfile(id uuid.UUID) *dto.ProfileResponse {
	return &dto.ProfileResponse{
		ID:        id.String(),
		Email:     "alice@example.com",
		Role:      "customer",
		FirstName: "Alice",
		LastName:  "Smith",
		Phone:     "0900000000",
		Addresses: []dto.AddressResponse{},
	}
}

func sampleAddress(id uuid.UUID) *dto.AddressResponse {
	return &dto.AddressResponse{
		ID:           id.String(),
		Label:        "home",
		AddressLine1: "123 Main St",
		City:         "Ho Chi Minh",
		Country:      "Vietnam",
		IsDefault:    false,
	}
}

// ─── GetProfile tests ─────────────────────────────────────────────────────────

func TestGetProfileHandler_Success_Returns200(t *testing.T) {
	userID := uuid.New()
	svc := new(mockUserService)
	router := newUserRouter(svc, userID.String())

	svc.On("GetProfile", mock.Anything, userID).Return(sampleProfile(userID), nil)

	w := doRequest(router, http.MethodGet, "/api/v1/users/profile", nil)

	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	assert.True(t, body["success"].(bool))
	data := body["data"].(map[string]any)
	assert.Equal(t, userID.String(), data["id"])
	assert.Equal(t, "alice@example.com", data["email"])
	assert.Equal(t, "Alice", data["first_name"])
	svc.AssertExpectations(t)
}

func TestGetProfileHandler_NotFound_Returns404(t *testing.T) {
	userID := uuid.New()
	svc := new(mockUserService)
	router := newUserRouter(svc, userID.String())

	svc.On("GetProfile", mock.Anything, userID).Return(nil, service.ErrUserNotFound)

	w := doRequest(router, http.MethodGet, "/api/v1/users/profile", nil)

	assert.Equal(t, http.StatusNotFound, w.Code)
	body := parseBody(t, w)
	assert.False(t, body["success"].(bool))
	svc.AssertExpectations(t)
}

func TestGetProfileHandler_MissingUserID_Returns401(t *testing.T) {
	svc := new(mockUserService)
	// Router with empty userID (simulates missing middleware)
	r := gin.New()
	h := handler.NewUserHandler(svc)
	r.GET("/api/v1/users/profile", h.GetProfile)

	w := doRequest(r, http.MethodGet, "/api/v1/users/profile", nil)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	svc.AssertNotCalled(t, "GetProfile")
}

// ─── UpdateProfile tests ──────────────────────────────────────────────────────

func TestUpdateProfileHandler_Success_Returns200(t *testing.T) {
	userID := uuid.New()
	svc := new(mockUserService)
	router := newUserRouter(svc, userID.String())

	req := dto.UpdateProfileRequest{FirstName: "Jane", LastName: "Doe", Phone: "0912345678"}
	updated := sampleProfile(userID)
	updated.FirstName = "Jane"
	updated.LastName = "Doe"
	updated.Phone = "0912345678"

	svc.On("UpdateProfile", mock.Anything, userID, req).Return(updated, nil)

	w := doRequest(router, http.MethodPut, "/api/v1/users/profile", req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	assert.True(t, body["success"].(bool))
	data := body["data"].(map[string]any)
	assert.Equal(t, "Jane", data["first_name"])
	assert.Equal(t, "Doe", data["last_name"])
	assert.Equal(t, "0912345678", data["phone"])
	svc.AssertExpectations(t)
}

func TestUpdateProfileHandler_ValidationError_Returns400(t *testing.T) {
	userID := uuid.New()
	svc := new(mockUserService)
	router := newUserRouter(svc, userID.String())

	// Missing required first_name and last_name
	w := doRequest(router, http.MethodPut, "/api/v1/users/profile", map[string]any{})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := parseBody(t, w)
	require.False(t, body["success"].(bool))
	errDetail := body["error"].(map[string]any)
	assert.Equal(t, "VALIDATION_ERROR", errDetail["code"])
	svc.AssertNotCalled(t, "UpdateProfile")
}

func TestUpdateProfileHandler_InvalidJSON_Returns400(t *testing.T) {
	userID := uuid.New()
	svc := new(mockUserService)
	router := newUserRouter(svc, userID.String())

	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/profile", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := parseBody(t, w)
	errDetail := body["error"].(map[string]any)
	assert.Equal(t, "INVALID_BODY", errDetail["code"])
	svc.AssertNotCalled(t, "UpdateProfile")
}

func TestUpdateProfileHandler_NotFound_Returns404(t *testing.T) {
	userID := uuid.New()
	svc := new(mockUserService)
	router := newUserRouter(svc, userID.String())

	req := dto.UpdateProfileRequest{FirstName: "Jane", LastName: "Doe"}
	svc.On("UpdateProfile", mock.Anything, userID, req).Return(nil, service.ErrUserNotFound)

	w := doRequest(router, http.MethodPut, "/api/v1/users/profile", req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	svc.AssertExpectations(t)
}

// ─── AddAddress tests ─────────────────────────────────────────────────────────

func TestAddAddressHandler_Success_Returns201(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	svc := new(mockUserService)
	router := newUserRouter(svc, userID.String())

	req := dto.CreateAddressRequest{AddressLine1: "123 Main St", City: "HCMC", Country: "Vietnam"}
	svc.On("AddAddress", mock.Anything, userID, req).Return(sampleAddress(addrID), nil)

	w := doRequest(router, http.MethodPost, "/api/v1/users/addresses", req)

	assert.Equal(t, http.StatusCreated, w.Code)
	body := parseBody(t, w)
	assert.True(t, body["success"].(bool))
	data := body["data"].(map[string]any)
	assert.Equal(t, "123 Main St", data["address_line1"])
	svc.AssertExpectations(t)
}

func TestAddAddressHandler_ValidationError_Returns400(t *testing.T) {
	userID := uuid.New()
	svc := new(mockUserService)
	router := newUserRouter(svc, userID.String())

	// Missing required address_line1 and country
	w := doRequest(router, http.MethodPost, "/api/v1/users/addresses", map[string]any{"city": "HCMC"})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := parseBody(t, w)
	errDetail := body["error"].(map[string]any)
	assert.Equal(t, "VALIDATION_ERROR", errDetail["code"])
	svc.AssertNotCalled(t, "AddAddress")
}

// ─── UpdateAddress tests ──────────────────────────────────────────────────────

func TestUpdateAddressHandler_Success_Returns200(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	svc := new(mockUserService)
	router := newUserRouter(svc, userID.String())

	req := dto.UpdateAddressRequest{AddressLine1: "456 New St", City: "Hanoi", Country: "Vietnam"}
	updated := sampleAddress(addrID)
	updated.AddressLine1 = "456 New St"
	svc.On("UpdateAddress", mock.Anything, userID, addrID, req).Return(updated, nil)

	w := doRequest(router, http.MethodPut, fmt.Sprintf("/api/v1/users/addresses/%s", addrID), req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	assert.True(t, body["success"].(bool))
	svc.AssertExpectations(t)
}

func TestUpdateAddressHandler_NotFound_Returns404(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	svc := new(mockUserService)
	router := newUserRouter(svc, userID.String())

	req := dto.UpdateAddressRequest{AddressLine1: "X", City: "Y", Country: "Z"}
	svc.On("UpdateAddress", mock.Anything, userID, addrID, req).Return(nil, service.ErrAddressNotFound)

	w := doRequest(router, http.MethodPut, fmt.Sprintf("/api/v1/users/addresses/%s", addrID), req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	svc.AssertExpectations(t)
}

func TestUpdateAddressHandler_Forbidden_Returns403(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	svc := new(mockUserService)
	router := newUserRouter(svc, userID.String())

	req := dto.UpdateAddressRequest{AddressLine1: "X", City: "Y", Country: "Z"}
	svc.On("UpdateAddress", mock.Anything, userID, addrID, req).Return(nil, service.ErrAddressForbidden)

	w := doRequest(router, http.MethodPut, fmt.Sprintf("/api/v1/users/addresses/%s", addrID), req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	svc.AssertExpectations(t)
}

// ─── DeleteAddress tests ──────────────────────────────────────────────────────

func TestDeleteAddressHandler_Success_Returns204(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	svc := new(mockUserService)
	router := newUserRouter(svc, userID.String())

	svc.On("DeleteAddress", mock.Anything, userID, addrID).Return(nil)

	w := doRequest(router, http.MethodDelete, fmt.Sprintf("/api/v1/users/addresses/%s", addrID), nil)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String())
	svc.AssertExpectations(t)
}

func TestDeleteAddressHandler_NotFound_Returns404(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	svc := new(mockUserService)
	router := newUserRouter(svc, userID.String())

	svc.On("DeleteAddress", mock.Anything, userID, addrID).Return(service.ErrAddressNotFound)

	w := doRequest(router, http.MethodDelete, fmt.Sprintf("/api/v1/users/addresses/%s", addrID), nil)

	assert.Equal(t, http.StatusNotFound, w.Code)
	svc.AssertExpectations(t)
}

// ─── SetDefaultAddress tests ──────────────────────────────────────────────────

func TestSetDefaultAddressHandler_Success_Returns200(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	svc := new(mockUserService)
	router := newUserRouter(svc, userID.String())

	defaultAddr := sampleAddress(addrID)
	defaultAddr.IsDefault = true
	svc.On("SetDefaultAddress", mock.Anything, userID, addrID).Return(defaultAddr, nil)

	w := doRequest(router, http.MethodPut, fmt.Sprintf("/api/v1/users/addresses/%s/default", addrID), nil)

	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	assert.True(t, body["success"].(bool))
	data := body["data"].(map[string]any)
	assert.Equal(t, true, data["is_default"])
	svc.AssertExpectations(t)
}

func TestSetDefaultAddressHandler_NotFound_Returns404(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	svc := new(mockUserService)
	router := newUserRouter(svc, userID.String())

	svc.On("SetDefaultAddress", mock.Anything, userID, addrID).Return(nil, service.ErrAddressNotFound)

	w := doRequest(router, http.MethodPut, fmt.Sprintf("/api/v1/users/addresses/%s/default", addrID), nil)

	assert.Equal(t, http.StatusNotFound, w.Code)
	svc.AssertExpectations(t)
}
