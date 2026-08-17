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

import { buildCanvasLaunchUrl } from './lib'

describe('buildCanvasLaunchUrl', () => {
  test('builds a New API session launch URL without an API key', () => {
    const url = buildCanvasLaunchUrl({
      canvasOrigin: 'https://canvas.maolaoapi.com',
      newApiOrigin: 'https://maolaoapi.com',
      group: 'vip group',
    })

    expect(url).toBe(
      'https://canvas.maolaoapi.com/?mode=newapi&baseUrl=https%3A%2F%2Fmaolaoapi.com%2Fcanvas&group=vip+group'
    )
    expect(url).not.toContain('apiKey')
  })

  test('accepts a configured bare canvas domain', () => {
    expect(
      buildCanvasLaunchUrl({
        canvasOrigin: 'canvas.example.com',
        newApiOrigin: 'https://maolaoapi.com/',
        group: 'default',
      })
    ).toBe(
      'https://canvas.example.com/?mode=newapi&baseUrl=https%3A%2F%2Fmaolaoapi.com%2Fcanvas&group=default'
    )
  })
})
