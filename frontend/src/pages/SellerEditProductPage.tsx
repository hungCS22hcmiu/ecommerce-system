import { Link, useParams } from 'react-router-dom'
import { useSellerProduct, useUpdateProduct } from '@/features/seller/useSellerProducts'
import { ProductForm, type ProductFormData } from '@/features/seller/ProductForm'
import { Skeleton } from '@/components/ui/skeleton'
import type { UpdateProductBody } from '@/types/seller'

export function SellerEditProductPage() {
  const { id } = useParams<{ id: string }>()
  const productId = Number(id)

  const { data, isLoading, isError } = useSellerProduct(productId)
  const updateProduct = useUpdateProduct()

  const product = data?.data

  function handleSubmit(formData: ProductFormData) {
    const body: UpdateProductBody = {
      name: formData.name || undefined,
      description: formData.description || undefined,
      price: formData.price ? parseFloat(formData.price) : undefined,
      categoryId: formData.categoryId ? parseInt(formData.categoryId) : undefined,
      status: formData.status,
      stockQuantity: formData.stockQuantity ? parseInt(formData.stockQuantity) : undefined,
      images: formData.images.filter((img) => img.url.trim()).map((img) => ({
        url: img.url.trim(),
        altText: img.altText || undefined,
        sortOrder: img.sortOrder,
      })),
    }
    updateProduct.mutate({ id: productId, body })
  }

  return (
    <div className="max-w-2xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      <div className="mb-6">
        <Link to="/seller/products" className="text-sm text-fg-subtle hover:text-fg-base transition-colors">
          ← Back to My Products
        </Link>
        <h1 className="font-display text-3xl text-fg-base mt-3">Edit Product</h1>
      </div>

      {isError && (
        <p className="text-sm text-status-failed">Failed to load product. Please try again.</p>
      )}

      {isLoading && (
        <div className="bg-surface-raised border border-surface-border rounded-lg p-6 space-y-4">
          <Skeleton className="h-5 w-32" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-5 w-28" />
          <Skeleton className="h-24 w-full" />
          <div className="grid grid-cols-2 gap-4">
            <Skeleton className="h-10" />
            <Skeleton className="h-10" />
          </div>
        </div>
      )}

      {product && (
        <div className="bg-surface-raised border border-surface-border rounded-lg p-6">
          <ProductForm
            defaultValues={{
              name: product.name,
              description: product.description ?? '',
              price: String(product.price),
              categoryId: product.categoryId ? String(product.categoryId) : '',
              categoryName: product.categoryName ?? '',
              stockQuantity: String(product.stockQuantity),
              status: product.status,
              images: product.images.map((img) => ({
                url: img.url,
                altText: img.altText ?? '',
                sortOrder: img.sortOrder,
              })),
            }}
            onSubmit={handleSubmit}
            isPending={updateProduct.isPending}
            isEdit={true}
          />
        </div>
      )}
    </div>
  )
}
