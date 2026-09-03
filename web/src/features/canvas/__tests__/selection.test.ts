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
import { describe, expect, it } from 'vitest'

import { resolveCanvasDefaultGroup } from '../lib'

describe('resolveCanvasDefaultGroup', () => {
  const groups = [
    { label: '默认', value: 'default', ratio: 1 },
    { label: 'VIP', value: 'vip', ratio: 0.8 },
  ]

  it('uses the configured group when it is available to the user', () => {
    expect(resolveCanvasDefaultGroup(groups, 'vip')).toBe('vip')
  })

  it('falls back to default or the first available group when configured group is unavailable', () => {
    expect(resolveCanvasDefaultGroup(groups, 'missing')).toBe('default')
    expect(resolveCanvasDefaultGroup(groups.slice(1), 'missing')).toBe('vip')
  })

  it('returns an empty selection when no groups are available', () => {
    expect(resolveCanvasDefaultGroup([], 'vip')).toBe('')
  })
})
