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

import { normalizeNotificationList, takeRecentNotifications } from './list'

describe('notification API list contracts', () => {
  test('accepts both direct arrays and paged item envelopes', () => {
    expect(normalizeNotificationList([1, 2])).toEqual([1, 2])
    expect(normalizeNotificationList({ items: [3, 4] })).toEqual([3, 4])
    expect(normalizeNotificationList(undefined)).toEqual([])
  })

  test('limits the center to five recent deliveries without mutating input', () => {
    const deliveries = [1, 2, 3, 4, 5, 6]
    expect(takeRecentNotifications(deliveries)).toEqual([1, 2, 3, 4, 5])
    expect(deliveries).toEqual([1, 2, 3, 4, 5, 6])
  })
})
