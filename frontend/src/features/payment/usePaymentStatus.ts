import { useQuery } from '@tanstack/react-query'
import { paymentApi } from './paymentApi'
import type { PaymentStatus } from '@/types/payment'

const TERMINAL: PaymentStatus[] = ['COMPLETED', 'FAILED']

export function usePaymentStatus(orderId: string) {
  return useQuery({
    queryKey: ['payment', 'order', orderId],
    queryFn: () => paymentApi.getByOrderId(orderId),
    enabled: !!orderId,
    refetchInterval: (query) => {
      if (query.state.status === 'error') return false
      const status = query.state.data?.data?.status
      return status && TERMINAL.includes(status) ? false : 2000
    },
    refetchIntervalInBackground: true,
  })
}
