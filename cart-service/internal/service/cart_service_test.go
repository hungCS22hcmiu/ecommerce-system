package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/client"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/model"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/repository"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/service"
)

// ─── Mock: RedisCartRepository ────────────────────────────────────────────────

type mockRedisRepo struct{ mock.Mock }

func (m *mockRedisRepo) GetCart(ctx context.Context, userID uuid.UUID) (map[int64]repository.CartItemValue, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[int64]repository.CartItemValue), args.Error(1)
}

func (m *mockRedisRepo) AddOrUpdateItem(ctx context.Context, userID uuid.UUID, productID int64, val repository.CartItemValue) error {
	args := m.Called(ctx, userID, productID, val)
	return args.Error(0)
}

func (m *mockRedisRepo) RemoveItem(ctx context.Context, userID uuid.UUID, productID int64) error {
	args := m.Called(ctx, userID, productID)
	return args.Error(0)
}

func (m *mockRedisRepo) ClearCart(ctx context.Context, userID uuid.UUID) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

// ─── Mock: CartRepository ─────────────────────────────────────────────────────

type mockCartRepo struct{ mock.Mock }

func (m *mockCartRepo) UpsertCart(ctx context.Context, userID uuid.UUID) (*model.Cart, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Cart), args.Error(1)
}

func (m *mockCartRepo) ReplaceItems(ctx context.Context, cartID uuid.UUID, items []model.CartItem) error {
	args := m.Called(ctx, cartID, items)
	return args.Error(0)
}

func (m *mockCartRepo) GetCartWithItems(ctx context.Context, userID uuid.UUID) (*model.Cart, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Cart), args.Error(1)
}

func (m *mockCartRepo) MarkCheckedOut(ctx context.Context, cartID uuid.UUID) error {
	args := m.Called(ctx, cartID)
	return args.Error(0)
}

func (m *mockCartRepo) ClearCart(ctx context.Context, userID uuid.UUID) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

// ─── Mock: ProductClient ──────────────────────────────────────────────────────

type mockProductClient struct{ mock.Mock }

func (m *mockProductClient) GetProduct(ctx context.Context, productID int64) (*client.ProductInfo, error) {
	args := m.Called(ctx, productID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*client.ProductInfo), args.Error(1)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func newSvc(r *mockRedisRepo, c *mockCartRepo, p *mockProductClient) service.CartService {
	return service.NewCartService(r, c, p)
}

var ctx = context.Background()

// ─── GetCart ──────────────────────────────────────────────────────────────────

func TestGetCart_Empty(t *testing.T) {
	redisRepo := &mockRedisRepo{}
	cartRepo := &mockCartRepo{}
	productClient := &mockProductClient{}

	userID := uuid.New()
	redisRepo.On("GetCart", ctx, userID).Return(map[int64]repository.CartItemValue{}, nil)

	svc := newSvc(redisRepo, cartRepo, productClient)
	resp, err := svc.GetCart(ctx, userID)

	require.NoError(t, err)
	assert.NotNil(t, resp.Items)
	assert.Empty(t, resp.Items)
	assert.Equal(t, 0.0, resp.Total)
	assert.Equal(t, userID.String(), resp.UserID)
	redisRepo.AssertExpectations(t)
}

func TestGetCart_WithItems(t *testing.T) {
	redisRepo := &mockRedisRepo{}
	cartRepo := &mockCartRepo{}
	productClient := &mockProductClient{}

	userID := uuid.New()
	redisRepo.On("GetCart", ctx, userID).Return(map[int64]repository.CartItemValue{
		1: {ProductName: "Widget", Quantity: 2, UnitPrice: 9.99},
		2: {ProductName: "Gadget", Quantity: 1, UnitPrice: 24.99},
	}, nil)

	svc := newSvc(redisRepo, cartRepo, productClient)
	resp, err := svc.GetCart(ctx, userID)

	require.NoError(t, err)
	assert.Len(t, resp.Items, 2)
	assert.InDelta(t, 44.97, resp.Total, 0.01) // 2*9.99 + 1*24.99
	redisRepo.AssertExpectations(t)
}

// ─── AddItem ─────────────────────────────────────────────────────────────────

func TestAddItem_Success(t *testing.T) {
	redisRepo := &mockRedisRepo{}
	cartRepo := &mockCartRepo{}
	productClient := &mockProductClient{}

	userID := uuid.New()
	req := dto.AddItemRequest{ProductID: 1, Quantity: 2}

	productClient.On("GetProduct", ctx, int64(1)).Return(&client.ProductInfo{
		ID: 1, Name: "Widget", Price: 9.99, Status: "ACTIVE",
	}, nil)
	redisRepo.On("AddOrUpdateItem", ctx, userID, int64(1), repository.CartItemValue{
		ProductName: "Widget", Quantity: 2, UnitPrice: 9.99,
	}).Return(nil)
	redisRepo.On("GetCart", ctx, userID).Return(map[int64]repository.CartItemValue{
		1: {ProductName: "Widget", Quantity: 2, UnitPrice: 9.99},
	}, nil)

	svc := newSvc(redisRepo, cartRepo, productClient)
	resp, err := svc.AddItem(ctx, userID, req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Items, 1)
	assert.InDelta(t, 19.98, resp.Total, 0.01)
	redisRepo.AssertExpectations(t)
	productClient.AssertExpectations(t)
}

func TestAddItem_ProductNotFound(t *testing.T) {
	redisRepo := &mockRedisRepo{}
	cartRepo := &mockCartRepo{}
	productClient := &mockProductClient{}

	userID := uuid.New()
	req := dto.AddItemRequest{ProductID: 99, Quantity: 1}

	productClient.On("GetProduct", ctx, int64(99)).Return(nil, client.ErrNotFound)

	svc := newSvc(redisRepo, cartRepo, productClient)
	_, err := svc.AddItem(ctx, userID, req)

	require.Error(t, err)
	assert.True(t, errors.Is(err, service.ErrProductNotFound))
	productClient.AssertExpectations(t)
}

func TestAddItem_ServiceDown(t *testing.T) {
	redisRepo := &mockRedisRepo{}
	cartRepo := &mockCartRepo{}
	productClient := &mockProductClient{}

	userID := uuid.New()
	req := dto.AddItemRequest{ProductID: 1, Quantity: 1}

	productClient.On("GetProduct", ctx, int64(1)).Return(nil, client.ErrServiceUnavailable)

	svc := newSvc(redisRepo, cartRepo, productClient)
	_, err := svc.AddItem(ctx, userID, req)

	require.Error(t, err)
	assert.True(t, errors.Is(err, service.ErrProductServiceUnavailable))
	productClient.AssertExpectations(t)
}

func TestAddItem_ConcurrentUpdate(t *testing.T) {
	redisRepo := &mockRedisRepo{}
	cartRepo := &mockCartRepo{}
	productClient := &mockProductClient{}

	userID := uuid.New()
	req := dto.AddItemRequest{ProductID: 1, Quantity: 1}

	productClient.On("GetProduct", ctx, int64(1)).Return(&client.ProductInfo{
		ID: 1, Name: "Widget", Price: 9.99, Status: "ACTIVE",
	}, nil)
	redisRepo.On("AddOrUpdateItem", ctx, userID, int64(1), mock.AnythingOfType("repository.CartItemValue")).
		Return(repository.ErrConcurrentUpdate)

	svc := newSvc(redisRepo, cartRepo, productClient)
	_, err := svc.AddItem(ctx, userID, req)

	require.Error(t, err)
	assert.True(t, errors.Is(err, service.ErrConcurrentUpdate))
	redisRepo.AssertExpectations(t)
}

// ─── UpdateItem ───────────────────────────────────────────────────────────────

func TestUpdateItem_Success(t *testing.T) {
	redisRepo := &mockRedisRepo{}
	cartRepo := &mockCartRepo{}
	productClient := &mockProductClient{}

	userID := uuid.New()
	req := dto.UpdateItemRequest{Quantity: 5}

	redisRepo.On("GetCart", ctx, userID).Return(map[int64]repository.CartItemValue{
		1: {ProductName: "Widget", Quantity: 2, UnitPrice: 9.99},
	}, nil).Once()
	redisRepo.On("AddOrUpdateItem", ctx, userID, int64(1), repository.CartItemValue{
		ProductName: "Widget", Quantity: 5, UnitPrice: 9.99,
	}).Return(nil)
	// GetCart called again by service after update
	redisRepo.On("GetCart", ctx, userID).Return(map[int64]repository.CartItemValue{
		1: {ProductName: "Widget", Quantity: 5, UnitPrice: 9.99},
	}, nil).Once()

	svc := newSvc(redisRepo, cartRepo, productClient)
	resp, err := svc.UpdateItem(ctx, userID, 1, req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 5, resp.Items[0].Quantity)
	redisRepo.AssertExpectations(t)
}

func TestUpdateItem_NotInCart(t *testing.T) {
	redisRepo := &mockRedisRepo{}
	cartRepo := &mockCartRepo{}
	productClient := &mockProductClient{}

	userID := uuid.New()

	redisRepo.On("GetCart", ctx, userID).Return(map[int64]repository.CartItemValue{}, nil)

	svc := newSvc(redisRepo, cartRepo, productClient)
	_, err := svc.UpdateItem(ctx, userID, 99, dto.UpdateItemRequest{Quantity: 3})

	require.Error(t, err)
	assert.True(t, errors.Is(err, service.ErrItemNotInCart))
	redisRepo.AssertExpectations(t)
}

func TestUpdateItem_ConcurrentUpdate(t *testing.T) {
	redisRepo := &mockRedisRepo{}
	cartRepo := &mockCartRepo{}
	productClient := &mockProductClient{}

	userID := uuid.New()

	redisRepo.On("GetCart", ctx, userID).Return(map[int64]repository.CartItemValue{
		1: {ProductName: "Widget", Quantity: 2, UnitPrice: 9.99},
	}, nil)
	redisRepo.On("AddOrUpdateItem", ctx, userID, int64(1), mock.AnythingOfType("repository.CartItemValue")).
		Return(repository.ErrConcurrentUpdate)

	svc := newSvc(redisRepo, cartRepo, productClient)
	_, err := svc.UpdateItem(ctx, userID, 1, dto.UpdateItemRequest{Quantity: 5})

	require.Error(t, err)
	assert.True(t, errors.Is(err, service.ErrConcurrentUpdate))
	redisRepo.AssertExpectations(t)
}

// ─── RemoveItem ───────────────────────────────────────────────────────────────

func TestRemoveItem(t *testing.T) {
	redisRepo := &mockRedisRepo{}
	cartRepo := &mockCartRepo{}
	productClient := &mockProductClient{}

	userID := uuid.New()

	redisRepo.On("RemoveItem", ctx, userID, int64(1)).Return(nil)

	svc := newSvc(redisRepo, cartRepo, productClient)
	err := svc.RemoveItem(ctx, userID, 1)

	require.NoError(t, err)
	redisRepo.AssertExpectations(t)
}

// ─── ClearCart ────────────────────────────────────────────────────────────────

func TestClearCart(t *testing.T) {
	redisRepo := &mockRedisRepo{}
	cartRepo := &mockCartRepo{}
	productClient := &mockProductClient{}

	userID := uuid.New()

	redisRepo.On("ClearCart", ctx, userID).Return(nil)
	cartRepo.On("ClearCart", ctx, userID).Return(nil)

	svc := newSvc(redisRepo, cartRepo, productClient)
	err := svc.ClearCart(ctx, userID)

	require.NoError(t, err)
	redisRepo.AssertExpectations(t)
	cartRepo.AssertExpectations(t)
}
