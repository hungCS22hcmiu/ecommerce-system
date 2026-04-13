package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/client"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/repository"
)

var (
	ErrProductNotFound           = errors.New("product not found")
	ErrProductServiceUnavailable = errors.New("product service unavailable")
	ErrItemNotInCart             = errors.New("item not in cart")
	ErrConcurrentUpdate          = errors.New("concurrent cart update, please retry")
)

type CartService interface {
	GetCart(ctx context.Context, userID uuid.UUID) (*dto.CartResponse, error)
	AddItem(ctx context.Context, userID uuid.UUID, req dto.AddItemRequest) (*dto.CartResponse, error)
	UpdateItem(ctx context.Context, userID uuid.UUID, productID int64, req dto.UpdateItemRequest) (*dto.CartResponse, error)
	RemoveItem(ctx context.Context, userID uuid.UUID, productID int64) error
	ClearCart(ctx context.Context, userID uuid.UUID) error
}

type cartService struct {
	redisRepo     repository.RedisCartRepository
	cartRepo      repository.CartRepository
	productClient client.ProductClient
}

func NewCartService(redisRepo repository.RedisCartRepository, cartRepo repository.CartRepository, productClient client.ProductClient) CartService {
	return &cartService{
		redisRepo:     redisRepo,
		cartRepo:      cartRepo,
		productClient: productClient,
	}
}

func (s *cartService) GetCart(ctx context.Context, userID uuid.UUID) (*dto.CartResponse, error) {
	items, err := s.redisRepo.GetCart(ctx, userID)
	if err != nil {
		return nil, err
	}

	var total float64
	itemResponses := make([]dto.CartItemResponse, 0, len(items))
	for productID, val := range items {
		subtotal := val.UnitPrice * float64(val.Quantity)
		total += subtotal
		itemResponses = append(itemResponses, dto.CartItemResponse{
			ProductID:   productID,
			ProductName: val.ProductName,
			Quantity:    val.Quantity,
			UnitPrice:   val.UnitPrice,
			Subtotal:    subtotal,
		})
	}

	return &dto.CartResponse{
		UserID:    userID.String(),
		Status:    "ACTIVE",
		Items:     itemResponses,
		Total:     total,
		UpdatedAt: time.Now().Format(time.RFC3339),
	}, nil
}

func (s *cartService) AddItem(ctx context.Context, userID uuid.UUID, req dto.AddItemRequest) (*dto.CartResponse, error) {
	product, err := s.productClient.GetProduct(ctx, req.ProductID)
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			return nil, ErrProductNotFound
		}
		if errors.Is(err, client.ErrServiceUnavailable) {
			return nil, ErrProductServiceUnavailable
		}
		return nil, err
	}

	err = s.redisRepo.AddOrUpdateItem(ctx, userID, req.ProductID, repository.CartItemValue{
		ProductName: product.Name,
		Quantity:    req.Quantity,
		UnitPrice:   product.Price,
	})

	if err != nil {
		if errors.Is(err, repository.ErrConcurrentUpdate) {
			return nil, ErrConcurrentUpdate
		}
		return nil, err
	}

	return s.GetCart(ctx, userID)
}

func (s *cartService) UpdateItem(ctx context.Context, userID uuid.UUID, productID int64, req dto.UpdateItemRequest) (*dto.CartResponse, error) {
	cart, err := s.redisRepo.GetCart(ctx, userID)
	if err != nil {
		return nil, err
	}

	item, ok := cart[productID]
	if !ok {
		return nil, ErrItemNotInCart
	}

	item.Quantity = req.Quantity
	err = s.redisRepo.AddOrUpdateItem(ctx, userID, productID, item)
	if err != nil {
		if errors.Is(err, repository.ErrConcurrentUpdate) {
			return nil, ErrConcurrentUpdate
		}
		return nil, err
	}

	return s.GetCart(ctx, userID)
}

func (s *cartService) RemoveItem(ctx context.Context, userID uuid.UUID, productID int64) error {
	return s.redisRepo.RemoveItem(ctx, userID, productID)
}

func (s *cartService) ClearCart(ctx context.Context, userID uuid.UUID) error {
	// First clear Redis
	if err := s.redisRepo.ClearCart(ctx, userID); err != nil {
		return err
	}

	// Also clear Postgres as per Phase 2: "On ClearCart: delete both Redis key and Postgres rows immediately"
	return s.cartRepo.ClearCart(ctx, userID)
}
