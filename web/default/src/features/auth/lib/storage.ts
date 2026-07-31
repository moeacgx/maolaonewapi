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
/**
 * Utilities for managing authentication-related browser storage
 */

// ============================================================================
// LocalStorage Keys
// ============================================================================

const STORAGE_KEYS = {
  USER_ID: 'uid',
  STATUS: 'status',
} as const

const INVITATION_STORAGE_KEYS = {
  AFFILIATE: 'aff',
} as const

export type InvitationCredentials = {
  aff: string
}

// ============================================================================
// User ID Storage
// ============================================================================

/**
 * Save user ID to localStorage
 */
export function saveUserId(userId: number | string): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(STORAGE_KEYS.USER_ID, String(userId))
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save user ID:', error)
  }
}

/**
 * Get user ID from localStorage
 */
export function getUserId(): string | null {
  if (typeof window === 'undefined') return null
  try {
    return window.localStorage.getItem(STORAGE_KEYS.USER_ID)
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to get user ID:', error)
    return null
  }
}

/**
 * Remove user ID from localStorage
 */
export function removeUserId(): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.removeItem(STORAGE_KEYS.USER_ID)
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to remove user ID:', error)
  }
}

// ============================================================================
// 邀请注册临时存储
// ============================================================================

/** 清理当前标签页的邀请凭证及旧版持久化残留。 */
export function clearInvitationCredentials(): void {
  if (typeof window === 'undefined') return

  try {
    window.sessionStorage.removeItem(INVITATION_STORAGE_KEYS.AFFILIATE)
    window.sessionStorage.removeItem('invite')
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to clear invitation session:', error)
  }

  try {
    window.localStorage.removeItem(INVITATION_STORAGE_KEYS.AFFILIATE)
    window.localStorage.removeItem('invite')
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to clear legacy invitation storage:', error)
  }
}

/** 保存当前注册流程的邀请码。 */
export function saveInvitationCredentials(
  aff: string
): InvitationCredentials | null {
  if (typeof window === 'undefined') return null

  const credentials = {
    aff: aff.trim(),
  }
  clearInvitationCredentials()
  if (!credentials.aff) return null

  try {
    window.sessionStorage.setItem(
      INVITATION_STORAGE_KEYS.AFFILIATE,
      credentials.aff
    )
    return credentials
  } catch (error) {
    clearInvitationCredentials()
    // eslint-disable-next-line no-console
    console.error('Failed to save invitation session:', error)
    return null
  }
}

/** 读取当前标签页的邀请码。 */
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
  } catch (error) {
    clearInvitationCredentials()
    // eslint-disable-next-line no-console
    console.error('Failed to read invitation session:', error)
    return null
  }
}

/** 从当前认证入口的 URL 初始化凭证；没有邀请码时清理旧流程残留。 */
export function syncInvitationCredentialsFromSearch(
  search: string
): InvitationCredentials | null {
  const params = new URLSearchParams(search)
  const hasInvitationQuery = params.has('aff') || params.has('invite')
  const credentials = saveInvitationCredentials(params.get('aff') ?? '')

  if (hasInvitationQuery && typeof window !== 'undefined') {
    params.delete('aff')
    params.delete('invite')
    const remainingQuery = params.toString()
    const sanitizedUrl = `${window.location.pathname}${remainingQuery ? `?${remainingQuery}` : ''}${window.location.hash}`
    window.history.replaceState(window.history.state, '', sanitizedUrl)
  }

  return credentials
}
