package dto

type RegisterRequest struct {
	Email     string `json:"email"      validate:"required,email,max=255"`
	Password  string `json:"password"   validate:"required,min=8,max=72"`
	FirstName string `json:"first_name" validate:"required,min=1,max=100"`
	LastName  string `json:"last_name"  validate:"required,min=1,max=100"`
}

type UserResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}
