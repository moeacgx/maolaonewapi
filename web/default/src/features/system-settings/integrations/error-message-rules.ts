export const ERROR_MESSAGE_REPLACEMENT_MODES = [
  'contains',
  'exact',
  'regex',
] as const

export type ErrorMessageReplacementMode =
  (typeof ERROR_MESSAGE_REPLACEMENT_MODES)[number]

export type ErrorMessageReplacementRule = {
  match: string
  mode: ErrorMessageReplacementMode
  statusCode?: number
  replace: string
  replaceStatusCode?: number
}

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
      .filter(
        (item) =>
          typeof item.match === 'string' &&
          typeof item.replace === 'string' &&
          isMode(item.mode)
      )
      .slice(0, 100)
      .map((item) => ({
        match: item.match as string,
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
      match: rule.match.trim(),
      mode: rule.mode,
      status_code: rule.statusCode,
      replace: rule.replace.trim(),
      replace_status_code: rule.replaceStatusCode,
    }))
  )
}

export function createErrorMessageReplacementRule(): ErrorMessageReplacementRule {
  return {
    match: '',
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
    rules.length <= 100 &&
    rules.every(
      (rule) =>
        rule.match.trim().length > 0 &&
        rule.match.trim().length <= 4096 &&
        rule.replace.trim().length > 0 &&
        rule.replace.trim().length <= 4096 &&
        isValidOriginalStatusCode(rule.statusCode) &&
        isValidReplaceStatusCode(rule.replaceStatusCode) &&
        isMode(rule.mode)
    )
  )
}
