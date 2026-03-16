//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
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

// ─── Shared infrastructure (initialised once in TestMain) ─────────────────────

var (
	testServer *httptest.Server
	testRDB    *redis.Client
	testDB     *gorm.DB
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	// ── Postgres ──────────────────────────────────────────────────────────
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
		getEnv("DB_HOST", "localhost"),
		getEnv("DB_PORT", "5432"),
		getEnv("DB_USER", "postgres"),
		getEnv("DB_PASSWORD", "postgres"),
		getEnv("DB_NAME", "ecommerce_users"),
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		fmt.Printf("SKIP: cannot connect to postgres: %v\n", err)
		os.Exit(0)
	}
	testDB = db

	// ── Redis ─────────────────────────────────────────────────────────────
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", getEnv("REDIS_HOST", "localhost"), getEnv("REDIS_PORT", "6379")),
		Password: getEnv("REDIS_PASSWORD", ""),
		DB:       0,
	})
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		fmt.Printf("SKIP: cannot connect to redis: %v\n", err)
		os.Exit(0)
	}
	testRDB = rdb

	// ── Schema ────────────────────────────────────────────────────────────
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserProfile{},
		&model.UserAddress{},
		&model.AuthToken{},
	); err != nil {
		fmt.Printf("FAIL: automigrate: %v\n", err)
		os.Exit(1)
	}

	// ── In-memory RSA key pair (no file I/O needed) ───────────────────────
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		fmt.Printf("FAIL: generate RSA key: %v\n", err)
		os.Exit(1)
	}
	pubKey := &privKey.PublicKey

	// ── Full production stack ─────────────────────────────────────────────
	userRepo := repository.NewUserRepository(db)
	authTokenRepo := repository.NewAuthTokenRepository(db)
	bl := blacklist.New(rdb)
	sc := session.New(rdb)
	ac := loginattempt.New(rdb)
	authSvc := service.NewAuthService(userRepo, authTokenRepo, db, bl, sc, ac, privKey, pubKey)
	authHandler := handler.NewAuthHandler(authSvc)
	authMw := middleware.Auth(pubKey, bl)

	r := gin.New()
	r.Use(middleware.Recovery())

	v1 := r.Group("/api/v1")

	// Public auth routes
	auth := v1.Group("/auth")
	auth.POST("/register", authHandler.Register)
	auth.POST("/login", authHandler.Login)
	auth.POST("/refresh", authHandler.Refresh)

	// Protected auth routes
	protectedAuth := v1.Group("/auth")
	protectedAuth.Use(authMw)
	protectedAuth.POST("/logout", authHandler.Logout)

	// Test-only protected probe — returns context values injected by middleware
	v1.GET("/test/ping", authMw, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"userID": c.GetString(middleware.CtxUserID),
			"role":   c.GetString(middleware.CtxRole),
		})
	})

	testServer = httptest.NewServer(r)

	// ── Run ───────────────────────────────────────────────────────────────
	code := m.Run()

	// ── Teardown ──────────────────────────────────────────────────────────
	testServer.Close()
	_ = rdb.Close()
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}

	os.Exit(code)
}

// ─── HTTP helpers ─────────────────────────────────────────────────────────────

func doPost(t *testing.T, path string, body any, bearer string) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(http.MethodPost, testServer.URL+path, r)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func doGet(t *testing.T, path string, bearer string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, testServer.URL+path, nil)
	require.NoError(t, err)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func parseBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

// dig traverses a nested map[string]any and returns the string at the final key.
func dig(m map[string]any, keys ...string) string {
	cur := m
	for i, k := range keys {
		if i == len(keys)-1 {
			v, _ := cur[k].(string)
			return v
		}
		next, _ := cur[k].(map[string]any)
		if next == nil {
			return ""
		}
		cur = next
	}
	return ""
}

// errCode returns the error.code string from a response body.
func errCode(body map[string]any) string {
	return dig(body, "error", "code")
}

// uniqueEmail returns a test email that won't collide across parallel runs.
func uniqueEmail(prefix string) string {
	return fmt.Sprintf("integration_%s_%d@example.com", prefix, time.Now().UnixNano())
}

// cleanupUser removes all DB rows and Redis keys created for a test user.
func cleanupUser(email, userID string) {
	ctx := context.Background()
	testDB.WithContext(ctx).Exec(
		"DELETE FROM auth_tokens WHERE user_id IN (SELECT id FROM users WHERE email = ?)", email)
	testDB.WithContext(ctx).Exec(
		"DELETE FROM user_profiles WHERE user_id IN (SELECT id FROM users WHERE email = ?)", email)
	testDB.WithContext(ctx).Exec("DELETE FROM users WHERE email = ?", email)
	if userID != "" {
		testRDB.Del(ctx, fmt.Sprintf("session:%s", userID))
	}
	testRDB.Del(ctx, fmt.Sprintf("login_attempts:%s", email))
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestFullAuthFlow exercises the complete auth lifecycle with a real server stack:
//
//	Register → Login → protected route → Refresh → Logout → same token rejected
//
// Also directly checks Redis to confirm session cache and blacklist behaviour.
func TestFullAuthFlow(t *testing.T) {
	email := uniqueEmail("flow")
	const pw = "Test1234!"

	// ── Step 1: Register ──────────────────────────────────────────────────
	resp := doPost(t, "/api/v1/auth/register", map[string]any{
		"email": email, "password": pw,
		"first_name": "Flow", "last_name": "Test",
	}, "")
	body := parseBody(t, resp)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "register: %v", body)
	assert.Equal(t, true, body["success"])
	data, _ := body["data"].(map[string]any)
	assert.Equal(t, email, data["email"])

	// ── Step 2: Login ─────────────────────────────────────────────────────
	resp = doPost(t, "/api/v1/auth/login", map[string]any{
		"email": email, "password": pw,
	}, "")
	body = parseBody(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, "login: %v", body)

	accessToken := dig(body, "data", "access_token")
	refreshToken := dig(body, "data", "refresh_token")
	userID := dig(body, "data", "user", "id")
	require.NotEmpty(t, accessToken, "access_token must be present")
	require.NotEmpty(t, refreshToken, "refresh_token must be present")
	require.NotEmpty(t, userID, "user.id must be present")

	// Profile names must be populated (regression: Preload("Profile") on FindByEmailForUpdate)
	assert.Equal(t, "Flow", dig(body, "data", "user", "first_name"))
	assert.Equal(t, "Test", dig(body, "data", "user", "last_name"))

	t.Cleanup(func() { cleanupUser(email, userID) })

	// ── Step 3: Access protected route with valid token ───────────────────
	resp = doGet(t, "/api/v1/test/ping", accessToken)
	body = parseBody(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, "ping with valid token: %v", body)
	assert.Equal(t, userID, body["userID"], "middleware must inject userID into context")
	assert.Equal(t, "customer", body["role"], "middleware must inject role into context")

	// ── Step 4: Redis session cache set after login ───────────────────────
	sessionKey := fmt.Sprintf("session:%s", userID)
	sessionVal, err := testRDB.Get(context.Background(), sessionKey).Result()
	require.NoError(t, err, "session cache key must exist after login")
	assert.Contains(t, sessionVal, email, "session cache must contain user email")

	// ── Step 5: Refresh token issues a new access token ───────────────────
	resp = doPost(t, "/api/v1/auth/refresh", map[string]any{
		"refresh_token": refreshToken,
	}, "")
	body = parseBody(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, "refresh: %v", body)
	newAccessToken := dig(body, "data", "access_token")
	require.NotEmpty(t, newAccessToken)
	assert.NotEqual(t, accessToken, newAccessToken, "refreshed token must differ from original")

	// ── Step 6: Logout ────────────────────────────────────────────────────
	resp = doPost(t, "/api/v1/auth/logout", nil, accessToken)
	body = parseBody(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, "logout: %v", body)
	assert.Equal(t, "logged out", dig(body, "data", "message"))

	// ── Step 7: Same token rejected (Redis blacklist) ─────────────────────
	resp = doPost(t, "/api/v1/auth/logout", nil, accessToken)
	body = parseBody(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"blacklisted token must be rejected: %v", body)
	assert.Equal(t, "UNAUTHORIZED", errCode(body))

	// ── Step 8: Redis session cache cleared after logout ──────────────────
	_, err = testRDB.Get(context.Background(), sessionKey).Result()
	assert.True(t, errors.Is(err, redis.Nil),
		"session cache key must be absent after logout")
}

// TestAttemptCounter_BlocksAfterFiveFailures verifies two-layer lockout:
// DB locks the account on the 5th bad password; Redis pre-check blocks the 6th.
func TestAttemptCounter_BlocksAfterFiveFailures(t *testing.T) {
	email := uniqueEmail("attempts")
	const pw = "Correct1234!"

	// Register
	resp := doPost(t, "/api/v1/auth/register", map[string]any{
		"email": email, "password": pw,
		"first_name": "Attempt", "last_name": "User",
	}, "")
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	body := parseBody(t, resp)
	userID := dig(body, "data", "id")
	t.Cleanup(func() { cleanupUser(email, userID) })

	wrong := map[string]any{"email": email, "password": "WRONGPASS!"}

	// Attempts 1–4: 401 INVALID_CREDENTIALS
	for i := 1; i <= 4; i++ {
		resp = doPost(t, "/api/v1/auth/login", wrong, "")
		body = parseBody(t, resp)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"attempt %d should be 401", i)
		assert.Equal(t, "INVALID_CREDENTIALS", errCode(body),
			"attempt %d wrong error code", i)
	}

	// Attempt 5: DB reaches the limit → 403 ACCOUNT_LOCKED, account permanently locked
	resp = doPost(t, "/api/v1/auth/login", wrong, "")
	body = parseBody(t, resp)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "5th attempt should lock: %v", body)
	assert.Equal(t, "ACCOUNT_LOCKED", errCode(body))

	// Attempt 6: Redis pre-check fires (count ≥ 5) → 403, no DB query
	resp = doPost(t, "/api/v1/auth/login", wrong, "")
	body = parseBody(t, resp)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "6th attempt blocked by Redis: %v", body)
	assert.Equal(t, "ACCOUNT_LOCKED", errCode(body))

	// Verify Redis counter value
	attemptKey := fmt.Sprintf("login_attempts:%s", email)
	count, err := testRDB.Get(context.Background(), attemptKey).Int64()
	require.NoError(t, err, "login_attempts key must exist in Redis")
	assert.GreaterOrEqual(t, count, int64(5), "Redis counter must be ≥ 5")
}

// TestJWTMiddleware_RejectsInvalidToken verifies garbage JWTs are rejected at the middleware.
func TestJWTMiddleware_RejectsInvalidToken(t *testing.T) {
	resp := doGet(t, "/api/v1/test/ping", "not.a.valid.jwt")
	body := parseBody(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, "UNAUTHORIZED", errCode(body))
}

// TestJWTMiddleware_RejectsMissingHeader verifies the middleware rejects requests with no Bearer header.
func TestJWTMiddleware_RejectsMissingHeader(t *testing.T) {
	resp := doGet(t, "/api/v1/test/ping", "")
	body := parseBody(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, "UNAUTHORIZED", errCode(body))
}
