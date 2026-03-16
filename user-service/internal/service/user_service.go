package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/model"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/repository"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/session"
)

var (
	ErrAddressNotFound = errors.New("address not found")
	ErrAddressForbidden = errors.New("address does not belong to this user")
)

type UserService interface {
	GetProfile(ctx context.Context, userID uuid.UUID) (*dto.ProfileResponse, error)
	UpdateProfile(ctx context.Context, userID uuid.UUID, req dto.UpdateProfileRequest) (*dto.ProfileResponse, error)
	AddAddress(ctx context.Context, userID uuid.UUID, req dto.CreateAddressRequest) (*dto.AddressResponse, error)
	UpdateAddress(ctx context.Context, userID, addressID uuid.UUID, req dto.UpdateAddressRequest) (*dto.AddressResponse, error)
	DeleteAddress(ctx context.Context, userID, addressID uuid.UUID) error
	SetDefaultAddress(ctx context.Context, userID, addressID uuid.UUID) (*dto.AddressResponse, error)
}

type userService struct {
	userRepo     repository.UserRepository
	addrRepo     repository.AddressRepository
	sessionCache session.Cache
}

func NewUserService(userRepo repository.UserRepository, addrRepo repository.AddressRepository, sessionCache session.Cache) UserService {
	return &userService{userRepo: userRepo, addrRepo: addrRepo, sessionCache: sessionCache}
}

func (s *userService) GetProfile(ctx context.Context, userID uuid.UUID) (*dto.ProfileResponse, error) {
	user, err := s.userRepo.FindByIDWithProfile(ctx, userID)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return toProfileResponse(user), nil
}

func (s *userService) UpdateProfile(ctx context.Context, userID uuid.UUID, req dto.UpdateProfileRequest) (*dto.ProfileResponse, error) {
	if err := s.userRepo.UpdateProfile(ctx, userID, req.FirstName, req.LastName, req.Phone); err != nil {
		return nil, err
	}

	// Invalidate session cache so subsequent refreshes re-read updated data from DB.
	_ = s.sessionCache.Delete(ctx, userID)

	user, err := s.userRepo.FindByIDWithProfile(ctx, userID)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return toProfileResponse(user), nil
}

func (s *userService) AddAddress(ctx context.Context, userID uuid.UUID, req dto.CreateAddressRequest) (*dto.AddressResponse, error) {
	addr := &model.UserAddress{
		UserID:       userID,
		Label:        req.Label,
		AddressLine1: req.AddressLine1,
		AddressLine2: req.AddressLine2,
		City:         req.City,
		State:        req.State,
		Country:      req.Country,
		PostalCode:   req.PostalCode,
	}
	if err := s.addrRepo.Create(ctx, addr); err != nil {
		return nil, err
	}
	resp := toAddressResponse(addr)
	return &resp, nil
}

func (s *userService) UpdateAddress(ctx context.Context, userID, addressID uuid.UUID, req dto.UpdateAddressRequest) (*dto.AddressResponse, error) {
	addr, err := s.addrRepo.FindByID(ctx, addressID)
	if err != nil {
		if errors.Is(err, repository.ErrAddressNotFound) {
			return nil, ErrAddressNotFound
		}
		return nil, err
	}
	if addr.UserID != userID {
		return nil, ErrAddressForbidden
	}

	addr.Label = req.Label
	addr.AddressLine1 = req.AddressLine1
	addr.AddressLine2 = req.AddressLine2
	addr.City = req.City
	addr.State = req.State
	addr.Country = req.Country
	addr.PostalCode = req.PostalCode

	if err := s.addrRepo.Update(ctx, addr); err != nil {
		return nil, err
	}
	resp := toAddressResponse(addr)
	return &resp, nil
}

func (s *userService) DeleteAddress(ctx context.Context, userID, addressID uuid.UUID) error {
	addr, err := s.addrRepo.FindByID(ctx, addressID)
	if err != nil {
		if errors.Is(err, repository.ErrAddressNotFound) {
			return ErrAddressNotFound
		}
		return err
	}
	if addr.UserID != userID {
		return ErrAddressForbidden
	}
	return s.addrRepo.Delete(ctx, addressID)
}

func (s *userService) SetDefaultAddress(ctx context.Context, userID, addressID uuid.UUID) (*dto.AddressResponse, error) {
	addr, err := s.addrRepo.FindByID(ctx, addressID)
	if err != nil {
		if errors.Is(err, repository.ErrAddressNotFound) {
			return nil, ErrAddressNotFound
		}
		return nil, err
	}
	if addr.UserID != userID {
		return nil, ErrAddressForbidden
	}
	if err := s.addrRepo.SetDefault(ctx, userID, addressID); err != nil {
		return nil, err
	}
	addr.IsDefault = true
	resp := toAddressResponse(addr)
	return &resp, nil
}

func toProfileResponse(u *model.User) *dto.ProfileResponse {
	p := &dto.ProfileResponse{
		ID:        u.ID.String(),
		Email:     u.Email,
		Role:      u.Role,
		Addresses: []dto.AddressResponse{},
	}
	if u.Profile != nil {
		p.FirstName = u.Profile.FirstName
		p.LastName = u.Profile.LastName
		p.Phone = u.Profile.Phone
		p.AvatarURL = u.Profile.AvatarURL
	}
	for _, a := range u.Addresses {
		p.Addresses = append(p.Addresses, toAddressResponse(&a))
	}
	return p
}

func toAddressResponse(a *model.UserAddress) dto.AddressResponse {
	return dto.AddressResponse{
		ID:           a.ID.String(),
		Label:        a.Label,
		AddressLine1: a.AddressLine1,
		AddressLine2: a.AddressLine2,
		City:         a.City,
		State:        a.State,
		Country:      a.Country,
		PostalCode:   a.PostalCode,
		IsDefault:    a.IsDefault,
	}
}
