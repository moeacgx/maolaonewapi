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

import { createPlaygroundGroupOptions } from '../group-options'

describe('playground group names', () => {
  test('shows the current name while retaining the group code as value', () => {
    expect(
      createPlaygroundGroupOptions({
        'Codex-Team': {
          name: 'Codex Benefits',
          desc: 'Priority route',
          ratio: 0.05,
        },
      })
    ).toEqual([
      {
        label: 'Codex Benefits',
        value: 'Codex-Team',
        ratio: 0.05,
        desc: 'Priority route',
      },
    ])
  })

  test('falls back to the code and suppresses duplicate descriptions', () => {
    expect(
      createPlaygroundGroupOptions({
        default: { name: 'default', desc: 'default', ratio: 1 },
      })
    ).toEqual([{ label: 'default', value: 'default', ratio: 1 }])
  })
})
