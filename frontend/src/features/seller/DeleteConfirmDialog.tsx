import { Button } from '@/components/ui/button'

interface Props {
  productName: string
  onConfirm: () => void
  onCancel: () => void
  isPending: boolean
}

export function DeleteConfirmDialog({ productName, onConfirm, onCancel, isPending }: Props) {
  return (
    <div className="flex items-center gap-3 p-3 bg-red-500/10 border border-red-500/30 rounded-md">
      <span className="text-sm text-fg-base flex-1">
        Delete <span className="font-medium">"{productName}"</span>? This cannot be undone.
      </span>
      <Button
        size="sm"
        variant="destructive"
        onClick={onConfirm}
        disabled={isPending}
      >
        {isPending ? 'Deleting…' : 'Delete'}
      </Button>
      <Button size="sm" variant="outline" onClick={onCancel} disabled={isPending}>
        Cancel
      </Button>
    </div>
  )
}
