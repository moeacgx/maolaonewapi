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
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { createOAuthFlow, register } from './api'
import {
  clearInvitationCredentials,
  getInvitationCredentials,
  saveInvitationCredentials,
  syncInvitationCredentialsFromSearch,
} from './lib/storage'

const registration = {
  username: 'invitee',
  password: 'password-123',
  aff_code: '',
}

describe('invitation registration credentials', () => {
  beforeEach(() => {
    window.localStorage.clear()
    window.sessionStorage.clear()
    window.history.replaceState(null, '', '/sign-up')
  })

  test('stores credentials per tab, removes legacy persistence, and scrubs URL', () => {
    window.localStorage.setItem('aff', 'legacy-aff')
    window.localStorage.setItem('invite', 'legacy-invite')
    window.history.replaceState(
      { source: 'test' },
      '',
      '/sign-up?aff=%20campaign-7%20&invite=obsolete&redirect=%2Fpricing#form'
    )

    const result = syncInvitationCredentialsFromSearch(window.location.search)

    expect(result).toEqual({ aff: 'campaign-7' })
    expect(getInvitationCredentials()).toEqual({ aff: 'campaign-7' })
    expect(window.localStorage.getItem('aff')).toBeNull()
    expect(window.localStorage.getItem('invite')).toBeNull()
    expect(
      `${window.location.pathname}${window.location.search}${window.location.hash}`
    ).toBe('/sign-up?redirect=%2Fpricing#form')
  })

  test('keeps captured credentials when the scrubbed page reloads', () => {
    window.history.replaceState(null, '', '/sign-up?aff=reload-campaign')

    expect(syncInvitationCredentialsFromSearch(window.location.search)).toEqual(
      { aff: 'reload-campaign' }
    )
    expect(window.location.search).toBe('')

    expect(syncInvitationCredentialsFromSearch(window.location.search)).toEqual(
      { aff: 'reload-campaign' }
    )
    expect(getInvitationCredentials()).toEqual({ aff: 'reload-campaign' })
  })

  test('scrubs a legacy query without clearing the captured affiliate', () => {
    saveInvitationCredentials('preserved-campaign')
    window.history.replaceState(
      null,
      '',
      '/sign-up?invite=legacy-value&redirect=%2Fpricing'
    )

    expect(syncInvitationCredentialsFromSearch(window.location.search)).toEqual(
      { aff: 'preserved-campaign' }
    )
    expect(getInvitationCredentials()).toEqual({ aff: 'preserved-campaign' })
    expect(`${window.location.pathname}${window.location.search}`).toBe(
      '/sign-up?redirect=%2Fpricing'
    )
  })

  test('sends the captured code on registration and clears only after success', async () => {
    saveInvitationCredentials('campaign-8')
    const post = vi.spyOn(api, 'post').mockResolvedValueOnce({
      data: { success: false, message: 'try again' },
    } as never)

    await register(registration)
    expect(post).toHaveBeenLastCalledWith(
      '/api/user/register',
      expect.objectContaining({ aff_code: 'campaign-8' }),
      { params: { turnstile: '' } }
    )
    expect(getInvitationCredentials()).toEqual({ aff: 'campaign-8' })

    post.mockResolvedValueOnce({
      data: { success: true, message: '' },
    } as never)
    await register(registration)
    expect(getInvitationCredentials()).toBeNull()
  })

  test('adds invitation data to login OAuth state and clears it after issuance', async () => {
    saveInvitationCredentials('oauth-campaign')
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, data: { flow_token: 'flow-1' } },
    } as never)

    await expect(createOAuthFlow('github', 'login')).resolves.toBe('flow-1')
    expect(post).toHaveBeenCalledWith(
      '/api/oauth/state',
      {
        provider: 'github',
        intent: 'login',
        aff: 'oauth-campaign',
      },
      { skipAuthRefresh: true }
    )
    expect(getInvitationCredentials()).toBeNull()
  })

  test('does not attach or consume invitation data for account binding', async () => {
    saveInvitationCredentials('keep-for-registration')
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, data: 'bind-flow' },
    } as never)

    await expect(createOAuthFlow('github', 'bind')).resolves.toBe('bind-flow')
    expect(post).toHaveBeenCalledWith(
      '/api/oauth/state',
      { provider: 'github', intent: 'bind', aff: undefined },
      { skipAuthRefresh: false }
    )
    expect(getInvitationCredentials()).toEqual({ aff: 'keep-for-registration' })

    clearInvitationCredentials()
  })
})
