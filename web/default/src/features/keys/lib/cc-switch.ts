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
export type StatusLike = Record<string, unknown> & {
  data?: Record<string, unknown>
}

type BuildCCSwitchURLParams = {
  app: string
  name: string
  models: Record<string, string>
  apiKey: string
  status?: StatusLike | null
  origin?: string
}

export function normalizeCCSwitchAddress(value: unknown): string {
  return typeof value === 'string' ? value.trim().replace(/\/+$/, '') : ''
}

function asStatusLike(value: unknown): StatusLike {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as StatusLike)
    : {}
}

export function withCCSwitchAPIAddress(
  status: unknown,
  value: unknown
): StatusLike {
  const current = asStatusLike(status)
  const normalized = normalizeCCSwitchAddress(value)
  const next: StatusLike = {
    ...current,
    cc_switch_api_address: normalized,
  }

  if (current.data !== null && typeof current.data === 'object') {
    next.data = {
      ...current.data,
      cc_switch_api_address: normalized,
    }
  }

  return next
}

export function isValidCCSwitchAddress(value: unknown): boolean {
  const address = normalizeCCSwitchAddress(value)
  if (!address) return true

  try {
    const parsed = new URL(address)
    return (
      (parsed.protocol === 'http:' || parsed.protocol === 'https:') &&
      !parsed.username &&
      !parsed.password &&
      !parsed.search &&
      !parsed.hash
    )
  } catch {
    return false
  }
}

function getStatusAddress(status: StatusLike | null | undefined, key: string) {
  return normalizeCCSwitchAddress(status?.[key] ?? status?.data?.[key])
}

function readStoredStatus(): StatusLike | undefined {
  if (typeof window === 'undefined') return undefined

  try {
    const raw = window.localStorage.getItem('status')
    return raw ? (JSON.parse(raw) as StatusLike) : undefined
  } catch {
    return undefined
  }
}

function getCurrentOrigin() {
  return typeof window === 'undefined' ? '' : window.location.origin
}

export function resolveCCSwitchAddresses(
  status?: StatusLike | null,
  origin = getCurrentOrigin()
): { apiAddress: string; homepage: string } {
  const storedStatus = readStoredStatus()
  const fallbackAddress = normalizeCCSwitchAddress(origin)
  const homepage =
    getStatusAddress(status, 'server_address') ||
    getStatusAddress(storedStatus, 'server_address') ||
    fallbackAddress
  const apiAddress =
    getStatusAddress(status, 'cc_switch_api_address') ||
    getStatusAddress(storedStatus, 'cc_switch_api_address') ||
    homepage

  return { apiAddress, homepage }
}

export function isCCSwitchPreset(url: string): boolean {
  return url.trim().toLowerCase().startsWith('ccswitch')
}

export function buildCCSwitchURL({
  app,
  name,
  models,
  apiKey,
  status,
  origin,
}: BuildCCSwitchURLParams): string {
  const { apiAddress, homepage } = resolveCCSwitchAddresses(status, origin)
  const endpoint =
    app === 'codex' && apiAddress && !/\/v1$/i.test(apiAddress)
      ? `${apiAddress}/v1`
      : apiAddress
  const params = new URLSearchParams()
  params.set('resource', 'provider')
  params.set('app', app)
  params.set('name', name)
  params.set('endpoint', endpoint)
  params.set('apiKey', apiKey)
  for (const [key, value] of Object.entries(models)) {
    if (value) params.set(key, value)
  }
  params.set('homepage', homepage)
  params.set('enabled', 'true')
  return `ccswitch://v1/import?${params.toString()}`
}
