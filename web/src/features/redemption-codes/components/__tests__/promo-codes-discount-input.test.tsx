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
// Verifies the fixed-discount input reuses the shared currency/quota-step helpers instead of hardcoding unit or precision.
import { fireEvent, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test } from 'vitest'

import { getEditableQuotaStep } from '@/lib/format'

import {
  buildPromoCode,
  renderPromoCodesPanel,
  resetApiClient,
} from './promo-codes-test-helpers'

const { useSystemConfigStore } = await import('@/stores/system-config-store')

function setCurrency(
  overrides: Partial<
    ReturnType<typeof useSystemConfigStore.getState>['config']['currency']
  >
): void {
  useSystemConfigStore.getState().setConfig({
    currency: {
      displayInCurrency: true,
      quotaDisplayType: 'USD',
      quotaPerUnit: 500000,
      usdExchangeRate: 1,
      customCurrencySymbol: '¤',
      customCurrencyExchangeRate: 1,
      ...overrides,
    },
  })
}

async function openCreateSheet(): Promise<void> {
  renderPromoCodesPanel([buildPromoCode(1)])
  await screen.findByText('promo-1')
  fireEvent.click(screen.getByRole('button', { name: /create promo code/i }))
  // The trigger button also reads "Create Promo Code", so scope to the heading.
  await screen.findByRole('heading', { name: 'Create Promo Code' })
}

async function switchToFixedDiscount(
  user: ReturnType<typeof userEvent.setup>
): Promise<void> {
  await user.click(screen.getByRole('combobox'))
  await user.click(await screen.findByRole('option', { name: /fixed quota/i }))
}

function getDiscountValueInput(): HTMLInputElement {
  const label = [...document.querySelectorAll('label')].find((candidate) =>
    candidate.textContent?.startsWith('Discount Value')
  )
  if (!label) throw new Error('Expected a "Discount Value" label')
  const input = label.parentElement?.querySelector('input')
  if (!input) throw new Error('Expected the discount value input')
  return input
}

afterEach(() => {
  resetApiClient()
  localStorage.clear()
})

describe('promo code discount input', () => {
  test('uses a whole-percent step and a % label for percentage discounts', async () => {
    setCurrency({ quotaDisplayType: 'USD' })
    await openCreateSheet()

    expect(screen.getByText('Discount Value (%)')).toBeInTheDocument()
    expect(getDiscountValueInput().step).toBe('1')
  })

  test('reuses the shared currency step and shows the configured currency label for fixed discounts', async () => {
    const user = userEvent.setup()
    setCurrency({ quotaDisplayType: 'USD' })
    await openCreateSheet()

    await switchToFixedDiscount(user)

    expect(await screen.findByText('Discount Value (USD)')).toBeInTheDocument()
    expect(getDiscountValueInput().step).toBe(String(getEditableQuotaStep()))
    expect(
      screen.getByText('Enter the discount amount in USD')
    ).toBeInTheDocument()
  })

  test('switches to an integer step and tokens wording under TOKENS display', async () => {
    const user = userEvent.setup()
    setCurrency({ quotaDisplayType: 'TOKENS' })
    await openCreateSheet()

    await switchToFixedDiscount(user)

    expect(
      await screen.findByText('Discount Value (Tokens)')
    ).toBeInTheDocument()
    expect(getDiscountValueInput().step).toBe('1')
    expect(
      screen.getByText('Enter the discount amount in tokens')
    ).toBeInTheDocument()
  })

  test('reflects a CNY display the same way as USD, via the shared label helper', async () => {
    const user = userEvent.setup()
    setCurrency({ quotaDisplayType: 'CNY', usdExchangeRate: 7 })
    await openCreateSheet()

    await switchToFixedDiscount(user)

    expect(await screen.findByText('Discount Value (CNY)')).toBeInTheDocument()
    expect(getDiscountValueInput().step).toBe(String(getEditableQuotaStep()))
  })
})
