export interface CartItem {
  product_id: number
  product_name: string
  quantity: number
  unit_price: number
  subtotal: number
}

export interface Cart {
  user_id: string
  status: 'ACTIVE' | 'CHECKED_OUT'
  items: CartItem[]
  total: number
  updated_at: string
}

export interface AddCartItemRequest {
  product_id: number
  quantity: number
}
