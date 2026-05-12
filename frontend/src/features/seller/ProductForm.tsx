import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { CategoryCombobox } from './CategoryCombobox'

export interface ProductFormData {
  name: string
  description: string
  price: string
  categoryId: string
  categoryName: string
  stockQuantity: string
  status: 'ACTIVE' | 'INACTIVE' | 'DELETED'
  images: Array<{ url: string; altText: string; sortOrder: number }>
}

interface Props {
  defaultValues?: Partial<ProductFormData>
  onSubmit: (data: ProductFormData) => void
  isPending: boolean
  isEdit: boolean
}

const empty: ProductFormData = {
  name: '',
  description: '',
  price: '',
  categoryId: '',
  categoryName: '',
  stockQuantity: '',
  status: 'ACTIVE',
  images: [],
}

export function ProductForm({ defaultValues, onSubmit, isPending, isEdit }: Props) {
  const [form, setForm] = useState<ProductFormData>({ ...empty, ...defaultValues })

  function setField<K extends keyof ProductFormData>(key: K, value: ProductFormData[K]) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  function addImage() {
    if (form.images.length >= 5) return
    setField('images', [...form.images, { url: '', altText: '', sortOrder: form.images.length }])
  }

  function removeImage(index: number) {
    setField(
      'images',
      form.images
        .filter((_, i) => i !== index)
        .map((img, i) => ({ ...img, sortOrder: i }))
    )
  }

  function updateImage(index: number, field: 'url' | 'altText', value: string) {
    const updated = form.images.map((img, i) => (i === index ? { ...img, [field]: value } : img))
    setField('images', updated)
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    onSubmit(form)
  }

  const labelClass = 'block text-sm font-medium text-fg-base mb-1'

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      <div className="space-y-4">
        <div>
          <label className={labelClass}>
            Product Name <span className="text-red-400">*</span>
          </label>
          <Input
            value={form.name}
            onChange={(e) => setField('name', e.target.value)}
            placeholder="e.g. Wireless Headphones"
            maxLength={200}
            required
          />
        </div>

        <div>
          <label className={labelClass}>Description</label>
          <textarea
            value={form.description}
            onChange={(e) => setField('description', e.target.value)}
            placeholder="Describe your product…"
            rows={4}
            className="flex w-full rounded-md border border-surface-border bg-surface-overlay px-3 py-2 text-sm text-fg-base placeholder:text-fg-subtle focus:outline-none focus:ring-1 focus:ring-accent resize-none"
          />
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className={labelClass}>
              Price (USD) <span className="text-red-400">*</span>
            </label>
            <Input
              type="number"
              value={form.price}
              onChange={(e) => setField('price', e.target.value)}
              placeholder="0.00"
              min="0"
              step="0.01"
              required
            />
          </div>
          <div>
            <label className={labelClass}>
              Stock Quantity <span className="text-red-400">*</span>
            </label>
            <Input
              type="number"
              value={form.stockQuantity}
              onChange={(e) => setField('stockQuantity', e.target.value)}
              placeholder="0"
              min="0"
              step="1"
              required
            />
          </div>
        </div>

        <div>
          <label className={labelClass}>Category</label>
          <CategoryCombobox
            value={{
              id: form.categoryId ? parseInt(form.categoryId) : null,
              name: form.categoryName,
            }}
            onChange={(id, name) => {
              setField('categoryId', id != null ? String(id) : '')
              setField('categoryName', name)
            }}
          />
        </div>

        {isEdit && (
          <div>
            <label className={labelClass}>Status</label>
            <select
              value={form.status}
              onChange={(e) => setField('status', e.target.value as ProductFormData['status'])}
              className="flex h-10 w-full rounded-md border border-surface-border bg-surface-overlay px-3 py-2 text-sm text-fg-base focus:outline-none focus:ring-1 focus:ring-accent"
            >
              <option value="ACTIVE">ACTIVE</option>
              <option value="INACTIVE">INACTIVE</option>
              <option value="DELETED">DELETED</option>
            </select>
          </div>
        )}
      </div>

      <div>
        <div className="flex items-center justify-between mb-2">
          <label className={labelClass + ' mb-0'}>Images ({form.images.length}/5)</label>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={addImage}
            disabled={form.images.length >= 5}
          >
            + Add Image
          </Button>
        </div>
        {form.images.length === 0 && (
          <p className="text-sm text-fg-subtle">No images added yet.</p>
        )}
        <div className="space-y-3">
          {form.images.map((img, i) => (
            <div key={i} className="flex gap-2 items-start">
              <div className="flex-1 grid grid-cols-2 gap-2">
                <Input
                  value={img.url}
                  onChange={(e) => updateImage(i, 'url', e.target.value)}
                  placeholder="Image URL (required)"
                  required
                />
                <Input
                  value={img.altText}
                  onChange={(e) => updateImage(i, 'altText', e.target.value)}
                  placeholder="Alt text (optional)"
                />
              </div>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => removeImage(i)}
                className="text-red-400 hover:text-red-300 mt-0.5"
              >
                ✕
              </Button>
            </div>
          ))}
        </div>
      </div>

      <div className="flex gap-3 pt-2">
        <Button type="submit" disabled={isPending}>
          {isPending ? (isEdit ? 'Saving…' : 'Creating…') : (isEdit ? 'Save Changes' : 'Create Product')}
        </Button>
      </div>
    </form>
  )
}
