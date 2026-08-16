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

export type GroupMode = 'inherit' | 'explicit' | 'auto'

export type GroupIdentity = {
  id?: number
  code: string
  name?: string
  exclusive?: boolean
}

export type GroupSelectionOption = {
  value: string
  label: string
  desc?: string
  ratio?: number | string
  id?: number
  exclusive?: boolean
}

export function createGroupSelectionOptions(
  groups: Record<
    string,
    {
      desc?: string
      ratio?: number | string
      id?: number
      code?: string
      name?: string
      exclusive?: boolean
    }
  >,
  retained: readonly GroupIdentity[] = []
): GroupSelectionOption[] {
  const options = Object.entries(groups).map<GroupSelectionOption>(
    ([key, info]) => ({
      value: info.code || key,
      label: info.name || info.code || key,
      desc: info.desc || info.name || info.code || key,
      ratio: info.ratio,
      id: Number.isInteger(info.id) ? info.id : undefined,
      exclusive: info.exclusive === true,
    })
  )
  const known = new Set(options.map((option) => option.value))

  for (const identity of retained) {
    if (!identity.code || known.has(identity.code)) continue
    options.push({
      value: identity.code,
      label: identity.name || identity.code,
      desc: identity.name || identity.code,
      id: Number.isInteger(identity.id) ? identity.id : undefined,
      exclusive: identity.exclusive === true,
    })
    known.add(identity.code)
  }

  return options
}

export function normalizeGroupSelection(
  previous: readonly string[],
  next: readonly string[],
  options: readonly GroupSelectionOption[]
): string[] {
  const unique = [...new Set(next.filter(Boolean))]
  const newlySelected = unique.find((value) => !previous.includes(value))
  const selectedOption = options.find(
    (option) => option.value === newlySelected
  )

  if (newlySelected === 'auto' || selectedOption?.exclusive) {
    return newlySelected ? [newlySelected] : []
  }
  return unique
    .filter((value) => value !== 'auto')
    .filter((value) => {
      const option = options.find((candidate) => candidate.value === value)
      return option?.exclusive !== true
    })
}

export function resolveApiKeyGroups(apiKey: {
  group?: string | null
  group_mode?: GroupMode | null
  group_details?: readonly GroupIdentity[]
}): string[] {
  if (apiKey.group_mode === 'inherit') return []
  if (apiKey.group_mode === 'auto' || apiKey.group === 'auto') return ['auto']
  if (apiKey.group_details?.length) {
    return apiKey.group_details.map((group) => group.code).filter(Boolean)
  }
  return (apiKey.group || '')
    .split(',')
    .map((group) => group.trim())
    .filter(Boolean)
}

export function buildGroupSelectionPayload(
  groups: readonly string[],
  options: readonly GroupSelectionOption[]
): { group: string; group_ids: number[]; group_mode: GroupMode } {
  if (groups.length === 0) {
    return { group: '', group_ids: [], group_mode: 'inherit' }
  }
  if (groups.length === 1 && groups[0] === 'auto') {
    return { group: 'auto', group_ids: [], group_mode: 'auto' }
  }

  const optionMap = new Map(options.map((option) => [option.value, option]))
  return {
    group: groups.join(','),
    group_ids: groups.flatMap((code) => {
      const id = optionMap.get(code)?.id
      return typeof id === 'number' && Number.isInteger(id) ? [id] : []
    }),
    group_mode: 'explicit',
  }
}
