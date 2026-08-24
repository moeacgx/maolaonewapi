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
import {
  DYNAMIC_ROUTING_ACTION_MODEL_REDIRECT,
  DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE,
  DYNAMIC_ROUTING_ACTIONS,
  DYNAMIC_ROUTING_IMAGE_TARGET_PATHS,
  DYNAMIC_ROUTING_IMAGE_GENERATION_PATH,
  DYNAMIC_ROUTING_CONDITION_REASONING_EFFORT,
  DYNAMIC_ROUTING_CONDITION_REQUEST_PREFIX,
  DYNAMIC_ROUTING_OPERATORS,
  DYNAMIC_ROUTING_RESPONSES_PATH,
  type DynamicRoutingChannelConfig,
  type DynamicRoutingChannelMode,
  type DynamicRoutingCondition,
  type DynamicRoutingAction,
  type DynamicRoutingOperator,
  type DynamicRoutingRule,
} from '../types'

const MAX_RULES = 100
const MAX_CONDITIONS = 8
const MAX_PRIORITY = 1000
const MAX_STRING_LENGTH = 256

let nextRuleNumber = 0

export const DYNAMIC_ROUTING_PRESETS = [
  {
    id: 'model_redirect',
    label: 'Basic model redirect',
    description:
      'Keep the request endpoint and rewrite only the final upstream model.',
  },
  {
    id: 'reasoning_high',
    label: 'Reasoning effort redirect',
    description:
      'Route requests with reasoning_effort=high to a dedicated upstream model.',
  },
  {
    id: 'responses_image_tool',
    label: 'Responses image tool to Responses',
    description:
      'Bridge an explicitly selected image_generation tool to a Responses-capable image model.',
  },
  {
    id: 'images_api_image_tool',
    label: 'Responses image tool to Images API',
    description:
      'Bridge an explicitly selected image_generation tool to /v1/images/generations.',
  },
] as const

export type DynamicRoutingPreset =
  (typeof DYNAMIC_ROUTING_PRESETS)[number]['id']

function uniqueStrings(values: string[] | undefined): string[] {
  return [
    ...new Set((values ?? []).map((value) => value.trim()).filter(Boolean)),
  ]
}

function uniqueChannelTypes(values: number[] | undefined): number[] {
  return [
    ...new Set(
      (values ?? []).filter(
        (value) => Number.isInteger(value) && Number.isFinite(value)
      )
    ),
  ]
}

function uniqueSourceGroups(values: string[] | undefined): string[] {
  return uniqueStrings(values)
}

function normalizeCondition(
  condition: DynamicRoutingCondition
): DynamicRoutingCondition {
  const operator = DYNAMIC_ROUTING_OPERATORS.includes(
    condition.operator as DynamicRoutingOperator
  )
    ? condition.operator
    : 'equals'

  const normalized: DynamicRoutingCondition = {
    field: condition.field.trim(),
    operator,
  }

  if (operator !== 'exists' && operator !== 'not_exists') {
    normalized.value = condition.value?.trim() ?? ''
  }

  return normalized
}

function normalizeAction(value: unknown): DynamicRoutingAction {
  return DYNAMIC_ROUTING_ACTIONS.includes(value as DynamicRoutingAction)
    ? (value as DynamicRoutingAction)
    : DYNAMIC_ROUTING_ACTION_MODEL_REDIRECT
}

export function normalizeDynamicRoutingRule(
  rule: DynamicRoutingRule
): DynamicRoutingRule {
  const normalized: DynamicRoutingRule = {
    id: rule.id.trim(),
    enabled: rule.enabled === true,
    action: normalizeAction(rule.action),
    source_model: rule.source_model.trim(),
    target_model: rule.target_model.trim(),
    priority: Number.isInteger(rule.priority) ? rule.priority : 0,
  }

  const sourceGroups = uniqueSourceGroups(rule.source_groups)
  if (sourceGroups.length > 0) normalized.source_groups = sourceGroups
  if (rule.target_group?.trim()) {
    normalized.target_group = rule.target_group.trim()
  }

  const channelTypes = uniqueChannelTypes(rule.channel_types)
  if (channelTypes.length > 0) normalized.channel_types = channelTypes

  const requestPaths =
    normalized.action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE
      ? ['/v1/responses']
      : uniqueStrings(rule.request_paths)
  if (requestPaths.length > 0) normalized.request_paths = requestPaths

  if (
    normalized.action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE
  ) {
    normalized.target_path =
      rule.target_path?.trim() || DYNAMIC_ROUTING_IMAGE_GENERATION_PATH
    if (
      !DYNAMIC_ROUTING_IMAGE_TARGET_PATHS.includes(
        normalized.target_path as (typeof DYNAMIC_ROUTING_IMAGE_TARGET_PATHS)[number]
      )
    ) {
      normalized.target_path = DYNAMIC_ROUTING_IMAGE_GENERATION_PATH
    }
  }

  const conditions = (rule.conditions ?? []).map(normalizeCondition)
  if (conditions.length > 0) normalized.conditions = conditions

  return normalized
}

export function normalizeDynamicRoutingRules(
  rules: DynamicRoutingRule[]
): DynamicRoutingRule[] {
  return rules.map(normalizeDynamicRoutingRule)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function parseCondition(value: unknown): DynamicRoutingCondition | null {
  if (!isRecord(value) || typeof value.field !== 'string') return null
  const operator = DYNAMIC_ROUTING_OPERATORS.includes(
    value.operator as DynamicRoutingOperator
  )
    ? (value.operator as DynamicRoutingOperator)
    : undefined
  return normalizeCondition({
    field: value.field,
    operator,
    value: typeof value.value === 'string' ? value.value : undefined,
  })
}

export function parseDynamicRoutingRules(value: unknown): DynamicRoutingRule[] {
  if (!Array.isArray(value)) return []
  return value.flatMap((item) => {
    if (!isRecord(item)) return []
    const conditions = Array.isArray(item.conditions)
      ? item.conditions.flatMap((condition) => {
          const parsed = parseCondition(condition)
          return parsed ? [parsed] : []
        })
      : []
    return [
      normalizeDynamicRoutingRule({
        id: typeof item.id === 'string' ? item.id : '',
        enabled: item.enabled === true,
        action: normalizeAction(item.action),
        source_model:
          typeof item.source_model === 'string' ? item.source_model : '',
        target_model:
          typeof item.target_model === 'string' ? item.target_model : '',
        target_path:
          typeof item.target_path === 'string' ? item.target_path : undefined,
        source_groups: Array.isArray(item.source_groups)
          ? item.source_groups.filter(
              (group): group is string => typeof group === 'string'
            )
          : [],
        target_group:
          typeof item.target_group === 'string' ? item.target_group : undefined,
        channel_types: Array.isArray(item.channel_types)
          ? item.channel_types.filter(
              (channelType): channelType is number =>
                typeof channelType === 'number'
            )
          : [],
        request_paths: Array.isArray(item.request_paths)
          ? item.request_paths.filter(
              (path): path is string => typeof path === 'string'
            )
          : [],
        conditions,
        priority:
          typeof item.priority === 'number' && Number.isInteger(item.priority)
            ? item.priority
            : 0,
      }),
    ]
  })
}

export function parseDynamicRoutingChannelConfig(
  value: unknown
): DynamicRoutingChannelConfig | undefined {
  if (!isRecord(value)) return undefined
  const rules = parseDynamicRoutingRules(value.rules)
  const enabled = typeof value.enabled === 'boolean' ? value.enabled : undefined
  let mode: DynamicRoutingChannelMode = 'inherit'
  if (enabled === true) {
    mode = 'enabled'
  } else if (enabled === false) {
    mode = 'disabled'
  }
  return buildDynamicRoutingChannelConfig(mode, rules)
}

function createDynamicRoutingRuleBase(prefix: string): DynamicRoutingRule {
  nextRuleNumber += 1
  return {
    id: `${prefix}-${Date.now().toString(36)}-${nextRuleNumber}`,
    enabled: true,
    action: DYNAMIC_ROUTING_ACTION_MODEL_REDIRECT,
    source_model: '',
    target_model: '',
    priority: 0,
  }
}

export function createDynamicRoutingRule(): DynamicRoutingRule {
  return createDynamicRoutingRuleBase('route')
}

export function createDynamicRoutingRuleFromPreset(
  preset: DynamicRoutingPreset
): DynamicRoutingRule {
  switch (preset) {
    case 'reasoning_high':
      return {
        ...createDynamicRoutingRuleBase('reasoning-high'),
        conditions: [
          {
            field: DYNAMIC_ROUTING_CONDITION_REASONING_EFFORT,
            operator: 'equals',
            value: 'high',
          },
        ],
      }
    case 'responses_image_tool':
      return {
        ...createDynamicRoutingRuleBase('responses-image'),
        action: DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE,
        request_paths: [DYNAMIC_ROUTING_RESPONSES_PATH],
        target_path: DYNAMIC_ROUTING_RESPONSES_PATH,
      }
    case 'images_api_image_tool':
      return {
        ...createDynamicRoutingRuleBase('images-api-image'),
        action: DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE,
        request_paths: [DYNAMIC_ROUTING_RESPONSES_PATH],
        target_path: DYNAMIC_ROUTING_IMAGE_GENERATION_PATH,
      }
    case 'model_redirect':
      return createDynamicRoutingRuleBase('model-redirect')
  }
}

export function getDynamicRoutingChannelMode(
  config: DynamicRoutingChannelConfig | undefined
): DynamicRoutingChannelMode {
  if (config?.enabled === true) return 'enabled'
  if (config?.enabled === false) return 'disabled'
  return 'inherit'
}

export function buildDynamicRoutingChannelConfig(
  mode: DynamicRoutingChannelMode,
  rules: DynamicRoutingRule[]
): DynamicRoutingChannelConfig | undefined {
  const normalizedRules = normalizeDynamicRoutingRules(rules)
  if (mode === 'inherit' && normalizedRules.length === 0) return undefined

  const config: DynamicRoutingChannelConfig = {}
  if (mode === 'enabled') config.enabled = true
  if (mode === 'disabled') config.enabled = false
  if (normalizedRules.length > 0) config.rules = normalizedRules
  return config
}

function isValidRequestField(field: string): boolean {
  const path = field.slice(DYNAMIC_ROUTING_CONDITION_REQUEST_PREFIX.length)
  return (
    path.length > 0 &&
    path.length <= MAX_STRING_LENGTH &&
    !path.startsWith('.') &&
    !path.endsWith('.') &&
    !path.includes('..') &&
    /^[A-Za-z0-9_.]+$/.test(path)
  )
}

export function validateDynamicRoutingRules(
  rules: DynamicRoutingRule[]
): string | null {
  if (rules.length > MAX_RULES) {
    return 'Dynamic routing supports at most 100 rules.'
  }

  const enabledIds = new Set<string>()
  for (const rule of rules) {
    const priority = rule.priority ?? 0
    if (
      !Number.isInteger(priority) ||
      priority < -MAX_PRIORITY ||
      priority > MAX_PRIORITY
    ) {
      return 'Dynamic routing priority must be an integer between -1000 and 1000.'
    }
    if ((rule.conditions?.length ?? 0) > MAX_CONDITIONS) {
      return 'Each dynamic routing rule supports at most 8 conditions.'
    }
    if (
      (rule.channel_types ?? []).some(
        (channelType) => !Number.isInteger(channelType) || channelType <= 0
      )
    ) {
      return 'Dynamic routing channel types must use valid positive IDs.'
    }
    if (
      (rule.request_paths ?? []).some(
        (path) =>
          !path.startsWith('/') ||
          path.includes('?') ||
          path.length > MAX_STRING_LENGTH
      )
    ) {
      return 'Dynamic routing request paths must start with "/" and cannot contain query strings.'
    }
    if (
      (rule.source_groups ?? []).some(
        (group) =>
          !group.trim() ||
          group.length > MAX_STRING_LENGTH ||
          group.includes(',') ||
          group.trim().toLowerCase() === 'auto'
      )
    ) {
      return 'Dynamic routing source groups must use valid group codes.'
    }
    if (
      rule.target_group !== undefined &&
      (rule.target_group.length > MAX_STRING_LENGTH ||
        rule.target_group.includes(',') ||
        rule.target_group.trim().toLowerCase() === 'auto')
    ) {
      return 'Dynamic routing target group must be one valid group code.'
    }
    for (const condition of rule.conditions ?? []) {
      const field = condition.field.trim()
      if (
        field !== DYNAMIC_ROUTING_CONDITION_REASONING_EFFORT &&
        (!field.startsWith(DYNAMIC_ROUTING_CONDITION_REQUEST_PREFIX) ||
          !isValidRequestField(field))
      ) {
        return 'Dynamic routing condition fields must be reasoning_effort or request.<simple_json_path>.'
      }
      if (
        condition.operator !== undefined &&
        !DYNAMIC_ROUTING_OPERATORS.includes(condition.operator)
      ) {
        return 'Dynamic routing conditions must use a supported operator.'
      }
      if ((condition.value?.length ?? 0) > MAX_STRING_LENGTH) {
        return 'Dynamic routing condition values must be 256 characters or fewer.'
      }
    }

    if (
      normalizeAction(rule.action) ===
        DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE &&
      ((rule.request_paths ?? []).length !== 1 ||
        rule.request_paths?.[0] !== '/v1/responses')
    ) {
      return 'Responses image tool bridge rules must use exactly the /v1/responses request path.'
    }
    if (
      normalizeAction(rule.action) ===
        DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE &&
      !DYNAMIC_ROUTING_IMAGE_TARGET_PATHS.includes(
        (rule.target_path?.trim() ||
          DYNAMIC_ROUTING_IMAGE_GENERATION_PATH) as (typeof DYNAMIC_ROUTING_IMAGE_TARGET_PATHS)[number]
      )
    ) {
      return 'Responses image tool bridge target path must be /v1/responses or /v1/images/generations.'
    }
    if (!rule.enabled) continue
    if (
      !rule.id.trim() ||
      !rule.source_model.trim() ||
      !rule.target_model.trim()
    ) {
      return 'Each enabled dynamic routing rule requires an ID, source model, and target model.'
    }
    if (
      rule.id.length > MAX_STRING_LENGTH ||
      rule.source_model.length > MAX_STRING_LENGTH ||
      rule.target_model.length > MAX_STRING_LENGTH ||
      (rule.target_group?.length ?? 0) > MAX_STRING_LENGTH
    ) {
      return 'Dynamic routing IDs and model names must be 256 characters or fewer.'
    }
    if (enabledIds.has(rule.id)) {
      return 'Enabled dynamic routing rule IDs must be unique.'
    }
    enabledIds.add(rule.id)
  }
  return null
}

export function validateDynamicRoutingChannelConfig(
  config: DynamicRoutingChannelConfig | undefined
): string | null {
  return validateDynamicRoutingRules(config?.rules ?? [])
}
