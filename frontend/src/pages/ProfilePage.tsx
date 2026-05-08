import { useState, useEffect } from 'react'
import {
  useProfile,
  useUpdateProfile,
  useAddAddress,
  useUpdateAddress,
  useDeleteAddress,
  useSetDefaultAddress,
} from '@/features/profile/useProfile'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import type { Address } from '@/types/auth'
import type { AddressRequest } from '@/features/profile/profileApi'

const EMPTY_ADDR: AddressRequest = {
  label: '', address_line1: '', address_line2: '',
  city: '', state: '', country: '', postal_code: '',
}

function AddressForm({
  initial,
  onSave,
  onCancel,
  isPending,
}: {
  initial: AddressRequest
  onSave: (body: AddressRequest) => void
  onCancel: () => void
  isPending: boolean
}) {
  const [form, setForm] = useState<AddressRequest>(initial)
  const set = (k: keyof AddressRequest, v: string) => setForm((p) => ({ ...p, [k]: v }))

  return (
    <div className="mt-3 p-4 bg-surface-base border border-surface-border rounded-lg space-y-3">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {(
          [
            ['label', 'Label (optional)'],
            ['address_line1', 'Street Address *'],
            ['address_line2', 'Apt / Suite (optional)'],
            ['city', 'City *'],
            ['state', 'State / Province'],
            ['country', 'Country *'],
            ['postal_code', 'ZIP / Postal Code'],
          ] as [keyof AddressRequest, string][]
        ).map(([field, label]) => (
          <div key={field} className={field === 'address_line1' ? 'sm:col-span-2' : ''}>
            <label className="block text-xs text-zinc-400 mb-1">{label}</label>
            <input
              type="text"
              value={form[field] ?? ''}
              onChange={(e) => set(field, e.target.value)}
              required={['address_line1', 'city', 'country'].includes(field)}
              className="w-full bg-surface-overlay border border-surface-border rounded-md px-3 py-2 text-sm text-white placeholder-zinc-600 focus:outline-none focus:border-accent transition-colors"
            />
          </div>
        ))}
      </div>
      <div className="flex gap-2">
        <Button size="sm" onClick={() => onSave(form)} disabled={isPending}>
          {isPending ? 'Saving…' : 'Save'}
        </Button>
        <Button size="sm" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </div>
  )
}

export function ProfilePage() {
  const { data, isLoading } = useProfile()
  const updateProfile = useUpdateProfile()
  const addAddress = useAddAddress()
  const updateAddress = useUpdateAddress()
  const deleteAddress = useDeleteAddress()
  const setDefault = useSetDefaultAddress()

  const profile = data?.data
  const [firstName, setFirstName] = useState('')
  const [lastName, setLastName] = useState('')
  const [phone, setPhone] = useState('')

  useEffect(() => {
    if (profile) {
      setFirstName(profile.first_name)
      setLastName(profile.last_name)
      setPhone(profile.phone ?? '')
    }
  }, [profile])

  // editingId: null = none open, 'new' = add form, address.id = edit form
  const [editingId, setEditingId] = useState<string | null>(null)

  function handleSaveProfile(e: React.FormEvent) {
    e.preventDefault()
    updateProfile.mutate({ first_name: firstName, last_name: lastName, phone: phone || undefined })
  }

  function handleSaveAddress(body: AddressRequest) {
    if (editingId === 'new') {
      addAddress.mutate(body, { onSuccess: () => setEditingId(null) })
    } else if (editingId) {
      updateAddress.mutate({ id: editingId, body }, { onSuccess: () => setEditingId(null) })
    }
  }

  function addrToRequest(a: Address): AddressRequest {
    return {
      label: a.label ?? '',
      address_line1: a.address_line1,
      address_line2: a.address_line2 ?? '',
      city: a.city,
      state: a.state ?? '',
      country: a.country,
      postal_code: a.postal_code ?? '',
    }
  }

  const initials = profile
    ? `${profile.first_name[0] ?? ''}${profile.last_name[0] ?? ''}`.toUpperCase()
    : '?'

  return (
    <div className="max-w-2xl mx-auto px-4 sm:px-6 py-8 space-y-8">
      {/* Header */}
      {isLoading ? (
        <div className="flex items-center gap-4">
          <Skeleton className="w-14 h-14 rounded-full" />
          <div className="space-y-2">
            <Skeleton className="h-5 w-32" />
            <Skeleton className="h-4 w-48" />
          </div>
        </div>
      ) : (
        <div className="flex items-center gap-4">
          <div className="w-14 h-14 rounded-full bg-accent flex items-center justify-center flex-shrink-0">
            <span className="font-mono font-semibold text-surface-base text-lg">{initials}</span>
          </div>
          <div>
            <p className="text-white font-semibold">
              {profile?.first_name} {profile?.last_name}
            </p>
            <p className="text-sm text-zinc-500">{profile?.email}</p>
          </div>
        </div>
      )}

      {/* Personal Info */}
      <div className="bg-surface-raised border border-surface-border rounded-lg p-5">
        <h2 className="text-sm font-semibold text-white mb-4">Personal Info</h2>
        {isLoading ? (
          <div className="space-y-3">
            <Skeleton className="h-9 w-full" />
            <Skeleton className="h-9 w-full" />
            <Skeleton className="h-9 w-full" />
          </div>
        ) : (
          <form onSubmit={handleSaveProfile} className="space-y-3">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <label className="block text-xs text-zinc-400 mb-1">First Name</label>
                <input
                  type="text"
                  value={firstName}
                  onChange={(e) => setFirstName(e.target.value)}
                  required
                  className="w-full bg-surface-base border border-surface-border rounded-md px-3 py-2 text-sm text-white focus:outline-none focus:border-accent transition-colors"
                />
              </div>
              <div>
                <label className="block text-xs text-zinc-400 mb-1">Last Name</label>
                <input
                  type="text"
                  value={lastName}
                  onChange={(e) => setLastName(e.target.value)}
                  required
                  className="w-full bg-surface-base border border-surface-border rounded-md px-3 py-2 text-sm text-white focus:outline-none focus:border-accent transition-colors"
                />
              </div>
            </div>
            <div>
              <label className="block text-xs text-zinc-400 mb-1">Phone (optional)</label>
              <input
                type="text"
                value={phone}
                onChange={(e) => setPhone(e.target.value)}
                className="w-full bg-surface-base border border-surface-border rounded-md px-3 py-2 text-sm text-white focus:outline-none focus:border-accent transition-colors"
              />
            </div>
            <Button type="submit" disabled={updateProfile.isPending}>
              {updateProfile.isPending ? 'Saving…' : 'Save Changes'}
            </Button>
          </form>
        )}
      </div>

      {/* Addresses */}
      <div className="bg-surface-raised border border-surface-border rounded-lg p-5">
        <h2 className="text-sm font-semibold text-white mb-4">Addresses</h2>

        {isLoading ? (
          <div className="space-y-3">
            <Skeleton className="h-16 w-full rounded-md" />
            <Skeleton className="h-16 w-full rounded-md" />
          </div>
        ) : (
          <div className="space-y-3">
            {(profile?.addresses ?? []).map((addr) => (
              <div key={addr.id}>
                <div className="flex items-start justify-between gap-3 p-3 border border-surface-border rounded-md">
                  <div className="flex items-start gap-2 min-w-0">
                    <span
                      className={`mt-1 flex-shrink-0 w-2.5 h-2.5 rounded-full border-2 ${
                        addr.is_default ? 'bg-accent border-accent' : 'border-zinc-500'
                      }`}
                    />
                    <div className="text-xs text-zinc-300 leading-relaxed min-w-0">
                      {addr.label && (
                        <span className="text-white font-medium block">
                          {addr.label}
                          {addr.is_default && (
                            <span className="ml-2 text-accent text-xs">(Default)</span>
                          )}
                        </span>
                      )}
                      <span className="text-zinc-400">
                        {addr.address_line1}
                        {addr.address_line2 && `, ${addr.address_line2}`}, {addr.city}
                        {addr.state && `, ${addr.state}`}, {addr.country}
                        {addr.postal_code && ` ${addr.postal_code}`}
                      </span>
                    </div>
                  </div>
                  <div className="flex gap-1 flex-shrink-0">
                    {!addr.is_default && (
                      <button
                        type="button"
                        onClick={() => setDefault.mutate(addr.id)}
                        disabled={setDefault.isPending}
                        className="text-xs text-zinc-500 hover:text-accent px-2 py-1 transition-colors disabled:opacity-40"
                      >
                        Default
                      </button>
                    )}
                    <button
                      type="button"
                      onClick={() => setEditingId(editingId === addr.id ? null : addr.id)}
                      className="text-xs text-zinc-500 hover:text-white px-2 py-1 transition-colors"
                    >
                      Edit
                    </button>
                    <button
                      type="button"
                      onClick={() => deleteAddress.mutate(addr.id)}
                      disabled={deleteAddress.isPending}
                      className="text-xs text-zinc-500 hover:text-status-failed px-2 py-1 transition-colors disabled:opacity-40"
                    >
                      Delete
                    </button>
                  </div>
                </div>

                {editingId === addr.id && (
                  <AddressForm
                    initial={addrToRequest(addr)}
                    onSave={handleSaveAddress}
                    onCancel={() => setEditingId(null)}
                    isPending={updateAddress.isPending}
                  />
                )}
              </div>
            ))}

            {/* Add form */}
            {editingId === 'new' ? (
              <AddressForm
                initial={EMPTY_ADDR}
                onSave={handleSaveAddress}
                onCancel={() => setEditingId(null)}
                isPending={addAddress.isPending}
              />
            ) : (
              <button
                type="button"
                onClick={() => setEditingId('new')}
                className="text-sm text-accent hover:text-accent-dim transition-colors mt-1"
              >
                + Add Address
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
