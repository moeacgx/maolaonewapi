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
import { nanoid } from 'nanoid'

export const ACTION_MASK = 'mask'
export const ACTION_BLOCK = 'block'
export const SCOPE_REQUEST = 'request'
export const SCOPE_RESPONSE = 'response'
export const SCOPE_BOTH = 'both'
export const TARGET_CHANNELS = 'channels'
export const TARGET_CHANNEL_TAGS = 'channel_tags'
export const TARGET_GROUPS = 'groups'
export const TARGET_ALL = 'all'
export const DEFAULT_REPLACEMENT = '[REDACTED]'

export type SensitiveRuleAction = typeof ACTION_MASK | typeof ACTION_BLOCK
export type SensitiveRuleScope =
  | typeof SCOPE_REQUEST
  | typeof SCOPE_RESPONSE
  | typeof SCOPE_BOTH
export type SensitiveRuleTargetType =
  | typeof TARGET_CHANNELS
  | typeof TARGET_CHANNEL_TAGS
  | typeof TARGET_GROUPS
  | typeof TARGET_ALL

export type SensitiveRule = {
  id: string
  name: string
  enabled: boolean
  action: SensitiveRuleAction
  scope?: SensitiveRuleScope
  replacement?: string
  keywords: string[]
  group_refs?: string[]
  target_type?: SensitiveRuleTargetType
  channel_ids?: number[]
  channel_tags?: string[]
  group_codes?: string[]
}

export type SensitiveRuleDraft = Omit<
  SensitiveRule,
  'keywords'
  | 'group_refs'
  | 'target_type'
  | 'channel_ids'
  | 'channel_tags'
  | 'group_codes'
> & {
  keywordsText: string
  groupRefs: string[]
  targetType: SensitiveRuleTargetType
  channelIds: number[]
  channelTags: string[]
  groupCodes: string[]
}

export type SensitiveRouteOption = {
  value: string
  label: string
}

type SensitiveRulesConfig = {
  rules?: SensitiveRule[]
}

export function splitSensitiveKeywords(value: string) {
  const seen = new Set<string>()
  const keywords: string[] = []

  value
    .replace(/\r\n/g, '\n')
    .split('\n')
    .map((item) => item.trim())
    .filter(Boolean)
    .forEach((item) => {
      const key = item.toLowerCase()
      if (seen.has(key)) return
      seen.add(key)
      keywords.push(item)
    })

  return keywords
}

export function normalizeSensitiveRouteIds(ids: readonly unknown[]) {
  const seen = new Set<number>()
  const normalized: number[] = []

  ids.forEach((rawId) => {
    const id =
      typeof rawId === 'number'
        ? rawId
        : Number.parseInt(String(rawId).trim(), 10)
    if (!Number.isInteger(id) || id <= 0 || seen.has(id)) return
    seen.add(id)
    normalized.push(id)
  })

  return normalized.sort((a, b) => a - b)
}

export function parseSensitiveRuleChannelIds(raw?: string) {
  const trimmed = raw?.trim()
  if (!trimmed) return []

  try {
    const parsed: unknown = JSON.parse(trimmed)
    return Array.isArray(parsed) ? normalizeSensitiveRouteIds(parsed) : []
  } catch {
    return []
  }
}

export function includeMissingSensitiveRouteOptions(
  options: SensitiveRouteOption[],
  selectedIds: number[],
  missingLabel: string
) {
  const knownValues = new Set(options.map((option) => option.value))
  const missingOptions = normalizeSensitiveRouteIds(selectedIds)
    .map(String)
    .filter((value) => !knownValues.has(value))
    .map((value) => ({ value, label: `${missingLabel} #${value}` }))
  return [...options, ...missingOptions]
}

export function normalizeSensitiveChannelTags(tags: readonly unknown[]) {
  const seen = new Set<string>()
  const normalized: string[] = []

  tags.forEach((rawTag) => {
    const tag = String(rawTag ?? '').trim()
    if (!tag || seen.has(tag)) return
    seen.add(tag)
    normalized.push(tag)
  })

  return normalized.sort()
}

export function normalizeSensitiveGroupCodes(codes: readonly unknown[]) {
  return Array.from(
    new Set(
      codes
        .map((code) => String(code ?? '').trim())
        .filter((code) => code && code.toLowerCase() !== 'auto')
    )
  ).sort()
}

export function includeMissingSensitiveGroupOptions(
  options: SensitiveRouteOption[],
  selectedCodes: string[],
  missingLabel: string
) {
  const knownValues = new Set(options.map((option) => option.value))
  const missingOptions = normalizeSensitiveGroupCodes(selectedCodes)
    .filter((value) => !knownValues.has(value))
    .map((value) => ({ value, label: `${missingLabel}: ${value}` }))
  return [...options, ...missingOptions]
}

export function includeMissingSensitiveTagOptions(
  options: SensitiveRouteOption[],
  selectedTags: string[],
  missingLabel: string
) {
  const knownValues = new Set(options.map((option) => option.value))
  const missingOptions = normalizeSensitiveChannelTags(selectedTags)
    .filter((value) => !knownValues.has(value))
    .map((value) => ({ value, label: `${missingLabel}: ${value}` }))
  return [...options, ...missingOptions]
}

function normalizeGroupRefs(groupRefs?: string[]) {
  const seen = new Set<string>()
  const normalized: string[] = []

  ;(groupRefs ?? [])
    .map((item) => String(item).trim())
    .filter(Boolean)
    .forEach((item) => {
      const key = item.toLowerCase()
      if (seen.has(key)) return
      seen.add(key)
      normalized.push(item)
    })

  return normalized
}

export function createSensitiveRuleDraft(): SensitiveRuleDraft {
  return {
    id: nanoid(),
    name: '',
    enabled: true,
    action: ACTION_MASK,
    scope: SCOPE_REQUEST,
    replacement: DEFAULT_REPLACEMENT,
    keywordsText: '',
    groupRefs: [],
    targetType: TARGET_CHANNELS,
    channelIds: [],
    channelTags: [],
    groupCodes: [],
  }
}

function normalizeSensitiveRule(
  rule: SensitiveRuleDraft
): SensitiveRule | null {
  const keywords = splitSensitiveKeywords(rule.keywordsText)
  const groupRefs = normalizeGroupRefs(rule.groupRefs)
  if (keywords.length === 0 && groupRefs.length === 0) return null

  const action = rule.action === ACTION_BLOCK ? ACTION_BLOCK : ACTION_MASK
  const scope =
    rule.scope === SCOPE_RESPONSE || rule.scope === SCOPE_BOTH
      ? rule.scope
      : SCOPE_REQUEST
  const targetType = [TARGET_CHANNEL_TAGS, TARGET_GROUPS, TARGET_ALL].includes(
    rule.targetType
  )
    ? rule.targetType
    : TARGET_CHANNELS
  const fallbackName = keywords[0] ?? groupRefs[0] ?? ''

  return {
    id: rule.id || fallbackName.toLowerCase() || nanoid(),
    name: rule.name.trim() || fallbackName,
    enabled: rule.enabled,
    action,
    scope,
    replacement:
      action === ACTION_MASK
        ? rule.replacement?.trim() || DEFAULT_REPLACEMENT
        : undefined,
    keywords,
    group_refs: groupRefs.length > 0 ? groupRefs : undefined,
    target_type: targetType,
    channel_ids:
      targetType === TARGET_CHANNELS
        ? normalizeSensitiveRouteIds(rule.channelIds)
        : undefined,
    channel_tags:
      targetType === TARGET_CHANNEL_TAGS
        ? normalizeSensitiveChannelTags(rule.channelTags)
        : undefined,
    group_codes:
      targetType === TARGET_GROUPS
        ? normalizeSensitiveGroupCodes(rule.groupCodes)
        : undefined,
  }
}

function rulesToDrafts(
  rules: SensitiveRule[],
  legacyChannelIds: number[]
): SensitiveRuleDraft[] {
  return rules.map((rule) => {
    const targetType =
      rule.target_type === TARGET_CHANNEL_TAGS ||
      rule.target_type === TARGET_GROUPS ||
      rule.target_type === TARGET_ALL
        ? rule.target_type
        : TARGET_CHANNELS
    const usesLegacyChannelScope = rule.target_type === undefined

    return {
      id: rule.id || nanoid(),
      name: rule.name ?? '',
      enabled: rule.enabled ?? true,
      action: rule.action === ACTION_BLOCK ? ACTION_BLOCK : ACTION_MASK,
      scope:
        rule.scope === SCOPE_RESPONSE || rule.scope === SCOPE_BOTH
          ? rule.scope
          : SCOPE_REQUEST,
      replacement: rule.replacement || DEFAULT_REPLACEMENT,
      keywordsText: (rule.keywords ?? []).join('\n'),
      groupRefs: normalizeGroupRefs(rule.group_refs),
      targetType,
      channelIds: normalizeSensitiveRouteIds(
        usesLegacyChannelScope ? legacyChannelIds : (rule.channel_ids ?? [])
      ),
      channelTags: normalizeSensitiveChannelTags(rule.channel_tags ?? []),
      groupCodes: normalizeSensitiveGroupCodes(rule.group_codes ?? []),
    }
  })
}

export function serializeSensitiveRules(rules: SensitiveRuleDraft[]) {
  return JSON.stringify(
    {
      rules: rules
        .map((rule) => normalizeSensitiveRule(rule))
        .filter((rule): rule is SensitiveRule => rule !== null),
    },
    null,
    2
  )
}

export function parseSensitiveRulesConfig(
  raw: string | undefined,
  legacyWords: string | undefined,
  legacyChannelIds: number[]
) {
  const trimmed = raw?.trim()
  if (trimmed) {
    try {
      const parsed = JSON.parse(trimmed) as SensitiveRulesConfig
      if (Array.isArray(parsed.rules)) {
        return rulesToDrafts(parsed.rules, legacyChannelIds)
      }
    } catch {
      return []
    }
  }

  const legacyKeywords = splitSensitiveKeywords(legacyWords ?? '')
  if (legacyKeywords.length === 0) return []

  return rulesToDrafts(
    [
      {
        id: 'legacy-sensitive-words',
        name: 'Legacy sensitive words',
        enabled: true,
        action: ACTION_BLOCK,
        keywords: legacyKeywords,
      },
    ],
    legacyChannelIds
  )
}

export function getEmptySensitiveRuleTarget(
  rule: SensitiveRuleDraft
): SensitiveRuleTargetType | null {
  const hasContent =
    splitSensitiveKeywords(rule.keywordsText).length > 0 ||
    normalizeGroupRefs(rule.groupRefs).length > 0
  if (!rule.enabled || !hasContent) return null

  if (
    rule.targetType === TARGET_CHANNEL_TAGS &&
    normalizeSensitiveChannelTags(rule.channelTags).length === 0
  ) {
    return TARGET_CHANNEL_TAGS
  }
  if (
    rule.targetType === TARGET_GROUPS &&
    normalizeSensitiveGroupCodes(rule.groupCodes).length === 0
  ) {
    return TARGET_GROUPS
  }
  if (
    rule.targetType === TARGET_CHANNELS &&
    normalizeSensitiveRouteIds(rule.channelIds).length === 0
  ) {
    return TARGET_CHANNELS
  }
  return null
}
