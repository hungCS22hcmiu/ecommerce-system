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

func (m *mockAuthService) Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.LoginResponse), args.Error(1)
}

func (m *mockAuthService) Refresh(ctx context.Context, refreshToken string) (*dto.LoginResponse, error) {
	args := m.Called(ctx, refreshToken)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.LoginResponse), args.Error(1)
}

func (m *mockAuthService) Logout(ctx context.Context, accessToken string) error {
	args := m.Called(ctx, accessToken)
	return args.Error(0)
}

func (m *mockAuthService) VerifyEmail(ctx context.Context, req dto.VerifyEmailRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *mockAuthService) ResendVerification(ctx context.Context, req dto.ResendVerificationRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func newRouter(svc service.AuthService) *gin.Engine {
	r := gin.New()
	h := handler.NewAuthHandler(svc)
	r.POST("/api/v1/auth/register", h.Register)
	r.POST("/api/v1/auth/login", h.Login)
	r.POST("/api/v1/auth/refresh", h.Refresh)
	r.POST("/api/v1/auth/logout", h.Logout)
	r.POST("/api/v1/auth/verify-email", h.VerifyEmail)
	r.POST("/api/v1/auth/resend-verification", h.ResendVerification)
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

// ─── Register tests ───────────────────────────────────────────────────────────

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

// ─── Login tests ──────────────────────────────────────────────────────────────

func TestLoginHandler_Success_Returns200(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	req := dto.LoginRequest{Email: "john@example.com", Password: "secret123"}
	loginResp := &dto.LoginResponse{
		AccessToken:  "access.token.here",
		RefreshToken: "refresh-token-hex",
		User:         dto.UserResponse{ID: "uuid-1", Email: "john@example.com", Role: "customer"},
	}
	svc.On("Login", mock.Anything, req).Return(loginResp, nil)

	w := postJSON(router, "/api/v1/auth/login", req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	assert.True(t, body["success"].(bool))
	data := body["data"].(map[string]any)
	assert.Equal(t, "access.token.here", data["access_token"])
	assert.Equal(t, "refresh-token-hex", data["refresh_token"])
	svc.AssertExpectations(t)
}

func TestLoginHandler_InvalidCredentials_Returns401(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	req := dto.LoginRequest{Email: "john@example.com", Password: "wrong"}
	svc.On("Login", mock.Anything, req).Return(nil, service.ErrInvalidCredentials)

	w := postJSON(router, "/api/v1/auth/login", req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	body := parseBody(t, w)
	assert.False(t, body["success"].(bool))
	errDetail := body["error"].(map[string]any)
	assert.Equal(t, "INVALID_CREDENTIALS", errDetail["code"])
}

func TestLoginHandler_AccountLocked_Returns403(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	req := dto.LoginRequest{Email: "locked@example.com", Password: "secret123"}
	svc.On("Login", mock.Anything, req).Return(nil, service.ErrAccountLocked)

	w := postJSON(router, "/api/v1/auth/login", req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	body := parseBody(t, w)
	assert.False(t, body["success"].(bool))
	errDetail := body["error"].(map[string]any)
	assert.Equal(t, "ACCOUNT_LOCKED", errDetail["code"])
}

func TestLoginHandler_ValidationError_Returns400(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	w := postJSON(router, "/api/v1/auth/login", map[string]any{"email": "not-valid"})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	svc.AssertNotCalled(t, "Login")
}

// ─── Refresh tests ────────────────────────────────────────────────────────────

func TestRefreshHandler_Success_Returns200(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	refreshToken := "some-refresh-token"
	refreshResp := &dto.LoginResponse{
		AccessToken: "new.access.token",
		User:        dto.UserResponse{ID: "uuid-1", Email: "john@example.com", Role: "customer"},
	}
	svc.On("Refresh", mock.Anything, refreshToken).Return(refreshResp, nil)

	w := postJSON(router, "/api/v1/auth/refresh", map[string]any{"refresh_token": refreshToken})

	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	assert.True(t, body["success"].(bool))
	data := body["data"].(map[string]any)
	assert.Equal(t, "new.access.token", data["access_token"])
	svc.AssertExpectations(t)
}

func TestRefreshHandler_InvalidToken_Returns401(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	svc.On("Refresh", mock.Anything, "bad-token").Return(nil, service.ErrInvalidToken)

	w := postJSON(router, "/api/v1/auth/refresh", map[string]any{"refresh_token": "bad-token"})

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	body := parseBody(t, w)
	assert.False(t, body["success"].(bool))
	errDetail := body["error"].(map[string]any)
	assert.Equal(t, "INVALID_TOKEN", errDetail["code"])
}

func TestRefreshHandler_ValidationError_Returns400(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	// missing refresh_token field
	w := postJSON(router, "/api/v1/auth/refresh", map[string]any{})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := parseBody(t, w)
	assert.False(t, body["success"].(bool))
	errDetail := body["error"].(map[string]any)
	assert.Equal(t, "VALIDATION_ERROR", errDetail["code"])
	svc.AssertNotCalled(t, "Refresh")
}

func TestRefreshHandler_InvalidJSON_Returns400(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := parseBody(t, w)
	assert.False(t, body["success"].(bool))
	errDetail := body["error"].(map[string]any)
	assert.Equal(t, "INVALID_BODY", errDetail["code"])
	svc.AssertNotCalled(t, "Refresh")
}

func TestRefreshHandler_ServiceError_Returns500(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	svc.On("Refresh", mock.Anything, "some-token").Return(nil, assert.AnError)

	w := postJSON(router, "/api/v1/auth/refresh", map[string]any{"refresh_token": "some-token"})

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	body := parseBody(t, w)
	assert.False(t, body["success"].(bool))
	svc.AssertExpectations(t)
}

func TestLoginHandler_InvalidJSON_Returns400(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := parseBody(t, w)
	assert.False(t, body["success"].(bool))
	errDetail := body["error"].(map[string]any)
	assert.Equal(t, "INVALID_BODY", errDetail["code"])
	svc.AssertNotCalled(t, "Login")
}

func TestLoginHandler_ServiceError_Returns500(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	req := dto.LoginRequest{Email: "john@example.com", Password: "secret123"}
	svc.On("Login", mock.Anything, req).Return(nil, assert.AnError)

	w := postJSON(router, "/api/v1/auth/login", req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	body := parseBody(t, w)
	assert.False(t, body["success"].(bool))
	svc.AssertExpectations(t)
}

// ─── Logout tests ─────────────────────────────────────────────────────────────

func logoutRequest(router *gin.Engine, bearerToken string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestLogoutHandler_Success_Returns200(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	svc.On("Logout", mock.Anything, "raw.token.value").Return(nil)

	w := logoutRequest(router, "raw.token.value")

	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	assert.True(t, body["success"].(bool))
	data := body["data"].(map[string]any)
	assert.Equal(t, "logged out", data["message"])
	svc.AssertExpectations(t)
}

func TestLogoutHandler_InvalidToken_Returns401(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	svc.On("Logout", mock.Anything, "bad.token").Return(service.ErrInvalidToken)

	w := logoutRequest(router, "bad.token")

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	body := parseBody(t, w)
	assert.False(t, body["success"].(bool))
	errDetail := body["error"].(map[string]any)
	assert.Equal(t, "UNAUTHORIZED", errDetail["code"])
	svc.AssertExpectations(t)
}

func TestLogoutHandler_ServiceError_Returns500(t *testing.T) {
	svc := new(mockAuthService)
	router := newRouter(svc)

	svc.On("Logout", mock.Anything, "some.token").Return(assert.AnError)

	w := logoutRequest(router, "some.token")

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	body := parseBody(t, w)
	assert.False(t, body["success"].(bool))
	svc.AssertExpectations(t)
}
