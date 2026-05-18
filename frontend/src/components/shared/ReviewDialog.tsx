import { useEffect, useState } from 'react'
import { StarRating } from '@/components/ui/StarRating'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useMyReviewByOrderItem, useCreateReview, useUpdateReview } from '@/features/reviews/useReviews'
import { showToast } from '@/lib/toast'
import type { OrderItem } from '@/types/order'

interface ReviewDialogProps {
  items: OrderItem[]
  onClose: () => void
}

interface ItemState {
  rating: number
  comment: string
}

function ReviewItemRow({ item, state, onChange }: {
  item: OrderItem
  state: ItemState
  onChange: (s: ItemState) => void
}) {
  const { data: existingData, isLoading } = useMyReviewByOrderItem(item.productId, item.id)
  const existing = existingData?.data ?? null

  useEffect(() => {
    if (existing && state.rating === 0 && state.comment === '') {
      onChange({ rating: existing.rating, comment: existing.comment ?? '' })
    }
  }, [existing]) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="py-5 border-b border-surface-border last:border-0">
      <div className="flex gap-4">
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium text-fg-base truncate">{item.productName}</p>
          <p className="text-xs text-fg-subtle mt-0.5">Qty: {item.quantity}</p>
          {isLoading && <Skeleton className="h-3 w-32 mt-1" />}
          {existing && !isLoading && (
            <p className="text-xs text-fg-subtle mt-0.5">
              Reviewed on {new Date(existing.createdAt).toLocaleDateString()}
            </p>
          )}
        </div>
        <div className="flex flex-col items-end gap-2 flex-shrink-0">
          <StarRating value={state.rating} onChange={(r) => onChange({ ...state, rating: r })} size="sm" />
          <p className="text-xs text-fg-subtle">{state.rating > 0 ? `${state.rating} / 5` : 'No rating'}</p>
        </div>
      </div>
      <textarea
        value={state.comment}
        onChange={(e) => onChange({ ...state, comment: e.target.value })}
        placeholder="Write a comment (optional)..."
        rows={2}
        className="mt-3 w-full bg-surface-overlay border border-surface-border rounded-md px-3 py-2 text-sm text-fg-base placeholder:text-fg-subtle focus:outline-none focus:border-accent resize-none"
      />
    </div>
  )
}

export function ReviewDialog({ items, onClose }: ReviewDialogProps) {
  const [states, setStates] = useState<ItemState[]>(() =>
    items.map(() => ({ rating: 0, comment: '' }))
  )
  const [submitting, setSubmitting] = useState(false)

  const createReviews = items.map((item) => useCreateReview(item.productId)) // eslint-disable-line react-hooks/rules-of-hooks
  const updateReviews = items.map((item) => useUpdateReview(item.productId)) // eslint-disable-line react-hooks/rules-of-hooks
  const existingReviews = items.map((item) => useMyReviewByOrderItem(item.productId, item.id)) // eslint-disable-line react-hooks/rules-of-hooks

  function updateState(index: number, s: ItemState) {
    setStates((prev) => prev.map((v, i) => (i === index ? s : v)))
  }

  async function handleSubmit() {
    const toSubmit = items
      .map((item, i) => ({ item, state: states[i], existing: existingReviews[i].data?.data ?? null }))
      .filter(({ state }) => state.rating > 0)

    if (toSubmit.length === 0) {
      showToast('Select at least one rating to submit', 'error')
      return
    }

    setSubmitting(true)
    let successCount = 0
    let errorCount = 0

    await Promise.all(
      toSubmit.map(({ item, state, existing }) => {
        const itemIndex = items.indexOf(item)
        return new Promise<void>((resolve) => {
          if (existing) {
            updateReviews[itemIndex].mutate(
              { reviewId: existing.id, req: { rating: state.rating, comment: state.comment || undefined } },
              {
                onSuccess: () => { successCount++; resolve() },
                onError: () => { errorCount++; resolve() },
              }
            )
          } else {
            createReviews[itemIndex].mutate(
              { orderItemId: item.id, rating: state.rating, comment: state.comment || undefined },
              {
                onSuccess: () => { successCount++; resolve() },
                onError: (err: unknown) => {
                  const code = (err as { response?: { data?: { code?: string } } })?.response?.data?.code
                  if (code !== 'ALREADY_REVIEWED') errorCount++
                  else successCount++
                  resolve()
                },
              }
            )
          }
        })
      })
    )

    setSubmitting(false)
    if (errorCount === 0) {
      showToast(`${successCount} review${successCount !== 1 ? 's' : ''} saved`, 'success')
    } else {
      showToast(`${successCount} saved, ${errorCount} failed`, 'error')
    }
    onClose()
  }

  const ratedCount = states.filter((s) => s.rating > 0).length

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/60" onClick={onClose} />
      <div className="relative z-10 bg-surface-raised border border-surface-border rounded-xl shadow-xl w-full max-w-2xl mx-4 flex flex-col max-h-[85vh]">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-surface-border flex-shrink-0">
          <div>
            <h2 className="font-display text-lg text-fg-base">Rate Your Purchase</h2>
            <p className="text-xs text-fg-subtle mt-0.5">{items.length} item{items.length !== 1 ? 's' : ''} · rate as many as you like</p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="text-fg-subtle hover:text-fg-base transition-colors text-lg leading-none"
          >
            ✕
          </button>
        </div>

        {/* Scrollable items */}
        <div className="overflow-y-auto flex-1 px-6">
          {items.map((item, i) => (
            <ReviewItemRow
              key={item.id}
              item={item}
              state={states[i]}
              onChange={(s) => updateState(i, s)}
            />
          ))}
        </div>

        {/* Footer */}
        <div className="px-6 py-4 border-t border-surface-border flex items-center justify-between flex-shrink-0">
          <span className="text-xs text-fg-subtle">
            {ratedCount > 0 ? `${ratedCount} item${ratedCount !== 1 ? 's' : ''} rated` : 'No ratings yet'}
          </span>
          <div className="flex gap-3">
            <Button variant="outline" onClick={onClose} disabled={submitting}>
              Cancel
            </Button>
            <Button onClick={handleSubmit} disabled={submitting || ratedCount === 0}>
              {submitting ? 'Saving…' : `Submit ${ratedCount > 0 ? ratedCount : ''} Review${ratedCount !== 1 ? 's' : ''}`}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}
