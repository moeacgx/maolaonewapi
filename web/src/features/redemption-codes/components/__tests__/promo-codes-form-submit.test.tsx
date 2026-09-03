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
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import {
  apiClient,
  buildPromoCode,
  renderPromoCodesPanel,
  resetApiClient,
} from './promo-codes-test-helpers'

const { toast } = await import('sonner')

const originalConsoleLog = Reflect.get(console, 'log') as typeof console.log

function getInputByLabel(labelText: string): HTMLInputElement {
  const label = [...document.querySelectorAll('label')].find(
    (candidate) => candidate.textContent === labelText
  )
  if (!label) throw new Error(`Expected a "${labelText}" label`)
  const input = label.parentElement?.querySelector('input')
  if (!input) throw new Error(`Expected an input for "${labelText}"`)
  return input
}

async function openCreateSheet(): Promise<void> {
  renderPromoCodesPanel([buildPromoCode(1)])
  await screen.findByText('promo-1')
  fireEvent.click(screen.getByRole('button', { name: /create promo code/i }))
  await screen.findByRole('heading', { name: 'Create Promo Code' })
}

function fillRequiredFields(): void {
  fireEvent.change(getInputByLabel('Name'), {
    target: { value: 'Spring Sale' },
  })
  fireEvent.change(getInputByLabel('Promo Code'), {
    target: { value: 'spring10' },
  })
}

function getSaveButton(): HTMLElement {
  return screen.getByRole('button', { name: /save changes|saving/i })
}

afterEach(() => {
  resetApiClient()
  Reflect.set(console, 'log', originalConsoleLog)
  toast.dismiss()
})

describe('promo code create/update submit', () => {
  test('blocks submit and does not call the API when required fields are missing', async () => {
    const postCalls: unknown[] = []
    apiClient.post = async (url, data) => {
      postCalls.push({ url, data })
      return { data: { success: true, data: {} } }
    }

    await openCreateSheet()
    fireEvent.click(getSaveButton())

    await waitFor(() =>
      expect(
        screen.getByText('Please complete all required promo code fields')
      ).toBeInTheDocument()
    )
    expect(postCalls).toHaveLength(0)
  })

  test('creates a promo code with the trimmed/normalized payload and closes the sheet on success', async () => {
    const postCalls: Array<{ url: string; data: unknown }> = []
    apiClient.post = async (url, data) => {
      postCalls.push({ url, data })
      return { data: { success: true, data: buildPromoCode(2) } }
    }

    await openCreateSheet()
    fillRequiredFields()
    fireEvent.click(getSaveButton())

    await waitFor(() => expect(postCalls).toHaveLength(1))
    expect(postCalls[0]?.url).toBe('/api/promo_code/')
    expect(postCalls[0]?.data).toEqual({
      name: 'Spring Sale',
      code: 'SPRING10',
      discount_type: 'percent',
      discount_value: 10,
      applies_to_topup: true,
      applies_to_all_subscription: false,
      subscription_plan_ids: '',
      max_redeem_count: 0,
      expired_time: 0,
    })

    expect(
      await screen.findByText('Promo code saved successfully')
    ).toBeInTheDocument()
    await waitFor(() =>
      expect(
        screen.queryByRole('heading', { name: 'Create Promo Code' })
      ).not.toBeInTheDocument()
    )
  })

  test('shows an error toast, keeps the sheet open, and re-enables Save when creation fails over the network', async () => {
    Reflect.set(console, 'log', () => undefined)
    apiClient.post = async () => {
      throw new Error('Network Error')
    }
    const errorSpy = vi.spyOn(toast, 'error')

    await openCreateSheet()
    fillRequiredFields()
    fireEvent.click(getSaveButton())

    await waitFor(() =>
      expect(errorSpy).toHaveBeenCalledWith('Something went wrong!')
    )

    expect(
      screen.getByRole('heading', { name: 'Create Promo Code' })
    ).toBeInTheDocument()
    expect(getSaveButton()).toBeEnabled()
    expect(getSaveButton()).toHaveTextContent('Save changes')
  })
})
