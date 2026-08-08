import assert from 'node:assert/strict'
import test from 'node:test'
import {
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
    '[{"match":"balance","mode":"contains","replace":"try later"}]'
  )
  assert.equal(validateErrorMessageReplacementRules(rules), true)
})

test('rejects incomplete replacement rules', () => {
  assert.equal(
    validateErrorMessageReplacementRules([
      { match: 'balance', mode: 'exact', replace: ' ' },
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
      match: 'balance',
      mode: 'contains',
      replaceStatusCode: 429,
      replace: 'try later',
    },
  ])
  assert.equal(
    serializeErrorMessageReplacementRules(rules),
    '[{"match":"balance","mode":"contains","status_code":403,"replace":"try later","replace_status_code":429}]'
  )
  assert.equal(validateErrorMessageReplacementRules(rules), true)
})

test('rejects invalid status-code conditions and replacement status codes', () => {
  assert.equal(
    validateErrorMessageReplacementRules([
      {
        match: 'balance',
        mode: 'exact',
        statusCode: 99,
        replace: 'try later',
        replaceStatusCode: 400,
      },
    ]),
    false
  )
  assert.equal(
    validateErrorMessageReplacementRules([
      {
        match: 'balance',
        mode: 'exact',
        statusCode: 100,
        replace: 'try later',
        replaceStatusCode: 399,
      },
    ]),
    false
  )
  assert.equal(
    validateErrorMessageReplacementRules([
      {
        match: 'balance',
        mode: 'exact',
        statusCode: 100,
        replace: 'try later',
        replaceStatusCode: 400,
      },
    ]),
    true
  )
})
