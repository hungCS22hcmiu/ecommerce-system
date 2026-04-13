package dto

type AddItemRequest struct {
	ProductID int64 `json:"product_id" validate:"required,min=1"`
	Quantity  int   `json:"quantity"   validate:"required,min=1,max=999"`
}

type UpdateItemRequest struct {
	Quantity int `json:"quantity" validate:"required,min=1,max=999"`
}

type CartItemResponse struct {
	ProductID   int64   `json:"product_id"`
	ProductName string  `json:"product_name"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	Subtotal    float64 `json:"subtotal"`
}

type CartResponse struct {
	UserID    string             `json:"user_id"`
	Status    string             `json:"status"`
	Items     []CartItemResponse `json:"items"`
	Total     float64            `json:"total"`
	UpdatedAt string             `json:"updated_at"`
}
