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

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { Route } from './index'

type BeforeLoadContext = Parameters<
  NonNullable<(typeof Route.options)['beforeLoad']>
>[0]

function runBeforeLoad(params: { moduleId: string; pageKey: string }) {
  return Route.options.beforeLoad?.({ params } as BeforeLoadContext)
}

function captureRedirect(params: { moduleId: string; pageKey: string }) {
  try {
    runBeforeLoad(params)
  } catch (error) {
    return error
  }
  throw new Error('Expected route guard to redirect')
}

describe('deep extension route authorization', () => {
  beforeEach(() => {
    useAuthStore.getState().auth.reset()
  })

  test('redirects a non-root deep link to the forbidden page', () => {
    useAuthStore.getState().auth.setUser({
      id: 2,
      username: 'admin',
      role: ROLE.ADMIN,
    })

    expect(
      captureRedirect({ moduleId: 'channel-quality', pageKey: 'index' })
    ).toMatchObject({ options: { to: '/403' } })
  })

  test('allows a root user to navigate to a valid extension page', () => {
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'root',
      role: ROLE.SUPER_ADMIN,
    })

    expect(
      runBeforeLoad({ moduleId: 'channel-quality', pageKey: 'index' })
    ).toBeUndefined()
  })

  test('keeps the existing invalid-parameter redirect for root users', () => {
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'root',
      role: ROLE.SUPER_ADMIN,
    })

    expect(captureRedirect({ moduleId: '', pageKey: '' })).toMatchObject({
      options: { to: '/extensions' },
    })
  })
})
