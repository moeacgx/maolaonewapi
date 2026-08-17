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

import {
  canPlacePredictionBet,
  parsePositiveInteger,
  toUnixSeconds,
} from './lib'

describe('game amount normalization', () => {
  test('accepts only a finite non-negative integer amount', () => {
    expect(parsePositiveInteger('42.9')).toBe(42)
    expect(parsePositiveInteger('-1')).toBe(0)
    expect(parsePositiveInteger('not-a-number')).toBe(0)
    expect(parsePositiveInteger('')).toBe(0)
  })
})

describe('prediction timing', () => {
  test('allows an open prediction only before its close time', () => {
    expect(canPlacePredictionBet('open', 101, 100)).toBe(true)
    expect(canPlacePredictionBet('open', 100, 100)).toBe(false)
    expect(canPlacePredictionBet('settled', 101, 100)).toBe(false)
  })

  test('normalizes local date input to whole Unix seconds', () => {
    expect(toUnixSeconds('1970-01-01T00:00:01Z')).toBe(1)
    expect(toUnixSeconds('invalid')).toBe(0)
    expect(toUnixSeconds('')).toBe(0)
  })
})
