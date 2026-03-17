//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/handler"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/middleware"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/model"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/repository"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/service"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/blacklist"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/loginattempt"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/session"
)

// TestProfileFlow_WithContainers spins up real Postgres and Redis containers,
// wires a full production stack, and exercises:
//
//	Register → Login → GET profile → PUT profile → re-read profile → Logout → token rejected
func TestProfileFlow_WithContainers(t *testing.T) {
	ctx := context.Background()

	// ── 1. Postgres container ─────────────────────────────────────────────
	pgCtr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("ecommerce_users"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
	)
	require.NoError(t, err, "start postgres container")
	t.Cleanup(func() { _ = pgCtr.Terminate(ctx) })

	// ── 2. Redis container ────────────────────────────────────────────────
	redisCtr, err := tcredis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err, "start redis container")
	t.Cleanup(func() { _ = redisCtr.Terminate(ctx) })

	// ── 3. Connect GORM ───────────────────────────────────────────────────
	dsn, err := pgCtr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err, "open gorm connection")

	// ── 4. Connect redis client ───────────────────────────────────────────
	redisURL, err := redisCtr.ConnectionString(ctx)
	require.NoError(t, err)
	opt, err := redis.ParseURL(redisURL)
	require.NoError(t, err)
	rdb := redis.NewClient(opt)
	t.Cleanup(func() { _ = rdb.Close() })

	// ── 5. Migrate schema ─────────────────────────────────────────────────
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.UserProfile{},
		&model.UserAddress{},
		&model.AuthToken{},
	), "automigrate")

	// ── 6. In-memory RSA key pair ─────────────────────────────────────────
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pubKey := &privKey.PublicKey

	// ── 7. Wire full production stack ─────────────────────────────────────
	userRepo := repository.NewUserRepository(db)
	authTokenRepo := repository.NewAuthTokenRepository(db)
	addrRepo := repository.NewAddressRepository(db)
	bl := blacklist.New(rdb)
	sc := session.New(rdb)
	ac := loginattempt.New(rdb)

	authSvc := service.NewAuthService(userRepo, authTokenRepo, db, bl, sc, ac, privKey, pubKey)
	userSvc := service.NewUserService(userRepo, addrRepo, sc)

	authHandler := handler.NewAuthHandler(authSvc)
	userHandler := handler.NewUserHandler(userSvc)
	authMw := middleware.Auth(pubKey, bl)

	r := gin.New()
	r.Use(middleware.Recovery())
	v1 := r.Group("/api/v1")

	authRoutes := v1.Group("/auth")
	authRoutes.POST("/register", authHandler.Register)
	authRoutes.POST("/login", authHandler.Login)

	protectedAuth := v1.Group("/auth")
	protectedAuth.Use(authMw)
	protectedAuth.POST("/logout", authHandler.Logout)

	users := v1.Group("/users")
	users.Use(authMw)
	users.GET("/profile", userHandler.GetProfile)
	users.PUT("/profile", userHandler.UpdateProfile)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	email := fmt.Sprintf("profile_%d@example.com", time.Now().UnixNano())
	const pw = "Test1234!"

	// ── Step 1: Register ──────────────────────────────────────────────────
	resp := postTo(t, srv.URL, "/api/v1/auth/register", map[string]any{
		"email": email, "password": pw,
		"first_name": "Jane", "last_name": "Doe",
	}, "")
	body := readBody(t, resp)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "register: %v", body)
	assert.Equal(t, true, body["success"])

	// ── Step 2: Login ─────────────────────────────────────────────────────
	resp = postTo(t, srv.URL, "/api/v1/auth/login", map[string]any{
		"email": email, "password": pw,
	}, "")
	body = readBody(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, "login: %v", body)
	accessToken := dig(body, "data", "access_token")
	require.NotEmpty(t, accessToken, "access_token must be present after login")

	// ── Step 3: GET /users/profile ────────────────────────────────────────
	resp = getTo(t, srv.URL, "/api/v1/users/profile", accessToken)
	body = readBody(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, "get profile: %v", body)
	assert.Equal(t, email, dig(body, "data", "email"))
	assert.Equal(t, "Jane", dig(body, "data", "first_name"))
	assert.Equal(t, "Doe", dig(body, "data", "last_name"))

	// ── Step 4: PUT /users/profile — update name + phone ─────────────────
	resp = putTo(t, srv.URL, "/api/v1/users/profile", map[string]any{
		"first_name": "Janet", "last_name": "Smith", "phone": "+84901234567",
	}, accessToken)
	body = readBody(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, "update profile: %v", body)
	assert.Equal(t, "Janet", dig(body, "data", "first_name"))
	assert.Equal(t, "Smith", dig(body, "data", "last_name"))
	assert.Equal(t, "+84901234567", dig(body, "data", "phone"))

	// ── Step 5: GET /users/profile — verify update persisted ─────────────
	resp = getTo(t, srv.URL, "/api/v1/users/profile", accessToken)
	body = readBody(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, "re-read profile: %v", body)
	assert.Equal(t, "Janet", dig(body, "data", "first_name"), "updated first_name must persist")
	assert.Equal(t, "Smith", dig(body, "data", "last_name"), "updated last_name must persist")
	assert.Equal(t, "+84901234567", dig(body, "data", "phone"), "phone must persist")

	// ── Step 6: Logout ────────────────────────────────────────────────────
	resp = postTo(t, srv.URL, "/api/v1/auth/logout", nil, accessToken)
	body = readBody(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, "logout: %v", body)
	assert.Equal(t, "logged out", dig(body, "data", "message"))

	// ── Step 7: Profile endpoint rejects blacklisted token ───────────────
	resp = getTo(t, srv.URL, "/api/v1/users/profile", accessToken)
	body = readBody(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"blacklisted token must be rejected: %v", body)
	assert.Equal(t, "UNAUTHORIZED", errCode(body))
}

// ─── URL-parametric HTTP helpers ─────────────────────────────────────────────
// These mirror doPost/doGet from auth_flow_test.go but accept an explicit
// base URL so the test can target its own httptest.Server instead of testServer.

func postTo(t *testing.T, base, path string, body any, bearer string) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(http.MethodPost, base+path, r)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func getTo(t *testing.T, base, path string, bearer string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	require.NoError(t, err)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func putTo(t *testing.T, base, path string, body any, bearer string) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, base+path, bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// readBody deserialises the response body into map[string]any.
// It is a local alias for parseBody (defined in auth_flow_test.go) so the
// test function stays readable with a shorter name.
func readBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}
