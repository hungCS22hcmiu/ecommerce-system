export interface LoginRequest {
  email: string
  password: string
}

export interface RegisterRequest {
  email: string
  password: string
  first_name: string
  last_name: string
}

export interface AuthResponse {
  access_token: string
  refresh_token: string
  user: {
    id: string
    email: string
    role: string
    first_name: string
    last_name: string
  }
}

export interface Address {
  id: string
  label?: string
  address_line1: string
  address_line2?: string
  city: string
  state?: string
  country: string
  postal_code?: string
  is_default: boolean
}

export interface UserProfile {
  id: string
  email: string
  role: string
  first_name: string
  last_name: string
  phone?: string
  avatar_url?: string
  addresses: Address[]
}
