package dto

type UpdateProfileRequest struct {
	FirstName string `json:"first_name" validate:"required,min=1,max=100"`
	LastName  string `json:"last_name"  validate:"required,min=1,max=100"`
	Phone     string `json:"phone"      validate:"omitempty,max=20"`
}

type AddressResponse struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	AddressLine1 string `json:"address_line1"`
	AddressLine2 string `json:"address_line2"`
	City         string `json:"city"`
	State        string `json:"state"`
	Country      string `json:"country"`
	PostalCode   string `json:"postal_code"`
	IsDefault    bool   `json:"is_default"`
}

type CreateAddressRequest struct {
	Label        string `json:"label"         validate:"omitempty,max=50"`
	AddressLine1 string `json:"address_line1" validate:"required,max=255"`
	AddressLine2 string `json:"address_line2" validate:"omitempty,max=255"`
	City         string `json:"city"          validate:"required,max=100"`
	State        string `json:"state"         validate:"omitempty,max=100"`
	Country      string `json:"country"       validate:"required,max=100"`
	PostalCode   string `json:"postal_code"   validate:"omitempty,max=20"`
}

type UpdateAddressRequest struct {
	Label        string `json:"label"         validate:"omitempty,max=50"`
	AddressLine1 string `json:"address_line1" validate:"required,max=255"`
	AddressLine2 string `json:"address_line2" validate:"omitempty,max=255"`
	City         string `json:"city"          validate:"required,max=100"`
	State        string `json:"state"         validate:"omitempty,max=100"`
	Country      string `json:"country"       validate:"required,max=100"`
	PostalCode   string `json:"postal_code"   validate:"omitempty,max=20"`
}

type ProfileResponse struct {
	ID        string            `json:"id"`
	Email     string            `json:"email"`
	Role      string            `json:"role"`
	FirstName string            `json:"first_name"`
	LastName  string            `json:"last_name"`
	Phone     string            `json:"phone"`
	AvatarURL string            `json:"avatar_url"`
	Addresses []AddressResponse `json:"addresses"`
}
