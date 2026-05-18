import { api } from '@/lib/axios'
import type { ApiResponse } from '@/types/api'
import type { SellerProfile } from '@/types/sellerProfile'

export const sellerProfileApi = {
  getById: (id: string) =>
    api.get<ApiResponse<SellerProfile>>(`/users/sellers/${id}`).then((r) => r.data),
}
