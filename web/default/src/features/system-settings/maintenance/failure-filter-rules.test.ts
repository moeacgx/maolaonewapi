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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  createFailureFilterRule,
  isValidFailureFilterRuleId,
  parseFailureFilterRules,
  serializeFailureFilterRules,
  type FailureFilterRule,
} from './failure-filter-rules.ts'

describe('model square failure filter rules', () => {
  test('parses supported fields and matching modes', () => {
    const rules = parseFailureFilterRules(
      JSON.stringify([
        {
          id: 'status-400',
          name: 'Status 400',
          enabled: true,
          field: 'status_code',
          mode: 'exact',
          value: '400',
        },
        {
          id: 'content-policy',
          name: 'Content policy',
          enabled: true,
          field: 'full_error',
          mode: 'contains',
          value: '违反了OpenAI的内容政策',
        },
        {
          id: 'policy-regex',
          name: 'Policy regex',
          enabled: false,
          field: 'message',
          mode: 'regex',
          value: 'content\\s+policy',
        },
      ])
    )

    assert.equal(rules.length, 3)
    assert.equal(rules[0]?.field, 'status_code')
    assert.equal(rules[1]?.mode, 'contains')
    assert.deepEqual(rules[2]?.values, ['content\\s+policy'])
  })

  test('keeps independent match values and embedded newlines intact', () => {
    const rules = parseFailureFilterRules(
      JSON.stringify([
        {
          id: 'multi-value',
          name: 'Multiple values',
          enabled: true,
          field: 'message',
          mode: 'contains',
          values: ['first value', 'second\nline'],
        },
      ])
    )

    assert.deepEqual(rules[0]?.values, ['first value', 'second\nline'])
    const serialized = JSON.parse(serializeFailureFilterRules(rules)) as Array<{
      values: string[]
    }>
    assert.deepEqual(serialized[0]?.values, ['first value', 'second\nline'])
  })

  test('ignores malformed entries and limits the editor to 100 rules', () => {
    const source = Array.from({ length: 101 }, (_, index) => ({
      id: `rule-${index}`,
      name: `Rule ${index}`,
      enabled: true,
      field: 'message',
      mode: 'contains',
      value: 'blocked',
    }))
    source.splice(1, 0, {
      id: 'invalid',
      name: 'Invalid',
      enabled: true,
      field: 'unknown',
      mode: 'contains',
      value: 'blocked',
    })

    assert.equal(parseFailureFilterRules(JSON.stringify(source)).length, 100)
    assert.deepEqual(parseFailureFilterRules('{invalid'), [])
    assert.deepEqual(parseFailureFilterRules('null'), [])
  })

  test('uses backend-compatible rule identifiers', () => {
    assert.equal(isValidFailureFilterRuleId(createFailureFilterRule().id), true)
    assert.equal(isValidFailureFilterRuleId('policy.rule_1-test'), true)
    assert.equal(isValidFailureFilterRuleId('  policy-rule  '), true)
    assert.equal(isValidFailureFilterRuleId('含中文'), false)
    assert.equal(isValidFailureFilterRuleId('contains spaces'), false)
    assert.equal(isValidFailureFilterRuleId('a'.repeat(65)), false)

    const parsed = parseFailureFilterRules(
      JSON.stringify([
        {
          id: '  policy-rule  ',
          name: 'Policy rule',
          enabled: true,
          field: 'message',
          mode: 'contains',
          value: 'blocked',
        },
      ])
    )
    assert.equal(parsed[0]?.id, 'policy-rule')
  })

  test('trims identifiers and names without changing exact match content', () => {
    const rule: FailureFilterRule = {
      id: '  exact-policy  ',
      name: '  Exact policy response  ',
      enabled: true,
      field: 'full_error',
      mode: 'exact',
      values: ['  content must retain surrounding spaces  '],
    }
    const serialized = JSON.parse(
      serializeFailureFilterRules([rule])
    ) as Array<{
      id: string
      name: string
      value: string
      values: string[]
    }>

    assert.equal(serialized[0]?.id, 'exact-policy')
    assert.equal(serialized[0]?.name, 'Exact policy response')
    assert.equal(
      serialized[0]?.value,
      '  content must retain surrounding spaces  '
    )
    assert.deepEqual(serialized[0]?.values, [
      '  content must retain surrounding spaces  ',
    ])
  })
})
