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
  buildCCSwitchURL,
  isCCSwitchPreset,
  isValidCCSwitchAddress,
  normalizeCCSwitchAddress,
  resolveCCSwitchAddresses,
} from '../cc-switch'

function parseImportURL(url: string) {
  return new URLSearchParams(url.slice(url.indexOf('?') + 1))
}

describe('CC Switch routing', () => {
  test('normalizes and validates hardened absolute HTTP addresses', () => {
    expect(normalizeCCSwitchAddress(' https://api.example.com/// ')).toBe(
      'https://api.example.com'
    )
    expect(isValidCCSwitchAddress('https://api.example.com/gateway')).toBe(true)
    expect(isValidCCSwitchAddress('https://user:pass@example.com')).toBe(false)
    expect(isValidCCSwitchAddress('https://api.example.com?token=x')).toBe(
      false
    )
    expect(isValidCCSwitchAddress('javascript:alert(1)')).toBe(false)
  })

  test('uses a valid API override while preserving the public homepage', () => {
    expect(
      resolveCCSwitchAddresses(
        {
          cc_switch_api_address: 'https://api.example.com/',
          server_address: 'https://www.example.com/',
        },
        'https://fallback.example.com'
      )
    ).toEqual({
      apiAddress: 'https://api.example.com',
      homepage: 'https://www.example.com',
    })
  })

  test('rejects an unsafe override and idempotently appends Codex /v1', () => {
    const common = {
      name: 'Provider',
      models: { model: 'gpt-test' },
      apiKey: 'sk-test',
      status: {
        cc_switch_api_address: 'https://user:pass@api.example.com',
        server_address: 'https://www.example.com/',
      },
      origin: 'https://fallback.example.com',
    }
    expect(
      parseImportURL(buildCCSwitchURL({ ...common, app: 'codex' })).get(
        'endpoint'
      )
    ).toBe('https://www.example.com/v1')
    expect(
      parseImportURL(
        buildCCSwitchURL({
          ...common,
          app: 'codex',
          status: { cc_switch_api_address: 'https://api.example.com/v1/' },
        })
      ).get('endpoint')
    ).toBe('https://api.example.com/v1')
  })

  test('recognizes only explicit CC Switch preset schemes', () => {
    expect(isCCSwitchPreset('ccswitch')).toBe(true)
    expect(isCCSwitchPreset('CCSwitch://open')).toBe(true)
    expect(isCCSwitchPreset('https://ccswitch.io')).toBe(false)
  })

  test('declares WebSocket support only for Codex imports', () => {
    const common = {
      name: 'Provider',
      models: { model: 'gpt-test' },
      apiKey: 'sk-test',
      origin: 'https://api.example.com',
    }
    expect(
      parseImportURL(buildCCSwitchURL({ ...common, app: 'codex' })).get(
        'supports_websockets'
      )
    ).toBe('true')
    expect(
      parseImportURL(buildCCSwitchURL({ ...common, app: 'claude' })).has(
        'supports_websockets'
      )
    ).toBe(false)
    expect(
      parseImportURL(buildCCSwitchURL({ ...common, app: 'gemini' })).has(
        'supports_websockets'
      )
    ).toBe(false)
  })
})
