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
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { describe, test } from 'node:test'
import { fileURLToPath } from 'node:url'
import {
  getInvitationCredentials,
  syncInvitationCredentialsFromSearch,
} from './lib/storage.ts'

const root = dirname(fileURLToPath(import.meta.url))
const readDefaultSource = (...parts: string[]) =>
  readFileSync(resolve(root, ...parts), 'utf8')
const readClassicSource = (...parts: string[]) =>
  readFileSync(
    resolve(root, '..', '..', '..', '..', 'classic', 'src', ...parts),
    'utf8'
  )

function createMemoryStorage(): Storage {
  const values = new Map<string, string>()
  return {
    get length() {
      return values.size
    },
    clear() {
      values.clear()
    },
    getItem(key) {
      return values.get(key) ?? null
    },
    key(index) {
      return [...values.keys()][index] ?? null
    },
    removeItem(key) {
      values.delete(key)
    },
    setItem(key, value) {
      values.set(key, String(value))
    },
  }
}

function withBrowserStorage(
  run: (storage: {
    local: Storage
    session: Storage
    replacedUrls: string[]
  }) => void
) {
  const local = createMemoryStorage()
  const session = createMemoryStorage()
  const replacedUrls: string[] = []
  const originalWindow = Object.getOwnPropertyDescriptor(globalThis, 'window')
  Object.defineProperty(globalThis, 'window', {
    configurable: true,
    value: {
      localStorage: local,
      sessionStorage: session,
      location: { pathname: '/sign-up', hash: '#form' },
      history: {
        state: null,
        replaceState: (
          _state: unknown,
          _title: string,
          url?: string | URL | null
        ) => {
          replacedUrls.push(String(url ?? ''))
        },
      },
    },
  })

  try {
    run({ local, session, replacedUrls })
  } finally {
    if (originalWindow) {
      Object.defineProperty(globalThis, 'window', originalWindow)
    } else {
      Reflect.deleteProperty(globalThis, 'window')
    }
  }
}

describe('invitation-only registration frontends', () => {
  test('exposes the independent option in both system settings themes', () => {
    const defaultSection = readDefaultSource(
      '..',
      'system-settings',
      'auth',
      'basic-auth-section.tsx'
    )
    const classicSettings = readClassicSource(
      'components',
      'settings',
      'SystemSetting.jsx'
    )
    const updateOptionHook = readDefaultSource(
      '..',
      'system-settings',
      'hooks',
      'use-update-option.ts'
    )

    assert.match(defaultSection, /InvitationRegisterEnabled/)
    assert.match(defaultSection, /Invitation Registration/)
    assert.match(classicSettings, /InvitationRegisterEnabled/)
    assert.match(updateOptionHook, /'RegisterEnabled'/)
    assert.match(updateOptionHook, /'PasswordRegisterEnabled'/)
    assert.match(updateOptionHook, /'InvitationRegisterEnabled'/)
  })

  test('keeps public sign-up links hidden when public registration is off', () => {
    const defaultSignIn = readDefaultSource('sign-in', 'index.tsx')
    const classicSignIn = readClassicSource(
      'components',
      'auth',
      'LoginForm.jsx'
    )

    assert.match(defaultSignIn, /status\?\.register_enabled !== false/)
    assert.equal(
      classicSignIn.match(/status\.register_enabled !== false/g)?.length,
      2
    )
  })

  test('stores only the short affiliate code and removes legacy signature parameters', () => {
    withBrowserStorage(({ local, session, replacedUrls }) => {
      local.setItem('aff', 'legacy-aff')

      assert.deepEqual(
        syncInvitationCredentialsFromSearch(
          '?aff=inviter&invite=signature&redirect=%2Fconsole'
        ),
        { aff: 'inviter' }
      )
      assert.equal(session.getItem('aff'), 'inviter')
      assert.equal(session.getItem('invite'), null)
      assert.equal(local.getItem('aff'), null)
      assert.equal(replacedUrls[0], '/sign-up?redirect=%2Fconsole#form')

      assert.deepEqual(syncInvitationCredentialsFromSearch('?aff=legacy'), {
        aff: 'legacy',
      })
      assert.deepEqual(getInvitationCredentials(), {
        aff: 'legacy',
      })
      assert.equal(session.getItem('aff'), 'legacy')
      assert.equal(session.getItem('invite'), null)

      session.setItem('aff', 'stale-aff')
      session.setItem('invite', 'stale-signature')
      assert.equal(syncInvitationCredentialsFromSearch(''), null)
      assert.equal(getInvitationCredentials(), null)
    })
  })

  test('submits only the affiliate code and clears it only after success', () => {
    const defaultSignUp = readDefaultSource(
      'sign-up',
      'components',
      'sign-up-form.tsx'
    )
    const classicSignUp = readClassicSource(
      'components',
      'auth',
      'RegisterForm.jsx'
    )

    assert.match(defaultSignUp, /aff_code:\s*invitation\?\.aff \?\? ''/)
    assert.doesNotMatch(defaultSignUp, /invite:\s*invitation/)
    assert.match(
      defaultSignUp,
      /if \(res\?\.success\) \{\s*clearInvitationCredentials\(\)/
    )
    assert.match(classicSignUp, /aff_code:\s*invitation\?\.aff \|\| ''/)
    assert.doesNotMatch(classicSignUp, /invite:\s*invitation/)
    assert.match(
      classicSignUp,
      /if \(success\) \{\s*clearInvitationCredentials\(\)/
    )
  })

  test('sends only the affiliate code while establishing OAuth state', () => {
    const defaultApi = readDefaultSource('api.ts')
    const classicApi = readClassicSource('helpers', 'api.js')
    const defaultSignUp = readDefaultSource(
      'sign-up',
      'components',
      'sign-up-form.tsx'
    )
    const defaultSignIn = readDefaultSource(
      'sign-in',
      'components',
      'user-auth-form.tsx'
    )
    const classicSignUp = readClassicSource(
      'components',
      'auth',
      'RegisterForm.jsx'
    )
    const classicSignIn = readClassicSource(
      'components',
      'auth',
      'LoginForm.jsx'
    )

    for (const apiSource of [defaultApi, classicApi]) {
      assert.match(apiSource, /aff:\s*invitation\?\.aff/)
      assert.doesNotMatch(apiSource, /invite:\s*invitation/)
      assert.match(
        apiSource,
        /if \(.*success.*data.*\) \{\s*clearInvitationCredentials\(\)/s
      )
    }
    for (const formSource of [
      defaultSignUp,
      defaultSignIn,
      classicSignUp,
      classicSignIn,
    ]) {
      assert.match(formSource, /const state = await getOAuthState\(\)/)
      assert.match(formSource, /if \(!state\)/)
    }
  })

  test('initializes both auth entry pages and removes the legacy root capture', () => {
    const defaultSignUp = readDefaultSource(
      'sign-up',
      'components',
      'sign-up-form.tsx'
    )
    const defaultSignIn = readDefaultSource('sign-in', 'index.tsx')
    const defaultRoot = readDefaultSource('..', '..', 'routes', '__root.tsx')
    const classicSignUp = readClassicSource(
      'components',
      'auth',
      'RegisterForm.jsx'
    )
    const classicSignIn = readClassicSource(
      'components',
      'auth',
      'LoginForm.jsx'
    )
    const classicInvitation = readClassicSource('helpers', 'invitation.js')

    for (const entrySource of [
      defaultSignUp,
      defaultSignIn,
      classicSignUp,
      classicSignIn,
    ]) {
      assert.match(
        entrySource,
        /syncInvitationCredentialsFromSearch\(window\.location\.search\)/
      )
    }
    assert.doesNotMatch(defaultRoot, /saveAffiliateCode|location\.search/)
    assert.match(classicInvitation, /window\.sessionStorage\.setItem/)
    assert.match(classicInvitation, /window\.history\.replaceState/)
    assert.match(defaultSignUp, /syncInvitationCredentialsFromSearch/)
    assert.doesNotMatch(classicInvitation, /localStorage\.getItem/)
    assert.doesNotMatch(defaultSignUp, /localStorage.*['"]aff['"]/)
    assert.doesNotMatch(classicSignUp, /localStorage.*['"]aff['"]/)
  })
})
