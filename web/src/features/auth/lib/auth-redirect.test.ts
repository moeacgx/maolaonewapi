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
import { beforeEach, describe, expect, test } from 'vitest'

import type { AuthUser } from '@/stores/auth-store'

import {
  consumeExternalAuthRedirect,
  getSavedLanguage,
  sanitizeAuthRedirect,
  saveExternalAuthRedirect,
} from './auth-redirect'

const origin = 'https://dashboard.example.com'

beforeEach(() => {
  window.sessionStorage.clear()
})

describe('authentication redirect validation', () => {
  test('preserves safe internal paths, search parameters, and fragments', () => {
    expect(sanitizeAuthRedirect('/console?tab=usage#recent', origin)).toBe(
      '/console?tab=usage#recent'
    )
    expect(
      sanitizeAuthRedirect(
        'https://dashboard.example.com/dashboard?tab=quota#daily',
        origin
      )
    ).toBe('/dashboard?tab=quota#daily')
  })

  test('rejects external and ambiguously parsed redirect targets', () => {
    const unsafeTargets: unknown[] = [
      undefined,
      '',
      'dashboard',
      '//attacker.example/path',
      'https://attacker.example/path',
      'javascript:alert(1)',
      '/\\attacker.example/path',
      'https:\\attacker.example/path',
    ]

    for (const target of unsafeTargets) {
      expect(sanitizeAuthRedirect(target, origin)).toBe(null)
    }
  })

  test('rejects invalid or non-HTTP application origins', () => {
    expect(sanitizeAuthRedirect('/dashboard', 'not-an-origin')).toBe(null)
    expect(sanitizeAuthRedirect('/dashboard', 'file:///tmp/app')).toBe(null)
  })
})

describe('external authentication redirect storage', () => {
  test('stores a validated HTTP(S) target per tab and consumes it once', () => {
    expect(
      saveExternalAuthRedirect(
        ' https://docs.example.com/start?from=nav#guide '
      )
    ).toBe('https://docs.example.com/start?from=nav#guide')

    expect(consumeExternalAuthRedirect()).toBe(
      'https://docs.example.com/start?from=nav#guide'
    )
    expect(consumeExternalAuthRedirect()).toBeNull()

    expect(saveExternalAuthRedirect('http://legacy.example.com/path')).toBe(
      'http://legacy.example.com/path'
    )
    expect(consumeExternalAuthRedirect()).toBe('http://legacy.example.com/path')
  })

  test('fails closed and clears malformed or non-HTTP stored targets', () => {
    const invalidTargets = [
      '/dashboard',
      '//attacker.example/path',
      'javascript:alert(1)',
      'https:\\attacker.example/path',
      'not a URL',
    ]

    for (const target of invalidTargets) {
      window.sessionStorage.setItem('external-auth-redirect', target)
      expect(consumeExternalAuthRedirect()).toBeNull()
      expect(consumeExternalAuthRedirect()).toBeNull()
    }
  })

  test('rejects an invalid configured target and clears prior state', () => {
    saveExternalAuthRedirect('https://docs.example.com/private')

    expect(saveExternalAuthRedirect('javascript:alert(1)')).toBeNull()
    expect(consumeExternalAuthRedirect()).toBeNull()
  })

  test('does not weaken internal redirect validation for external targets', () => {
    saveExternalAuthRedirect('https://docs.example.com/private')

    expect(
      sanitizeAuthRedirect(
        'https://docs.example.com/private',
        'https://dashboard.example.com'
      )
    ).toBeNull()
    expect(consumeExternalAuthRedirect()).toBe(
      'https://docs.example.com/private'
    )
  })
})

describe('saved authentication language', () => {
  const user: AuthUser = { id: 1, username: 'user', role: 1 }

  test('prefers the explicit user language', () => {
    expect(
      getSavedLanguage({
        ...user,
        language: 'ja',
        setting: { language: 'fr' },
      })
    ).toBe('ja')
  })

  test('reads object and JSON string settings', () => {
    expect(getSavedLanguage({ ...user, setting: { language: 'fr' } })).toBe(
      'fr'
    )
    expect(getSavedLanguage({ ...user, setting: '{"language":"ru"}' })).toBe(
      'ru'
    )
  })

  test('ignores malformed and non-string setting languages', () => {
    expect(getSavedLanguage({ ...user, setting: '{' })).toBe(undefined)
    expect(getSavedLanguage({ ...user, setting: { language: 123 } })).toBe(
      undefined
    )
  })
})
