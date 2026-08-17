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
export type SidebarModuleConfig = {
  enabled: boolean
  [key: string]: boolean
}

export type SidebarModulesConfig = Record<string, SidebarModuleConfig>

export type SidebarSectionDefinition = {
  key: string
  title: string
  description: string
  modules: { key: string; title: string; description: string }[]
}

export const createDefaultSidebarConfig = (
  sectionDefs: SidebarSectionDefinition[]
): SidebarModulesConfig =>
  sectionDefs.reduce<SidebarModulesConfig>((defaults, section) => {
    defaults[section.key] = { enabled: true }
    section.modules.forEach((module) => {
      defaults[section.key][module.key] = true
    })
    return defaults
  }, {})

export function normalizeUserSidebarConfig(
  raw: unknown,
  fallback: SidebarModulesConfig
): SidebarModulesConfig {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return fallback

  const normalized = Object.fromEntries(
    Object.entries(fallback).map(([key, section]) => [key, { ...section }])
  ) as SidebarModulesConfig
  Object.entries(raw as Record<string, unknown>).forEach(
    ([sectionKey, sectionValue]) => {
      if (
        !sectionValue ||
        typeof sectionValue !== 'object' ||
        Array.isArray(sectionValue)
      ) {
        return
      }
      const section = normalized[sectionKey] ?? { enabled: true }
      Object.entries(sectionValue as Record<string, unknown>).forEach(
        ([moduleKey, moduleValue]) => {
          if (typeof moduleValue === 'boolean') section[moduleKey] = moduleValue
        }
      )
      normalized[sectionKey] = section
    }
  )
  return normalized
}
