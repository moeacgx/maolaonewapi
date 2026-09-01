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
import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

import {
  buildPromoCode,
  renderPromoCodesPanel,
  resetApiClient,
} from './promo-codes-test-helpers'

const { toast } = await import('sonner')

function rowFor(name: string): HTMLElement {
  const row = screen.getByText(name).closest('tr')
  if (!row) throw new Error(`Expected a row for "${name}"`)
  return row
}

afterEach(() => {
  resetApiClient()
  toast.dismiss()
})

describe('promo codes selection toolbar', () => {
  test('does not render the delete-selected button when nothing is selected', async () => {
    renderPromoCodesPanel([buildPromoCode(1), buildPromoCode(2)])
    await screen.findByText('promo-1')

    expect(
      screen.queryByRole('button', { name: /delete selected/i })
    ).not.toBeInTheDocument()
  })

  test('shows the delete-selected button with a live count once a row is checked', async () => {
    renderPromoCodesPanel([buildPromoCode(1), buildPromoCode(2)])
    await screen.findByText('promo-1')

    fireEvent.click(within(rowFor('promo-1')).getByLabelText('Select row'))
    expect(
      await screen.findByRole('button', { name: 'Delete selected (1)' })
    ).toBeInTheDocument()

    fireEvent.click(within(rowFor('promo-2')).getByLabelText('Select row'))
    expect(
      await screen.findByRole('button', { name: 'Delete selected (2)' })
    ).toBeInTheDocument()
  })

  test('selects and clears every row on the current page via the header checkbox', async () => {
    renderPromoCodesPanel([buildPromoCode(1), buildPromoCode(2)])
    await screen.findByText('promo-1')

    fireEvent.click(screen.getByLabelText('Select all'))
    expect(
      await screen.findByRole('button', { name: 'Delete selected (2)' })
    ).toBeInTheDocument()

    fireEvent.click(screen.getByLabelText('Select all'))
    await waitFor(() =>
      expect(
        screen.queryByRole('button', { name: /delete selected/i })
      ).not.toBeInTheDocument()
    )
  })

  test('clears selection when the page changes so a prior page id cannot leak into a later delete', async () => {
    // 21 items exceeds the panel's fixed pageSize of 20, so "Next" is enabled.
    const items = Array.from({ length: 21 }, (_, index) =>
      buildPromoCode(index + 1)
    )
    renderPromoCodesPanel(items)
    await screen.findByText('promo-1')

    fireEvent.click(within(rowFor('promo-1')).getByLabelText('Select row'))
    expect(
      await screen.findByRole('button', { name: 'Delete selected (1)' })
    ).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Next' }))

    await waitFor(() =>
      expect(
        screen.queryByRole('button', { name: /delete selected/i })
      ).not.toBeInTheDocument()
    )
  })

  test('exposes Delete Invalid from the More menu independently of row selection', async () => {
    renderPromoCodesPanel([buildPromoCode(1)])
    await screen.findByText('promo-1')

    expect(
      screen.queryByRole('button', { name: /delete selected/i })
    ).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'More' }))
    const deleteInvalidItem = await screen.findByRole('menuitem', {
      name: 'Delete Invalid',
    })
    fireEvent.click(deleteInvalidItem)

    expect(
      await screen.findByText('Delete Invalid Promo Codes?')
    ).toBeInTheDocument()
  })
})
