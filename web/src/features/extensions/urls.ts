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

/** Builds a same-origin host proxy URL; manifest paths never become origins. */
export function getExtensionPageUrl(
  moduleId: string,
  pagePath: string,
  moduleVersion = ''
): string {
  const trimmedPath = pagePath.trim()
  const normalizedPath = trimmedPath.startsWith('/')
    ? trimmedPath
    : `/${trimmedPath}`
  const fragmentIndex = normalizedPath.indexOf('#')
  const pathAndQuery =
    fragmentIndex < 0 ? normalizedPath : normalizedPath.slice(0, fragmentIndex)
  const fragment = fragmentIndex < 0 ? '' : normalizedPath.slice(fragmentIndex)
  const baseUrl = `/api/extensions/${encodeURIComponent(moduleId)}/proxy${pathAndQuery}`

  const queryIndex = pathAndQuery.indexOf('?')
  const hasModuleVersion =
    queryIndex >= 0 &&
    new URLSearchParams(pathAndQuery.slice(queryIndex + 1)).has(
      'module_version'
    )
  if (!moduleVersion || hasModuleVersion) return `${baseUrl}${fragment}`

  const separator = baseUrl.includes('?') ? '&' : '?'
  return `${baseUrl}${separator}module_version=${encodeURIComponent(moduleVersion)}${fragment}`
}

/** Builds a version-pinned same-origin URL for native extension assets. */
export function getNativeExtensionAssetUrl(
  moduleId: string,
  pageKey: string,
  moduleVersion: string,
  assetRevision: string | undefined,
  asset: 'entry' | `style-${number}`,
  loadAttempt = 0
): string {
  const baseUrl = `/api/extensions/${encodeURIComponent(moduleId)}/native/${encodeURIComponent(pageKey)}/default/${asset}`
  const params = new URLSearchParams({ module_version: moduleVersion })
  if (assetRevision) params.set('asset_revision', assetRevision)
  if (loadAttempt > 0) params.set('load_attempt', String(loadAttempt))
  return `${baseUrl}?${params.toString()}`
}
