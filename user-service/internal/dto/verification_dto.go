package dto

type VerifyEmailRequest struct {
	Email string `json:"email" validate:"required,email"`
	Code  string `json:"code" validate:"required,len=6,numeric"`
}

type ResendVerificationRequest struct {
	Email string `json:"email" validate:"required,email"`
}
