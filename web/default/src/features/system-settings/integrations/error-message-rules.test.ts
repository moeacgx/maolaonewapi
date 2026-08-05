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
