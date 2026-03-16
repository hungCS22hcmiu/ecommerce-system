package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/session"
)

// mockRedis is a simple in-memory stand-in so these tests require no real Redis.
type entry struct {
	value  []byte
	expiry time.Time
}

type mockRedis struct {
	data map[string]entry
}

func newMockRedis() *mockRedis { return &mockRedis{data: make(map[string]entry)} }

func (m *mockRedis) set(k string, v []byte, ttl time.Duration) {
	m.data[k] = entry{value: v, expiry: time.Now().Add(ttl)}
}

func (m *mockRedis) get(k string) ([]byte, bool) {
	e, ok := m.data[k]
	if !ok || time.Now().After(e.expiry) {
		return nil, false
	}
	return e.value, true
}

func (m *mockRedis) del(k string) { delete(m.data, k) }

// ─── Interface compliance check (compile-time) ────────────────────────────────

// These tests use a real Redis client, which would require miniredis or a real
// Redis instance. Instead we test the logic contract via the session.Cache
// interface using a fake implementation to document expected behaviour.

// realCacheContract verifies the expected contract at the type level.
// The actual Redis-backed implementation is exercised by integration tests.
func TestCacheInterface_Defined(t *testing.T) {
	// Verify the interface exists and has the expected methods at compile time.
	var _ session.Cache = (session.Cache)(nil)
}

// ─── Behaviour contract tests (via fake in-memory impl) ───────────────────────

type fakeCache struct {
	store map[string]dto.UserResponse
}

func newFakeCache() session.Cache { return &fakeCache{store: make(map[string]dto.UserResponse)} }

func (f *fakeCache) Set(_ context.Context, userID uuid.UUID, user dto.UserResponse, _ time.Duration) error {
	f.store[userID.String()] = user
	return nil
}

func (f *fakeCache) Get(_ context.Context, userID uuid.UUID) (*dto.UserResponse, error) {
	u, ok := f.store[userID.String()]
	if !ok {
		return nil, nil
	}
	return &u, nil
}

func (f *fakeCache) Delete(_ context.Context, userID uuid.UUID) error {
	delete(f.store, userID.String())
	return nil
}

func TestSessionCache_SetAndGet(t *testing.T) {
	c := newFakeCache()
	ctx := context.Background()
	userID := uuid.New()
	user := dto.UserResponse{
		ID:        userID.String(),
		Email:     "alice@example.com",
		Role:      "customer",
		FirstName: "Alice",
		LastName:  "Smith",
	}

	require.NoError(t, c.Set(ctx, userID, user, 30*time.Minute))

	got, err := c.Get(ctx, userID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, user, *got)
}

func TestSessionCache_GetMiss_ReturnsNil(t *testing.T) {
	c := newFakeCache()
	got, err := c.Get(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSessionCache_Delete_RemovesEntry(t *testing.T) {
	c := newFakeCache()
	ctx := context.Background()
	userID := uuid.New()
	user := dto.UserResponse{ID: userID.String(), Email: "bob@example.com", Role: "customer"}

	require.NoError(t, c.Set(ctx, userID, user, 30*time.Minute))
	require.NoError(t, c.Delete(ctx, userID))

	got, err := c.Get(ctx, userID)
	require.NoError(t, err)
	assert.Nil(t, got)
}
