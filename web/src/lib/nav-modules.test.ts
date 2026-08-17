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
  DEFAULT_CANVAS_APP_ORIGIN,
  getCanvasSettingsFromSidebarModules,
} from './canvas-settings'
import {
  parseHeaderNavModules,
  parseSidebarModulesFromStatus,
} from './nav-modules'

describe('navigation status parsing', () => {
  test('preserves custom header and sidebar item payloads', () => {
    const header = parseHeaderNavModules(
      JSON.stringify({
        home: false,
        customItems: [{ id: 'docs', title: 'Docs', url: '/docs' }],
      })
    )
    const sidebar = parseSidebarModulesFromStatus({
      SidebarModulesAdmin: JSON.stringify({
        chat: { canvas: true },
        customItems: [
          {
            id: 'placed',
            title: 'Placed',
            url: '/placed',
            section: 'header',
          },
        ],
      }),
    })

    expect(header.home).toBe(false)
    expect(header.customItems[0]?.id).toBe('docs')
    expect(sidebar.customItems[0]?.section).toBe('header')
  })

  test('normalizes canvas origin and icon from sidebar status', () => {
    const raw = JSON.stringify({
      chat: {
        canvasOrigin: 'canvas.example.com/a/path',
        canvasIcon: 'Sparkles',
      },
    })
    const settings = getCanvasSettingsFromSidebarModules(raw)
    const parsed = parseSidebarModulesFromStatus({
      SidebarModulesAdmin: raw,
    })

    expect(settings).toEqual({
      canvasOrigin: 'https://canvas.example.com',
      canvasIcon: 'Sparkles',
    })
    expect(parsed.chat).toMatchObject(settings)
  })

  test('rejects unsafe canvas origins', () => {
    expect(
      getCanvasSettingsFromSidebarModules(
        JSON.stringify({ chat: { canvasOrigin: 'javascript:alert(1)' } })
      ).canvasOrigin
    ).toBe(DEFAULT_CANVAS_APP_ORIGIN)
  })
})
