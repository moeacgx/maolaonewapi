/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

const i18n = (await import('i18next')).default
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { QueryClient, QueryClientProvider } = await import(
  '@tanstack/react-query'
)
const { Toaster, toast } = await import('sonner')
const { api } = await import('@/lib/api')
const { PromoCodesPanel } = await import('../promo-codes-panel')

await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

type ApiMethod = (
  url: string,
  data?: unknown,
  config?: unknown
) => Promise<{ data: unknown }>
type MockableApi = {
  get: ApiMethod
  delete: ApiMethod
}

const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get
const originalDelete = apiClient.delete

function promo(id: number, name = `promo-${id}`) {
  return {
    id,
    user_id: 1,
    name,
    code: `CODE${id}`,
    status: 1,
    discount_type: 'percent' as const,
    discount_value: 10,
    applies_to_topup: true,
    applies_to_all_subscription: true,
    subscription_plan_ids: '',
    max_redeem_count: 0,
    redeemed_count: 0,
    created_time: 1,
    updated_time: 1,
    expired_time: 0,
  }
}

function renderPanel(items: ReturnType<typeof promo>[]) {
  apiClient.get = async (url) => {
    if (url.startsWith('/api/promo_code/?')) {
      return {
        data: {
          success: true,
          data: { items, total: items.length, page: 1, page_size: 20 },
        },
      }
    }
    if (url === '/api/subscriptions/plans') {
      return { data: { success: true, data: [] } }
    }
    throw new Error(`Unexpected GET ${url}`)
  }

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <PromoCodesPanel />
      </QueryClientProvider>
      <Toaster duration={60_000} />
    </I18nextProvider>
  )
}

afterEach(() => {
  apiClient.get = originalGet
  apiClient.delete = originalDelete
  toast.dismiss()
})

describe('promo code bulk delete', () => {
  test('uses the registered promo_code batch route with selected ids', async () => {
    const calls: Array<{ url: string; config: unknown }> = []
    apiClient.delete = async (url, config) => {
      calls.push({ url, config })
      return {
        data: { success: true, data: { deleted_ids: [7], skipped: [] } },
      }
    }

    renderPanel([promo(7)])
    await screen.findByText('promo-7')

    const row = screen.getByText('promo-7').closest('tr')
    if (!row) throw new Error('Expected promo row')
    fireEvent.click(within(row).getByLabelText('Select row'))

    fireEvent.click(
      screen.getByRole('button', { name: /delete selected \(1\)/i })
    )
    fireEvent.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() => expect(calls).toHaveLength(1))
    expect(calls[0]?.url).toBe('/api/promo_code/batch')
    expect(calls[0]?.config).toEqual({ data: { ids: [7] } })
  })

  test('reports deleted and skipped counts from deleted_ids, not the request size', async () => {
    apiClient.delete = async () => ({
      data: {
        success: true,
        data: { deleted_ids: [1], skipped: [{ id: 2, reason: 'reserved' }] },
      },
    })

    renderPanel([promo(1), promo(2)])
    await screen.findByText('promo-1')

    const row1 = screen.getByText('promo-1').closest('tr')
    const row2 = screen.getByText('promo-2').closest('tr')
    if (!row1 || !row2) throw new Error('Expected promo rows')
    fireEvent.click(within(row1).getByLabelText('Select row'))
    fireEvent.click(within(row2).getByLabelText('Select row'))

    fireEvent.click(
      screen.getByRole('button', { name: /delete selected \(2\)/i })
    )
    fireEvent.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() =>
      expect(document.body).toHaveTextContent(
        'Deleted 1 promo code(s), skipped 1'
      )
    )
  })

  test('cleans up invalid promo codes via the batch-shaped invalid contract', async () => {
    const calls: string[] = []
    apiClient.delete = async (url) => {
      calls.push(url)
      return {
        data: {
          success: true,
          data: { deleted_ids: [3, 4], skipped: [] },
        },
      }
    }

    renderPanel([promo(3)])
    await screen.findByText('promo-3')

    fireEvent.click(screen.getByRole('button', { name: /delete invalid/i }))
    fireEvent.click(screen.getByRole('button', { name: /^delete invalid$/i }))

    await waitFor(() => expect(calls).toContain('/api/promo_code/invalid'))
    await waitFor(() =>
      expect(document.body).toHaveTextContent(
        'Successfully deleted 2 invalid promo codes'
      )
    )
  })

  test('surfaces a server error message instead of a false success toast', async () => {
    apiClient.delete = async () => ({
      data: { success: false, message: 'cannot delete reserved code' },
    })
    const errorSpy = vi.spyOn(toast, 'error')

    renderPanel([promo(9)])
    await screen.findByText('promo-9')

    const row = screen.getByText('promo-9').closest('tr')
    if (!row) throw new Error('Expected promo row')
    fireEvent.click(within(row).getByLabelText('Select row'))

    fireEvent.click(
      screen.getByRole('button', { name: /delete selected \(1\)/i })
    )
    fireEvent.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() =>
      expect(errorSpy).toHaveBeenCalledWith('cannot delete reserved code')
    )
  })
})
