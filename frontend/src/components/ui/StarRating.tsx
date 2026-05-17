interface StarRatingProps {
  value: number
  onChange?: (v: number) => void
  size?: 'sm' | 'md'
}

export function StarRating({ value, onChange, size = 'md' }: StarRatingProps) {
  const starSize = size === 'sm' ? 'text-sm' : 'text-xl'
  const readonly = !onChange

  return (
    <div className="flex items-center gap-0.5">
      {[1, 2, 3, 4, 5].map((star) => (
        <button
          key={star}
          type="button"
          disabled={readonly}
          onClick={() => onChange?.(star)}
          className={`${starSize} leading-none transition-colors ${
            readonly
              ? 'cursor-default'
              : 'cursor-pointer hover:scale-110 transition-transform'
          } ${star <= Math.round(value) ? 'text-amber-400' : 'text-fg-subtle'}`}
        >
          ★
        </button>
      ))}
    </div>
  )
}
