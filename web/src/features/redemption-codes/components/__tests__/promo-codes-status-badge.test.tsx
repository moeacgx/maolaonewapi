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
import { screen, within } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

import {
  buildPromoCode,
  renderPromoCodesPanel,
  resetApiClient,
} from './promo-codes-test-helpers'

const ONE_HOUR_AGO = Math.floor(Date.now() / 1000) - 3600

afterEach(() => {
  resetApiClient()
})

describe('promo code status badge', () => {
  test.each([
    ['Enabled, never expires', { status: 1, expired_time: 0 }, 'Enabled'],
    [
      'Enabled but past its expiration time',
      { status: 1, expired_time: ONE_HOUR_AGO },
      'Expired',
    ],
    ['Disabled, never expires', { status: 2, expired_time: 0 }, 'Disabled'],
    [
      'Disabled takes precedence over a past expiration time',
      { status: 2, expired_time: ONE_HOUR_AGO },
      'Disabled',
    ],
    ['Exhausted, never expires', { status: 3, expired_time: 0 }, 'Exhausted'],
    [
      'Exhausted takes precedence over a past expiration time',
      { status: 3, expired_time: ONE_HOUR_AGO },
      'Exhausted',
    ],
  ] as const)('%s', async (_case, fields, expectedLabel) => {
    renderPromoCodesPanel([buildPromoCode(1, fields)])

    const row = (await screen.findByText('promo-1')).closest('tr')
    if (!row) throw new Error('Expected a promo row')

    expect(within(row).getByText(expectedLabel)).toBeInTheDocument()
  })
})
