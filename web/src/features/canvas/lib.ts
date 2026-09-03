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
import type { GroupOption } from '@/features/playground/types'
import { normalizeCanvasOrigin } from '@/lib/canvas-settings'

type CanvasLaunchUrlOptions = {
  canvasOrigin: string
  newApiOrigin: string
  group: string
}

export function buildCanvasLaunchUrl(options: CanvasLaunchUrlOptions): string {
  const canvasUrl = new URL('/', normalizeCanvasOrigin(options.canvasOrigin))
  const newApiOrigin = options.newApiOrigin.trim().replace(/\/+$/, '')

  canvasUrl.searchParams.set('mode', 'newapi')
  canvasUrl.searchParams.set('baseUrl', `${newApiOrigin}/canvas`)
  canvasUrl.searchParams.set('group', options.group)

  return canvasUrl.toString()
}

export function resolveCanvasDefaultGroup(
  groups: GroupOption[],
  configuredGroup: string
): string {
  const configured = configuredGroup.trim()
  if (configured && groups.some((group) => group.value === configured)) {
    return configured
  }

  return (
    groups.find((group) => group.value === 'default')?.value ??
    groups[0]?.value ??
    ''
  )
}
