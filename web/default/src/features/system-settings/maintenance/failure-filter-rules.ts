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
*/

export const FAILURE_FILTER_FIELDS = [
  'status_code',
  'error_code',
  'message',
  'full_error',
] as const

export const FAILURE_FILTER_MODES = ['contains', 'exact', 'regex'] as const
export const FAILURE_FILTER_RULE_ID_PATTERN = /^[A-Za-z0-9._-]+$/
export const MAX_FAILURE_FILTER_VALUES = 64
export const MAX_FAILURE_FILTER_VALUE_LENGTH = 4096

export type FailureFilterField = (typeof FAILURE_FILTER_FIELDS)[number]
export type FailureFilterMode = (typeof FAILURE_FILTER_MODES)[number]

export type FailureFilterRule = {
  id: string
  name: string
  enabled: boolean
  field: FailureFilterField
  mode: FailureFilterMode
  values: string[]
}

const isFailureFilterField = (value: unknown): value is FailureFilterField =>
  typeof value === 'string' &&
  FAILURE_FILTER_FIELDS.includes(value as FailureFilterField)

const isFailureFilterMode = (value: unknown): value is FailureFilterMode =>
  typeof value === 'string' &&
  FAILURE_FILTER_MODES.includes(value as FailureFilterMode)

export const isValidFailureFilterRuleId = (value: unknown): value is string =>
  typeof value === 'string' &&
  value.trim().length > 0 &&
  value.trim().length <= 64 &&
  FAILURE_FILTER_RULE_ID_PATTERN.test(value.trim())

export function parseFailureFilterRules(raw: string): FailureFilterRule[] {
  try {
    const value: unknown = JSON.parse(raw)
    if (!Array.isArray(value)) return []

    return value
      .filter(
        (item): item is Record<string, unknown> =>
          item !== null && typeof item === 'object' && !Array.isArray(item)
      )
      .filter(
        (item) =>
          isValidFailureFilterRuleId(item.id) &&
          typeof item.name === 'string' &&
          typeof item.enabled === 'boolean' &&
          isFailureFilterField(item.field) &&
          isFailureFilterMode(item.mode) &&
          (Array.isArray(item.values) || typeof item.value === 'string')
      )
      .slice(0, 100)
      .map((item) => ({
        id: (item.id as string).trim(),
        name: item.name as string,
        enabled: item.enabled as boolean,
        field: item.field as FailureFilterField,
        mode: item.mode as FailureFilterMode,
        values:
          Array.isArray(item.values) &&
          item.values.every((value) => typeof value === 'string')
            ? (item.values as string[])
            : typeof item.value === 'string'
              ? [item.value]
              : [],
      }))
  } catch {
    return []
  }
}

export function serializeFailureFilterRules(
  rules: FailureFilterRule[]
): string {
  return JSON.stringify(
    rules.map((rule) => ({
      ...rule,
      id: rule.id.trim(),
      name: rule.name.trim(),
      // 保留 value 作为旧版本后端的首个值兼容字段；新后端使用 values。
      value: rule.values[0] ?? '',
    }))
  )
}

export function createFailureFilterRule(): FailureFilterRule {
  const randomPart =
    typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
      ? crypto.randomUUID()
      : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`

  return {
    id: `failure-filter-${randomPart}`,
    name: '',
    enabled: true,
    field: 'status_code',
    mode: 'exact',
    values: [],
  }
}
