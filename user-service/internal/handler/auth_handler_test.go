package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/handler"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/model"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/service"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ─── Mock service ─────────────────────────────────────────────────────────────

type mockAuthService struct {
	mock.Mock
}

func (m *mockAuthService) Register(ctx context.Context, req dto.RegisterRequest) (*model.User, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func newRouter(svc service.AuthService) *gin.Engine {
	r := gin.New()
	h := handler.NewAuthHandler(svc)
	r.POST("/api/v1/auth/register", h.Register)
	return r
}

func postJSON(router *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
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

// ─── Tests ───────────────────────────────────────────────────────────────────

func TestRegisterHandler_Success_Returns201(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	req := dto.RegisterRequest{
		Email:     "john@example.com",
		Password:  "secret123",
		FirstName: "John",
		LastName:  "Doe",
	}
	returnedUser := &model.User{
		Email: "john@example.com",
		Role:  "customer",
		Profile: &model.UserProfile{
			FirstName: "John",
			LastName:  "Doe",
		},
	}
	svc.On("Register", mock.Anything, req).Return(returnedUser, nil)

	w := postJSON(router, "/api/v1/auth/register", req)

	assert.Equal(t, http.StatusCreated, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, true, body["success"])
	data := body["data"].(map[string]any)
	assert.Equal(t, "john@example.com", data["email"])
	assert.Equal(t, "customer", data["role"])
	assert.Equal(t, "John", data["first_name"])
	assert.Equal(t, "Doe", data["last_name"])
	svc.AssertExpectations(t)
}

func TestRegisterHandler_DuplicateEmail_Returns409(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	req := dto.RegisterRequest{
		Email:     "john@example.com",
		Password:  "secret123",
		FirstName: "John",
		LastName:  "Doe",
	}
	svc.On("Register", mock.Anything, req).Return(nil, service.ErrDuplicateEmail)

	w := postJSON(router, "/api/v1/auth/register", req)

	assert.Equal(t, http.StatusConflict, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, false, body["success"])
	errDetail := body["error"].(map[string]any)
	assert.Equal(t, "DUPLICATE_EMAIL", errDetail["code"])
}

func TestRegisterHandler_ValidationError_Returns400(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	// missing first_name, last_name; bad email; password too short
	w := postJSON(router, "/api/v1/auth/register", map[string]any{
		"email":    "not-an-email",
		"password": "short",
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, false, body["success"])
	errDetail := body["error"].(map[string]any)
	assert.Equal(t, "VALIDATION_ERROR", errDetail["code"])
	details := errDetail["details"].(map[string]any)
	assert.Contains(t, details, "Email")
	assert.Contains(t, details, "Password")
	assert.Contains(t, details, "FirstName")
	assert.Contains(t, details, "LastName")
	svc.AssertNotCalled(t, "Register")
}

func TestRegisterHandler_InvalidJSON_Returns400(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, false, body["success"])
	errDetail := body["error"].(map[string]any)
	assert.Equal(t, "INVALID_BODY", errDetail["code"])
	svc.AssertNotCalled(t, "Register")
}

func TestRegisterHandler_ServiceError_Returns500(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	req := dto.RegisterRequest{
		Email:     "john@example.com",
		Password:  "secret123",
		FirstName: "John",
		LastName:  "Doe",
	}
	svc.On("Register", mock.Anything, req).Return(nil, assert.AnError)

	w := postJSON(router, "/api/v1/auth/register", req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, false, body["success"])
}
