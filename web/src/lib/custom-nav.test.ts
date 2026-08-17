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
  getCustomNavIcon,
  getSidebarCustomModuleKey,
  parseCustomNavItems,
  parseTopNavCustomItems,
} from './custom-nav'

describe('custom navigation helpers', () => {
  test('validates URLs, whitelists icons, and orders enabled items', () => {
    const items = parseCustomNavItems([
      {
        id: 'canvas',
        title: 'Canvas',
        url: '/canvas',
        icon: 'Brush',
        order: 20,
        section: 'chat',
      },
      {
        id: 'script',
        title: 'Script',
        url: 'javascript:alert(1)',
        order: 1,
      },
      {
        id: 'docs',
        title: 'Docs',
        url: 'https://docs.example.com',
        icon: 'NotARealIcon',
        order: 5,
      },
      {
        id: 'disabled',
        title: 'Disabled',
        url: '/disabled',
        enabled: false,
        order: 0,
      },
    ])

    expect(items.map((item) => item.id)).toEqual(['docs', 'canvas'])
    expect(items[0]).toMatchObject({
      external: true,
      openInNewTab: true,
      icon: undefined,
    })
    expect(items[1]).toMatchObject({
      external: false,
      section: 'chat',
      icon: 'Brush',
    })
    expect(getCustomNavIcon('Brush')).toBeDefined()
    expect(getCustomNavIcon('NotARealIcon')).toBeUndefined()
  })

  test('normalizes stable sidebar keys', () => {
    expect(getSidebarCustomModuleKey('工具 1')).toBe('custom:工具-1')
    expect(getSidebarCustomModuleKey('  report / daily  ')).toBe(
      'custom:report-daily'
    )
  })

  test('gives dedicated header entries precedence over placed duplicates', () => {
    const items = parseTopNavCustomItems(
      [
        {
          id: 'shared item',
          title: 'Header managed',
          url: '/header',
          order: 20,
        },
      ],
      [
        {
          id: 'first',
          title: 'First',
          url: '/first',
          order: 1,
          section: 'header',
        },
        {
          id: 'shared item',
          title: 'Duplicate',
          url: '/duplicate',
          order: 0,
          section: 'header',
        },
        {
          id: 'sidebar',
          title: 'Sidebar only',
          url: '/sidebar',
          order: 2,
          section: 'personal',
        },
      ]
    )

    expect(items.map(({ id, url }) => [id, url])).toEqual([
      ['first', '/first'],
      ['shared-item', '/header'],
    ])
  })
})
