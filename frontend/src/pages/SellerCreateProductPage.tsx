import { Link } from 'react-router-dom'
import { ProductForm, type ProductFormData } from '@/features/seller/ProductForm'
import { useCreateProduct } from '@/features/seller/useSellerProducts'
import type { CreateProductBody } from '@/types/seller'

export function SellerCreateProductPage() {
  const createProduct = useCreateProduct()

  function handleSubmit(formData: ProductFormData) {
    const body: CreateProductBody = {
      name: formData.name,
      description: formData.description || undefined,
      price: parseFloat(formData.price),
      categoryId: formData.categoryId ? parseInt(formData.categoryId) : undefined,
      stockQuantity: parseInt(formData.stockQuantity),
      images: formData.images.filter((img) => img.url.trim()).map((img) => ({
        url: img.url.trim(),
        altText: img.altText || undefined,
        sortOrder: img.sortOrder,
      })),
    }
    createProduct.mutate(body)
  }

  return (
    <div className="max-w-2xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      <div className="mb-6">
        <Link to="/seller/products" className="text-sm text-fg-subtle hover:text-fg-base transition-colors">
          ← Back to My Products
        </Link>
        <h1 className="font-display text-3xl text-fg-base mt-3">New Product</h1>
      </div>

      <div className="bg-surface-raised border border-surface-border rounded-lg p-6">
        <ProductForm
          onSubmit={handleSubmit}
          isPending={createProduct.isPending}
          isEdit={false}
        />
      </div>
    </div>
  )
}
