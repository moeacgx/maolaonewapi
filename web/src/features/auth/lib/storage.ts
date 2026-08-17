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
/** Utilities for tab-scoped invitation credentials. */

const INVITATION_STORAGE_KEYS = {
  AFFILIATE: 'aff',
  LEGACY_INVITE: 'invite',
} as const

export type InvitationCredentials = {
  aff: string
}

/** Clear current-tab credentials and legacy persistent invitation residue. */
export function clearInvitationCredentials(): void {
  if (typeof window === 'undefined') return

  try {
    window.sessionStorage.removeItem(INVITATION_STORAGE_KEYS.AFFILIATE)
    window.sessionStorage.removeItem(INVITATION_STORAGE_KEYS.LEGACY_INVITE)
  } catch {
    /* Storage can be unavailable in restricted browser contexts. */
  }

  try {
    window.localStorage.removeItem(INVITATION_STORAGE_KEYS.AFFILIATE)
    window.localStorage.removeItem(INVITATION_STORAGE_KEYS.LEGACY_INVITE)
  } catch {
    /* Storage can be unavailable in restricted browser contexts. */
  }
}

export function saveInvitationCredentials(
  aff: string
): InvitationCredentials | null {
  if (typeof window === 'undefined') return null

  const normalizedAff = aff.trim()
  clearInvitationCredentials()
  if (!normalizedAff) return null

  try {
    window.sessionStorage.setItem(
      INVITATION_STORAGE_KEYS.AFFILIATE,
      normalizedAff
    )
    return { aff: normalizedAff }
  } catch {
    clearInvitationCredentials()
    return null
  }
}

export function getInvitationCredentials(): InvitationCredentials | null {
  if (typeof window === 'undefined') return null

  try {
    const aff =
      window.sessionStorage
        .getItem(INVITATION_STORAGE_KEYS.AFFILIATE)
        ?.trim() ?? ''
    if (!aff) {
      clearInvitationCredentials()
      return null
    }
    return { aff }
  } catch {
    clearInvitationCredentials()
    return null
  }
}

/** Capture invitation data from an auth entry URL and scrub it from history. */
export function syncInvitationCredentialsFromSearch(
  search: string
): InvitationCredentials | null {
  const params = new URLSearchParams(search)
  const hasAffQuery = params.has('aff')
  const hasLegacyInviteQuery = params.has('invite')

  if (!hasAffQuery && !hasLegacyInviteQuery) {
    return getInvitationCredentials()
  }

  const credentials = hasAffQuery
    ? saveInvitationCredentials(params.get('aff') ?? '')
    : getInvitationCredentials()
  if (typeof window !== 'undefined') {
    params.delete('aff')
    params.delete('invite')
    const remainingQuery = params.toString()
    const sanitizedUrl = `${window.location.pathname}${remainingQuery ? `?${remainingQuery}` : ''}${window.location.hash}`
    window.history.replaceState(window.history.state, '', sanitizedUrl)
  }

  return credentials
}
