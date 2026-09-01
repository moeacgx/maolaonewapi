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
// Shared fixtures/render harness for PromoCodesPanel tests; not itself a test file, so vitest's `include` glob skips it.
import { QueryClientProvider, QueryClient } from '@tanstack/react-query'
import { render } from '@testing-library/react'

import type { PromoCode } from '../../types'
import { PromoCodesPanel } from '../promo-codes-panel'

const i18n = (await import('i18next')).default
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { Toaster } = await import('sonner')
const { api } = await import('@/lib/api')

await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

export type ApiMethod = (
  url: string,
  data?: unknown,
  config?: unknown
) => Promise<{ data: unknown }>

export type MockableApi = {
  get: ApiMethod
  post: ApiMethod
  put: ApiMethod
  delete: ApiMethod
}

export const apiClient = api as unknown as MockableApi

const originalMethods = {
  get: apiClient.get,
  post: apiClient.post,
  put: apiClient.put,
  delete: apiClient.delete,
}

/** Restore the real axios instance methods after a test replaces them. */
export function resetApiClient(): void {
  apiClient.get = originalMethods.get
  apiClient.post = originalMethods.post
  apiClient.put = originalMethods.put
  apiClient.delete = originalMethods.delete
}

/** Builds a schema-valid PromoCode fixture (subscription_plan_ids is a CSV string, not an array). */
export function buildPromoCode(
  id: number,
  overrides: Partial<PromoCode> = {}
): PromoCode {
  return {
    id,
    user_id: 1,
    name: `promo-${id}`,
    code: `CODE${id}`,
    status: 1,
    discount_type: 'percent',
    discount_value: 10,
    applies_to_topup: true,
    applies_to_all_subscription: true,
    subscription_plan_ids: '',
    max_redeem_count: 0,
    redeemed_count: 0,
    created_time: 1,
    updated_time: 1,
    expired_time: 0,
    ...overrides,
  }
}

/** Mounts PromoCodesPanel with the list + admin plans GETs mocked; set apiClient.post/put/delete beforehand for mutation tests. */
export function renderPromoCodesPanel(items: PromoCode[]) {
  apiClient.get = async (url) => {
    if (url.startsWith('/api/promo_code/?')) {
      return {
        data: {
          success: true,
          data: { items, total: items.length, page: 1, page_size: 20 },
        },
      }
    }
    if (url === '/api/subscription/admin/plans') {
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

export { i18n }
