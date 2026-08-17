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

import { getExtensionPageUrl, getNativeExtensionAssetUrl } from './urls'

describe('extension host URLs', () => {
  test('keeps proxy pages same-origin and version-pins static modules', () => {
    expect(
      getExtensionPageUrl(
        'risk module',
        'dashboard?range=7d#errors',
        '1.2.0+build'
      )
    ).toBe(
      '/api/extensions/risk%20module/proxy/dashboard?range=7d&module_version=1.2.0%2Bbuild#errors'
    )
    expect(getExtensionPageUrl('module', 'https://evil.example/page')).toBe(
      '/api/extensions/module/proxy/https://evil.example/page'
    )
  })

  test('does not duplicate a manifest-provided version query', () => {
    expect(
      getExtensionPageUrl(
        'module',
        '/page?module_version=server-pinned',
        'client-version'
      )
    ).toBe('/api/extensions/module/proxy/page?module_version=server-pinned')
  })

  test('encodes native asset identities and cache-busting values', () => {
    expect(
      getNativeExtensionAssetUrl(
        'module/one',
        'overview detail',
        '2.0.0',
        'sha:123',
        'style-1',
        2
      )
    ).toBe(
      '/api/extensions/module%2Fone/native/overview%20detail/default/style-1?module_version=2.0.0&asset_revision=sha%3A123&load_attempt=2'
    )
  })
})
