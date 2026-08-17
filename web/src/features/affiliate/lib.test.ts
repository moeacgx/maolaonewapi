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
import { describe, expect, test } from 'vitest'

import { isValidWithdrawalAmount } from './lib'

describe('affiliate withdrawal validation', () => {
  test('requires a finite positive display amount that converts to quota', () => {
    expect(isValidWithdrawalAmount(1, 500_000)).toBe(true)
    expect(isValidWithdrawalAmount(Number.NaN, 500_000)).toBe(false)
    expect(isValidWithdrawalAmount(0, 0)).toBe(false)
    expect(isValidWithdrawalAmount(0.000001, 0)).toBe(false)
  })
})
