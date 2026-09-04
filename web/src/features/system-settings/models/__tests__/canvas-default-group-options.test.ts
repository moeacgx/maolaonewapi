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

import { buildCanvasDefaultGroupOptions } from '../canvas-default-group-options'

describe('buildCanvasDefaultGroupOptions', () => {
  const groups = [
    {
      id: 1,
      code: 'default',
      name: '默认分组',
      description: '',
      ratio: 1,
      user_selectable: true,
      exclusive: false,
      status: 1,
      auto_enabled: true,
      auto_order: 1,
    },
    {
      id: 2,
      code: 'disabled',
      name: '停用分组',
      description: '',
      ratio: 1,
      user_selectable: true,
      exclusive: false,
      status: 0,
      auto_enabled: false,
      auto_order: 0,
    },
  ]

  it('places selectable auto before active physical groups', () => {
    expect(
      buildCanvasDefaultGroupOptions(groups, {
        user_selectable: true,
        description: '自动选择可用分组',
      })
    ).toEqual([
      { value: 'auto', label: 'Automatic selection' },
      { value: 'default', label: '默认分组 (default)' },
    ])
  })

  it('omits auto when users cannot select it', () => {
    expect(
      buildCanvasDefaultGroupOptions(groups, {
        user_selectable: false,
        description: '',
      })
    ).toEqual([{ value: 'default', label: '默认分组 (default)' }])
  })
})
