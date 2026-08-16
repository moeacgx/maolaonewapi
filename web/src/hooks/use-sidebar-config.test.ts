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

import type { NavGroup } from '@/components/layout/types'
import { getSidebarCustomModuleKey } from '@/lib/custom-nav'

import {
  filterSidebarNavGroups,
  isModuleEnabled,
  parseSidebarConfig,
} from './use-sidebar-config'

const customKey = getSidebarCustomModuleKey('工具 1')
const customGroups: NavGroup[] = [
  {
    title: 'Custom',
    items: [
      {
        title: 'Tool',
        url: '/tools',
        configUrls: [customKey],
      },
    ],
  },
]

describe('sidebar configuration filtering', () => {
  test('combines administrator and user custom settings with false precedence', () => {
    const adminAllows = { custom: { enabled: true, [customKey]: true } }
    const adminDenies = { custom: { enabled: true, [customKey]: false } }
    const userAllows = { custom: { enabled: true, [customKey]: true } }
    const userDenies = { custom: { enabled: true, [customKey]: false } }

    expect(
      filterSidebarNavGroups(customGroups, adminAllows, userDenies)
    ).toEqual([])
    expect(
      filterSidebarNavGroups(customGroups, adminDenies, userAllows)
    ).toEqual([])
    expect(
      filterSidebarNavGroups(customGroups, adminAllows, userAllows)
    ).toHaveLength(1)
  })

  test('uses trailing-slash normalization and longest route prefix', () => {
    const admin = parseSidebarConfig(
      JSON.stringify({
        console: { game: false },
        admin: { setting: true, affiliate_admin: false },
      })
    )

    expect(isModuleEnabled('/game-center/', admin, null)).toBe(false)
    expect(
      isModuleEnabled('/system-settings/billing/affiliate/payouts', admin, null)
    ).toBe(false)
    expect(isModuleEnabled('/system-settings/security', admin, null)).toBe(true)
  })

  test('filters collapsible subitems by each subitem configUrls', () => {
    const groups: NavGroup[] = [
      {
        title: 'Workspace',
        items: [
          {
            title: 'Nested',
            items: [
              {
                title: 'Prediction',
                url: '/game-center/prediction/42',
                configUrls: ['/game-center/prediction/42'],
              },
              {
                title: 'Settings',
                url: '/system-settings/site',
                configUrls: ['/system-settings/site'],
              },
            ],
          },
        ],
      },
    ]
    const admin = parseSidebarConfig(
      JSON.stringify({ console: { game: false }, admin: { setting: true } })
    )
    const filtered = filterSidebarNavGroups(groups, admin, null)

    expect(filtered[0]?.items[0]).toMatchObject({
      items: [{ title: 'Settings' }],
    })
  })

  test('applies backend permission narrowing before user preferences', () => {
    const admin = parseSidebarConfig(undefined)
    const user = { admin: { enabled: true, invoice_admin: true } }

    expect(
      isModuleEnabled('/invoice-management', admin, user, {
        admin: { invoice_admin: false },
      })
    ).toBe(false)
  })
})
