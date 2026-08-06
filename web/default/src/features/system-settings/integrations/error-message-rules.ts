export const ERROR_MESSAGE_REPLACEMENT_MODES = [
  'contains',
  'exact',
  'regex',
] as const

export type ErrorMessageReplacementMode =
  (typeof ERROR_MESSAGE_REPLACEMENT_MODES)[number]

export type ErrorMessageReplacementRule = {
  matches: string[]
  mode: ErrorMessageReplacementMode
  statusCode?: number
  replace: string
  replaceStatusCode?: number
}

export const MAX_ERROR_MESSAGE_REPLACEMENT_RULES = 100
export const MAX_ERROR_MESSAGE_MATCHES_PER_RULE = 64
export const MAX_ERROR_MESSAGE_MATCH_LENGTH = 4096
export const MAX_ERROR_MESSAGE_REPLACE_LENGTH = 4096

const isMode = (value: unknown): value is ErrorMessageReplacementMode =>
  typeof value === 'string' &&
  ERROR_MESSAGE_REPLACEMENT_MODES.includes(value as ErrorMessageReplacementMode)

const parseOriginalStatusCode = (value: unknown): number | undefined =>
  typeof value === 'number' &&
  Number.isInteger(value) &&
  value >= 100 &&
  value <= 599
    ? value
    : undefined

const parseReplaceStatusCode = (value: unknown): number | undefined =>
  typeof value === 'number' &&
  Number.isInteger(value) &&
  value >= 400 &&
  value <= 599
    ? value
    : undefined

const isValidOriginalStatusCode = (value: number | undefined): boolean =>
  value === undefined ||
  (Number.isInteger(value) && value >= 100 && value <= 599)

const isValidReplaceStatusCode = (value: number | undefined): boolean =>
  value === undefined ||
  (Number.isInteger(value) && value >= 400 && value <= 599)

const parseMatches = (item: Record<string, unknown>): string[] => {
  if (Array.isArray(item.matches)) {
    return item.matches.every(
      (value): value is string => typeof value === 'string'
    )
      ? item.matches
      : []
  }
  return typeof item.match === 'string' ? [item.match] : []
}

export function parseErrorMessageReplacementRules(
  raw: string
): ErrorMessageReplacementRule[] {
  try {
    const value: unknown = JSON.parse(raw)
    if (!Array.isArray(value)) return []
    return value
      .filter(
        (item): item is Record<string, unknown> =>
          item !== null && typeof item === 'object' && !Array.isArray(item)
      )
      .filter((item) => typeof item.replace === 'string' && isMode(item.mode))
      .slice(0, MAX_ERROR_MESSAGE_REPLACEMENT_RULES)
      .map((item) => ({
        matches: parseMatches(item),
        mode: item.mode as ErrorMessageReplacementMode,
        statusCode: parseOriginalStatusCode(item.status_code),
        replace: item.replace as string,
        replaceStatusCode: parseReplaceStatusCode(item.replace_status_code),
      }))
  } catch {
    return []
  }
}

export function serializeErrorMessageReplacementRules(
  rules: ErrorMessageReplacementRule[]
): string {
  return JSON.stringify(
    rules.map((rule) => ({
      match: rule.matches[0]?.trim(),
      matches: rule.matches.map((match) => match.trim()),
      mode: rule.mode,
      status_code: rule.statusCode,
      replace: rule.replace.trim(),
      replace_status_code: rule.replaceStatusCode,
    }))
  )
}

export function createErrorMessageReplacementRule(): ErrorMessageReplacementRule {
  return {
    matches: [''],
    mode: 'contains',
    statusCode: undefined,
    replace: '',
    replaceStatusCode: undefined,
  }
}

export function validateErrorMessageReplacementRules(
  rules: ErrorMessageReplacementRule[]
): boolean {
  return (
    rules.length <= MAX_ERROR_MESSAGE_REPLACEMENT_RULES &&
    rules.every(
      (rule) =>
        rule.matches.length > 0 &&
        rule.matches.length <= MAX_ERROR_MESSAGE_MATCHES_PER_RULE &&
        rule.matches.every(
          (match) =>
            match.trim().length > 0 &&
            match.trim().length <= MAX_ERROR_MESSAGE_MATCH_LENGTH
        ) &&
        rule.replace.trim().length > 0 &&
        rule.replace.trim().length <= MAX_ERROR_MESSAGE_REPLACE_LENGTH &&
        isValidOriginalStatusCode(rule.statusCode) &&
        isValidReplaceStatusCode(rule.replaceStatusCode) &&
        isMode(rule.mode)
    )
  )
}
