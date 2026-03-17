package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/model"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/repository"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/service"
)

// ─── Mock address repository ──────────────────────────────────────────────────

type mockAddressRepo struct {
	mock.Mock
}

func (m *mockAddressRepo) Create(ctx context.Context, addr *model.UserAddress) error {
	args := m.Called(ctx, addr)
	return args.Error(0)
}

func (m *mockAddressRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.UserAddress, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.UserAddress), args.Error(1)
}

func (m *mockAddressRepo) Update(ctx context.Context, addr *model.UserAddress) error {
	args := m.Called(ctx, addr)
	return args.Error(0)
}

func (m *mockAddressRepo) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockAddressRepo) SetDefault(ctx context.Context, userID, addressID uuid.UUID) error {
	args := m.Called(ctx, userID, addressID)
	return args.Error(0)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func userWithProfile(id uuid.UUID) *model.User {
	return &model.User{
		ID:    id,
		Email: "alice@example.com",
		Role:  "customer",
		Profile: &model.UserProfile{
			UserID:    id,
			FirstName: "Alice",
			LastName:  "Smith",
			Phone:     "0900000000",
		},
		Addresses: []model.UserAddress{
			{
				ID:           uuid.New(),
				UserID:       id,
				Label:        "home",
				AddressLine1: "123 Main St",
				City:         "Ho Chi Minh",
				Country:      "Vietnam",
				IsDefault:    true,
			},
		},
	}
}

func newUserSvc(userRepo *mockUserRepo, addrRepo *mockAddressRepo, sess *mockSessionCache) service.UserService {
	return service.NewUserService(userRepo, addrRepo, sess)
}

// ─── GetUser tests ────────────────────────────────────────────────────────────

func TestGetUser_Success(t *testing.T) {
	userID := uuid.New()
	user := userWithProfile(userID)

	repo := &mockUserRepo{}
	repo.On("FindByIDWithProfile", mock.Anything, userID).Return(user, nil)

	svc := newUserSvc(repo, &mockAddressRepo{}, &mockSessionCache{})
	resp, err := svc.GetUser(context.Background(), userID)

	require.NoError(t, err)
	assert.Equal(t, userID.String(), resp.ID)
	assert.Equal(t, "alice@example.com", resp.Email)
	assert.Equal(t, "customer", resp.Role)
	assert.Equal(t, "Alice", resp.FirstName)
	assert.Equal(t, "Smith", resp.LastName)
	repo.AssertExpectations(t)
}

func TestGetUser_NotFound(t *testing.T) {
	userID := uuid.New()

	repo := &mockUserRepo{}
	repo.On("FindByIDWithProfile", mock.Anything, userID).Return(nil, repository.ErrNotFound)

	svc := newUserSvc(repo, &mockAddressRepo{}, &mockSessionCache{})
	_, err := svc.GetUser(context.Background(), userID)

	assert.ErrorIs(t, err, service.ErrUserNotFound)
	repo.AssertExpectations(t)
}

// ─── GetProfile tests ─────────────────────────────────────────────────────────

func TestGetProfile_Success(t *testing.T) {
	userID := uuid.New()
	user := userWithProfile(userID)

	repo := &mockUserRepo{}
	sess := &mockSessionCache{}
	repo.On("FindByIDWithProfile", mock.Anything, userID).Return(user, nil)

	svc := newUserSvc(repo, &mockAddressRepo{}, sess)
	profile, err := svc.GetProfile(context.Background(), userID)

	require.NoError(t, err)
	assert.Equal(t, userID.String(), profile.ID)
	assert.Equal(t, "alice@example.com", profile.Email)
	assert.Equal(t, "Alice", profile.FirstName)
	assert.Equal(t, "Smith", profile.LastName)
	assert.Equal(t, "0900000000", profile.Phone)
	assert.Len(t, profile.Addresses, 1)
	assert.Equal(t, "home", profile.Addresses[0].Label)
	repo.AssertExpectations(t)
}

func TestGetProfile_NotFound(t *testing.T) {
	userID := uuid.New()

	repo := &mockUserRepo{}
	sess := &mockSessionCache{}
	repo.On("FindByIDWithProfile", mock.Anything, userID).Return(nil, repository.ErrNotFound)

	svc := newUserSvc(repo, &mockAddressRepo{}, sess)
	_, err := svc.GetProfile(context.Background(), userID)

	assert.ErrorIs(t, err, service.ErrUserNotFound)
	repo.AssertExpectations(t)
}

// ─── UpdateProfile tests ──────────────────────────────────────────────────────

func TestUpdateProfile_Success(t *testing.T) {
	userID := uuid.New()
	updatedUser := userWithProfile(userID)
	updatedUser.Profile.FirstName = "Jane"
	updatedUser.Profile.LastName = "Doe"
	updatedUser.Profile.Phone = "0912345678"

	repo := &mockUserRepo{}
	sess := &mockSessionCache{}
	repo.On("UpdateProfile", mock.Anything, userID, "Jane", "Doe", "0912345678").Return(nil)
	sess.On("Delete", mock.Anything, userID).Return(nil)
	repo.On("FindByIDWithProfile", mock.Anything, userID).Return(updatedUser, nil)

	svc := newUserSvc(repo, &mockAddressRepo{}, sess)
	req := dto.UpdateProfileRequest{FirstName: "Jane", LastName: "Doe", Phone: "0912345678"}
	profile, err := svc.UpdateProfile(context.Background(), userID, req)

	require.NoError(t, err)
	assert.Equal(t, "Jane", profile.FirstName)
	assert.Equal(t, "Doe", profile.LastName)
	assert.Equal(t, "0912345678", profile.Phone)
	repo.AssertExpectations(t)
	sess.AssertExpectations(t)
}

func TestUpdateProfile_UserNotFoundAfterUpdate(t *testing.T) {
	userID := uuid.New()

	repo := &mockUserRepo{}
	sess := &mockSessionCache{}
	repo.On("UpdateProfile", mock.Anything, userID, "Jane", "Doe", "").Return(nil)
	sess.On("Delete", mock.Anything, userID).Return(nil)
	repo.On("FindByIDWithProfile", mock.Anything, userID).Return(nil, repository.ErrNotFound)

	svc := newUserSvc(repo, &mockAddressRepo{}, sess)
	req := dto.UpdateProfileRequest{FirstName: "Jane", LastName: "Doe"}
	_, err := svc.UpdateProfile(context.Background(), userID, req)

	assert.ErrorIs(t, err, service.ErrUserNotFound)
	repo.AssertExpectations(t)
}

// ─── AddAddress tests ─────────────────────────────────────────────────────────

func TestAddAddress_Success(t *testing.T) {
	userID := uuid.New()

	addrRepo := &mockAddressRepo{}
	addrRepo.On("Create", mock.Anything, mock.MatchedBy(func(a *model.UserAddress) bool {
		return a.UserID == userID && a.AddressLine1 == "123 Main St" && a.City == "Ho Chi Minh"
	})).Return(nil)

	svc := newUserSvc(&mockUserRepo{}, addrRepo, &mockSessionCache{})
	req := dto.CreateAddressRequest{
		Label:        "home",
		AddressLine1: "123 Main St",
		City:         "Ho Chi Minh",
		Country:      "Vietnam",
	}
	addr, err := svc.AddAddress(context.Background(), userID, req)

	require.NoError(t, err)
	assert.Equal(t, "123 Main St", addr.AddressLine1)
	assert.Equal(t, "home", addr.Label)
	addrRepo.AssertExpectations(t)
}

// ─── UpdateAddress tests ──────────────────────────────────────────────────────

func TestUpdateAddress_Success(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	existing := &model.UserAddress{
		ID:           addrID,
		UserID:       userID,
		AddressLine1: "Old St",
		City:         "Old City",
		Country:      "Vietnam",
	}

	addrRepo := &mockAddressRepo{}
	addrRepo.On("FindByID", mock.Anything, addrID).Return(existing, nil)
	addrRepo.On("Update", mock.Anything, mock.MatchedBy(func(a *model.UserAddress) bool {
		return a.AddressLine1 == "New St" && a.City == "New City"
	})).Return(nil)

	svc := newUserSvc(&mockUserRepo{}, addrRepo, &mockSessionCache{})
	req := dto.UpdateAddressRequest{AddressLine1: "New St", City: "New City", Country: "Vietnam"}
	addr, err := svc.UpdateAddress(context.Background(), userID, addrID, req)

	require.NoError(t, err)
	assert.Equal(t, "New St", addr.AddressLine1)
	addrRepo.AssertExpectations(t)
}

func TestUpdateAddress_NotFound(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()

	addrRepo := &mockAddressRepo{}
	addrRepo.On("FindByID", mock.Anything, addrID).Return(nil, repository.ErrAddressNotFound)

	svc := newUserSvc(&mockUserRepo{}, addrRepo, &mockSessionCache{})
	_, err := svc.UpdateAddress(context.Background(), userID, addrID, dto.UpdateAddressRequest{
		AddressLine1: "X", City: "Y", Country: "Z",
	})

	assert.ErrorIs(t, err, service.ErrAddressNotFound)
}

func TestUpdateAddress_Forbidden(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	otherUserID := uuid.New()
	existing := &model.UserAddress{ID: addrID, UserID: otherUserID}

	addrRepo := &mockAddressRepo{}
	addrRepo.On("FindByID", mock.Anything, addrID).Return(existing, nil)

	svc := newUserSvc(&mockUserRepo{}, addrRepo, &mockSessionCache{})
	_, err := svc.UpdateAddress(context.Background(), userID, addrID, dto.UpdateAddressRequest{
		AddressLine1: "X", City: "Y", Country: "Z",
	})

	assert.ErrorIs(t, err, service.ErrAddressForbidden)
}

// ─── DeleteAddress tests ──────────────────────────────────────────────────────

func TestDeleteAddress_Success(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	existing := &model.UserAddress{ID: addrID, UserID: userID}

	addrRepo := &mockAddressRepo{}
	addrRepo.On("FindByID", mock.Anything, addrID).Return(existing, nil)
	addrRepo.On("Delete", mock.Anything, addrID).Return(nil)

	svc := newUserSvc(&mockUserRepo{}, addrRepo, &mockSessionCache{})
	err := svc.DeleteAddress(context.Background(), userID, addrID)

	require.NoError(t, err)
	addrRepo.AssertExpectations(t)
}

func TestDeleteAddress_Forbidden(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	existing := &model.UserAddress{ID: addrID, UserID: uuid.New()}

	addrRepo := &mockAddressRepo{}
	addrRepo.On("FindByID", mock.Anything, addrID).Return(existing, nil)

	svc := newUserSvc(&mockUserRepo{}, addrRepo, &mockSessionCache{})
	err := svc.DeleteAddress(context.Background(), userID, addrID)

	assert.ErrorIs(t, err, service.ErrAddressForbidden)
}

// ─── SetDefaultAddress tests ──────────────────────────────────────────────────

func TestSetDefaultAddress_Success(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	existing := &model.UserAddress{ID: addrID, UserID: userID, AddressLine1: "123 St", City: "HCMC", Country: "VN"}

	addrRepo := &mockAddressRepo{}
	addrRepo.On("FindByID", mock.Anything, addrID).Return(existing, nil)
	addrRepo.On("SetDefault", mock.Anything, userID, addrID).Return(nil)

	svc := newUserSvc(&mockUserRepo{}, addrRepo, &mockSessionCache{})
	addr, err := svc.SetDefaultAddress(context.Background(), userID, addrID)

	require.NoError(t, err)
	assert.True(t, addr.IsDefault)
	addrRepo.AssertExpectations(t)
}

func TestSetDefaultAddress_Forbidden(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	existing := &model.UserAddress{ID: addrID, UserID: uuid.New()}

	addrRepo := &mockAddressRepo{}
	addrRepo.On("FindByID", mock.Anything, addrID).Return(existing, nil)

	svc := newUserSvc(&mockUserRepo{}, addrRepo, &mockSessionCache{})
	_, err := svc.SetDefaultAddress(context.Background(), userID, addrID)

	assert.ErrorIs(t, err, service.ErrAddressForbidden)
}
