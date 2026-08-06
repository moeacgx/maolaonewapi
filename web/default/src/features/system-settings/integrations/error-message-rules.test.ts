import assert from 'node:assert/strict'
import test from 'node:test'
import {
  MAX_ERROR_MESSAGE_MATCHES_PER_RULE,
  parseErrorMessageReplacementRules,
  serializeErrorMessageReplacementRules,
  validateErrorMessageReplacementRules,
} from './error-message-rules'

test('parses and serializes client error replacement rules', () => {
  const rules = parseErrorMessageReplacementRules(
    '[{"match":"balance","mode":"contains","replace":"try later"}]'
  )
  assert.equal(rules.length, 1)
  assert.equal(
    serializeErrorMessageReplacementRules(rules),
    '[{"match":"balance","matches":["balance"],"mode":"contains","replace":"try later"}]'
  )
  assert.equal(validateErrorMessageReplacementRules(rules), true)
})

test('rejects incomplete replacement rules', () => {
  assert.equal(
    validateErrorMessageReplacementRules([
      { matches: ['balance'], mode: 'exact', replace: ' ' },
    ]),
    false
  )
})

test('parses status-code conditions and replacements', () => {
  const rules = parseErrorMessageReplacementRules(
    '[{"status_code":403,"match":"balance","mode":"contains","replace_status_code":429,"replace":"try later"}]'
  )

  assert.deepEqual(rules, [
    {
      statusCode: 403,
      matches: ['balance'],
      mode: 'contains',
      replaceStatusCode: 429,
      replace: 'try later',
    },
  ])
  assert.equal(
    serializeErrorMessageReplacementRules(rules),
    '[{"match":"balance","matches":["balance"],"mode":"contains","status_code":403,"replace":"try later","replace_status_code":429}]'
  )
  assert.equal(validateErrorMessageReplacementRules(rules), true)
})

test('rejects invalid status-code conditions and replacements', () => {
  assert.equal(
    validateErrorMessageReplacementRules([
      {
        matches: ['balance'],
        mode: 'exact',
        statusCode: 99,
        replace: 'try later',
        replaceStatusCode: 600,
      },
    ]),
    false
  )
})

test('round-trips multiple match values and prefers the new array', () => {
  const rules = parseErrorMessageReplacementRules(
    '[{"match":"legacy","matches":["balance","quota"],"mode":"contains","replace":"try later"}]'
  )

  assert.deepEqual(rules[0]?.matches, ['balance', 'quota'])
  assert.equal(
    serializeErrorMessageReplacementRules(rules),
    '[{"match":"balance","matches":["balance","quota"],"mode":"contains","replace":"try later"}]'
  )
  assert.equal(validateErrorMessageReplacementRules(rules), true)
})

test('rejects empty, blank, and excessive match values', () => {
  const baseRule = {
    mode: 'contains' as const,
    replace: 'try later',
  }

  assert.equal(
    validateErrorMessageReplacementRules([{ ...baseRule, matches: [] }]),
    false
  )
  assert.equal(
    validateErrorMessageReplacementRules([
      { ...baseRule, matches: ['balance', ' '] },
    ]),
    false
  )
  assert.equal(
    validateErrorMessageReplacementRules([
      {
        ...baseRule,
        matches: Array.from(
          { length: MAX_ERROR_MESSAGE_MATCHES_PER_RULE + 1 },
          (_, index) => `match-${index}`
        ),
      },
    ]),
    false
  )
})

test('does not silently keep a partially invalid matches array', () => {
  const rules = parseErrorMessageReplacementRules(
    '[{"matches":["balance",42],"mode":"contains","replace":"try later"}]'
  )

  assert.deepEqual(rules[0]?.matches, [])
  assert.equal(validateErrorMessageReplacementRules(rules), false)
})
