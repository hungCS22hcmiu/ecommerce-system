import { useState } from 'react'
import { StarRating } from '@/components/ui/StarRating'
import { Button } from '@/components/ui/button'
import { useMyReviewByOrderItem, useCreateReview, useUpdateReview } from '@/features/reviews/useReviews'
import { showToast } from '@/lib/toast'
import type { OrderItem } from '@/types/order'

interface ReviewDialogProps {
  items: OrderItem[]
  onClose: () => void
}

function ReviewItemForm({
  item,
  onDone,
}: {
  item: OrderItem
  onDone: () => void
}) {
  const { data: existingData, isLoading } = useMyReviewByOrderItem(item.productId, item.id)
  const existing = existingData?.data ?? null

  const [rating, setRating] = useState(existing?.rating ?? 0)
  const [comment, setComment] = useState(existing?.comment ?? '')

  // Keep form in sync when existing review loads
  const [initialized, setInitialized] = useState(false)
  if (!isLoading && existing && !initialized) {
    setRating(existing.rating)
    setComment(existing.comment ?? '')
    setInitialized(true)
  }

  const createReview = useCreateReview(item.productId)
  const updateReview = useUpdateReview(item.productId)
  const isPending = createReview.isPending || updateReview.isPending

  function handleSubmit() {
    if (rating === 0) {
      showToast('Please select a rating', 'error')
      return
    }
    if (existing) {
      updateReview.mutate(
        { reviewId: existing.id, req: { rating, comment: comment || undefined } },
        {
          onSuccess: () => {
            showToast('Review updated', 'success')
            onDone()
          },
          onError: () => showToast('Failed to update review', 'error'),
        },
      )
    } else {
      createReview.mutate(
        { orderItemId: item.id, rating, comment: comment || undefined },
        {
          onSuccess: () => {
            showToast('Review submitted', 'success')
            onDone()
          },
          onError: (err: unknown) => {
            const code = (err as { response?: { data?: { code?: string } } })?.response?.data?.code
            if (code === 'ALREADY_REVIEWED') {
              showToast('Already reviewed this item', 'error')
            } else {
              showToast('Failed to submit review', 'error')
            }
            onDone()
          },
        },
      )
    }
  }

  if (isLoading) {
    return <p className="text-fg-subtle text-sm text-center py-4">Loading...</p>
  }

  return (
    <div className="space-y-4">
      {existing && (
        <p className="text-xs text-fg-subtle">
          You reviewed this on {new Date(existing.createdAt).toLocaleDateString()}.
          Update your review below.
        </p>
      )}
      <div className="flex flex-col items-center gap-2">
        <StarRating value={rating} onChange={setRating} size="md" />
        <p className="text-xs text-fg-subtle">{rating > 0 ? `${rating} / 5` : 'Select a rating'}</p>
      </div>
      <textarea
        value={comment}
        onChange={(e) => setComment(e.target.value)}
        placeholder="Write a comment (optional)..."
        rows={3}
        className="w-full bg-surface-overlay border border-surface-border rounded-md px-3 py-2 text-sm text-fg-base placeholder:text-fg-subtle focus:outline-none focus:border-accent resize-none"
      />
      <div className="flex gap-3">
        <Button
          type="button"
          variant="outline"
          className="flex-1"
          disabled={isPending}
          onClick={onDone}
        >
          Skip
        </Button>
        <Button
          type="button"
          className="flex-1"
          disabled={isPending || rating === 0}
          onClick={handleSubmit}
        >
          {isPending ? 'Saving...' : existing ? 'Update Review' : 'Submit Review'}
        </Button>
      </div>
    </div>
  )
}

export function ReviewDialog({ items, onClose }: ReviewDialogProps) {
  const [index, setIndex] = useState(0)

  const current = items[index]
  const isLast = index >= items.length - 1

  function handleDone() {
    if (isLast) {
      onClose()
    } else {
      setIndex((i) => i + 1)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/60" onClick={onClose} />
      <div className="relative z-10 bg-surface-raised border border-surface-border rounded-xl shadow-xl w-full max-w-md mx-4 p-6">
        <div className="flex items-center justify-between mb-5">
          <h2 className="font-display text-lg text-fg-base">Rate Your Purchase</h2>
          <button
            type="button"
            onClick={onClose}
            className="text-fg-subtle hover:text-fg-base transition-colors text-lg leading-none"
          >
            ✕
          </button>
        </div>

        <div className="mb-5">
          <p className="text-sm font-medium text-fg-base">{current.productName}</p>
          <p className="text-xs text-fg-subtle mt-0.5">
            Qty: {current.quantity} — item {index + 1} of {items.length}
          </p>
        </div>

        <ReviewItemForm key={current.id} item={current} onDone={handleDone} />
      </div>
    </div>
  )
}
