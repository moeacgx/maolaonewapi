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

function rowMenuButton(): HTMLElement {
  return screen.getByRole('button', { name: 'Open menu' })
}

afterEach(() => {
  resetApiClient()
  Reflect.set(console, 'log', originalConsoleLog)
  toast.dismiss()
})

describe('promo code row action menu', () => {
  test('aggregates edit, enable/disable, and delete into a single row menu', async () => {
    renderPromoCodesPanel([buildPromoCode(1)])
    await screen.findByText('promo-1')

    fireEvent.click(rowMenuButton())

    expect(
      await screen.findByRole('menuitem', { name: /edit/i })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('menuitem', { name: /disable/i })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('menuitem', { name: /delete/i })
    ).toBeInTheDocument()
    // Only one trigger per row: no separate always-visible edit/delete controls.
    expect(screen.getAllByRole('button', { name: 'Open menu' })).toHaveLength(1)
  })

  test('opens the edit sheet pre-filled from the row menu', async () => {
    renderPromoCodesPanel([buildPromoCode(1, { name: 'Launch Promo' })])
    await screen.findByText('Launch Promo')

    fireEvent.click(rowMenuButton())
    fireEvent.click(await screen.findByRole('menuitem', { name: /edit/i }))

    expect(await screen.findByText('Update Promo Code')).toBeInTheDocument()
  })

  test('toggles status and shows a success toast', async () => {
    const putCalls: Array<{ url: string; data: unknown }> = []
    apiClient.put = async (url, data) => {
      putCalls.push({ url, data })
      return { data: { success: true, data: {} } }
    }

    renderPromoCodesPanel([buildPromoCode(1, { status: 1 })])
    await screen.findByText('promo-1')

    fireEvent.click(rowMenuButton())
    fireEvent.click(await screen.findByRole('menuitem', { name: /disable/i }))

    await waitFor(() => expect(putCalls).toHaveLength(1))
    expect(putCalls[0]?.url).toBe('/api/promo_code/?status_only=true')
    expect(putCalls[0]?.data).toEqual({ id: 1, status: 2 })
    expect(
      await screen.findByText('Status updated successfully')
    ).toBeInTheDocument()
  })

  test('shows an error toast and re-enables the toggle when it fails over the network', async () => {
    Reflect.set(console, 'log', () => undefined)
    apiClient.put = async () => {
      throw new Error('Network Error')
    }
    const errorSpy = vi.spyOn(toast, 'error')

    renderPromoCodesPanel([buildPromoCode(1, { status: 1 })])
    await screen.findByText('promo-1')

    fireEvent.click(rowMenuButton())
    const disableItem = await screen.findByRole('menuitem', {
      name: /disable/i,
    })
    fireEvent.click(disableItem)

    await waitFor(() =>
      expect(errorSpy).toHaveBeenCalledWith('Something went wrong!')
    )

    // The menu item must not be left permanently disabled after the failure.
    fireEvent.click(rowMenuButton())
    const disableItemAgain = await screen.findByRole('menuitem', {
      name: /disable/i,
    })
    expect(disableItemAgain).not.toHaveAttribute('aria-disabled', 'true')
  })

  test('opens a controlled confirm dialog before deleting a single promo code', async () => {
    renderPromoCodesPanel([buildPromoCode(1, { name: 'Launch Promo' })])
    await screen.findByText('Launch Promo')

    fireEvent.click(rowMenuButton())
    fireEvent.click(await screen.findByRole('menuitem', { name: /delete/i }))

    expect(await screen.findByText('Delete promo code?')).toBeInTheDocument()
    // Row name still shows in the table; dialog also names the target code.
    expect(screen.getAllByText('Launch Promo')).toHaveLength(2)
  })

  test('deletes a single promo code and refreshes the list on confirm', async () => {
    const deleteCalls: string[] = []
    apiClient.delete = async (url) => {
      deleteCalls.push(url)
      return { data: { success: true } }
    }

    renderPromoCodesPanel([buildPromoCode(1)])
    await screen.findByText('promo-1')

    fireEvent.click(rowMenuButton())
    fireEvent.click(await screen.findByRole('menuitem', { name: /delete/i }))
    await screen.findByText('Delete promo code?')
    fireEvent.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() => expect(deleteCalls).toEqual(['/api/promo_code/1/']))
    expect(
      await screen.findByText('Promo code deleted successfully')
    ).toBeInTheDocument()
    await waitFor(() =>
      expect(screen.queryByText('Delete promo code?')).not.toBeInTheDocument()
    )
  })

  test('shows an error toast, keeps the dialog open, and resets loading when single delete fails over the network', async () => {
    Reflect.set(console, 'log', () => undefined)
    apiClient.delete = async () => {
      throw new Error('Network Error')
    }
    const errorSpy = vi.spyOn(toast, 'error')

    renderPromoCodesPanel([buildPromoCode(1)])
    await screen.findByText('promo-1')

    fireEvent.click(rowMenuButton())
    fireEvent.click(await screen.findByRole('menuitem', { name: /delete/i }))
    await screen.findByText('Delete promo code?')

    const confirmButton = screen.getByRole('button', { name: /^delete$/i })
    fireEvent.click(confirmButton)

    await waitFor(() =>
      expect(errorSpy).toHaveBeenCalledWith('Something went wrong!')
    )

    // Confirmation stays open with its button usable again, not stuck loading.
    expect(screen.getByText('Delete promo code?')).toBeInTheDocument()
    expect(confirmButton).toBeEnabled()
  })
})
