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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  ACTION_BLOCK,
  getEmptySensitiveRuleTarget,
  includeMissingSensitiveRouteOptions,
  includeMissingSensitiveTagOptions,
  parseSensitiveRuleChannelIds,
  parseSensitiveRulesConfig,
  serializeSensitiveRules,
  TARGET_CHANNEL_TAGS,
  TARGET_CHANNELS,
} from './sensitive-rule-config.ts'

describe('sensitive rule routing targets', () => {
  test('materializes legacy global channels into rules without target_type', () => {
    const drafts = parseSensitiveRulesConfig(
      JSON.stringify({
        rules: [
          {
            id: 'legacy',
            name: 'Legacy',
            enabled: true,
            action: ACTION_BLOCK,
            keywords: ['blocked'],
          },
        ],
      }),
      '',
      [19, 3, 19]
    )

    assert.equal(drafts.length, 1)
    assert.equal(drafts[0]?.targetType, TARGET_CHANNELS)
    assert.deepEqual(drafts[0]?.channelIds, [3, 19])

    const saved = JSON.parse(serializeSensitiveRules(drafts)) as {
      rules: Array<{
        target_type: string
        channel_ids: number[]
        channel_tags?: string[]
      }>
    }
    assert.equal(saved.rules[0]?.target_type, TARGET_CHANNELS)
    assert.deepEqual(saved.rules[0]?.channel_ids, [3, 19])
    assert.equal(saved.rules[0]?.channel_tags, undefined)
  })

  test('keeps channel management tags separate and normalized', () => {
    const drafts = parseSensitiveRulesConfig(
      JSON.stringify({
        rules: [
          {
            id: 'groups',
            name: 'Groups',
            enabled: true,
            action: ACTION_BLOCK,
            keywords: ['blocked'],
            group_refs: ['keyword-library'],
            target_type: TARGET_CHANNEL_TAGS,
            channel_tags: [' backup ', 'primary', 'backup', ''],
          },
        ],
      }),
      '',
      [99]
    )

    assert.equal(drafts[0]?.targetType, TARGET_CHANNEL_TAGS)
    assert.deepEqual(drafts[0]?.channelIds, [])
    assert.deepEqual(drafts[0]?.channelTags, ['backup', 'primary'])
    assert.deepEqual(drafts[0]?.groupRefs, ['keyword-library'])

    const saved = JSON.parse(serializeSensitiveRules(drafts)) as {
      rules: Array<{
        target_type: string
        channel_ids?: number[]
        channel_tags: string[]
        group_refs: string[]
      }>
    }
    assert.equal(saved.rules[0]?.target_type, TARGET_CHANNEL_TAGS)
    assert.equal(saved.rules[0]?.channel_ids, undefined)
    assert.deepEqual(saved.rules[0]?.channel_tags, ['backup', 'primary'])
    assert.deepEqual(saved.rules[0]?.group_refs, ['keyword-library'])
  })

  test('requires a target only for enabled rules that will be serialized', () => {
    const [rule] = parseSensitiveRulesConfig(
      JSON.stringify({
        rules: [
          {
            id: 'empty-target',
            name: 'Empty target',
            enabled: true,
            action: ACTION_BLOCK,
            keywords: ['blocked'],
            target_type: TARGET_CHANNELS,
            channel_ids: [],
          },
        ],
      }),
      '',
      []
    )
    assert.ok(rule)
    assert.equal(getEmptySensitiveRuleTarget(rule), TARGET_CHANNELS)

    assert.equal(
      getEmptySensitiveRuleTarget({
        ...rule,
        enabled: false,
      }),
      null
    )
    assert.equal(
      getEmptySensitiveRuleTarget({
        ...rule,
        keywordsText: '',
        groupRefs: [],
      }),
      null
    )
  })

  test('normalizes legacy channel option values', () => {
    assert.deepEqual(
      parseSensitiveRuleChannelIds('[19,"3",19,0,-1,"invalid"]'),
      [3, 19]
    )
    assert.deepEqual(parseSensitiveRuleChannelIds('{"id":1}'), [])
    assert.deepEqual(parseSensitiveRuleChannelIds('invalid json'), [])
  })

  test('keeps missing selected IDs visible so they can be removed', () => {
    assert.deepEqual(
      includeMissingSensitiveRouteOptions(
        [{ value: '3', label: 'Channel 3 #3' }],
        [19, 3],
        'Unavailable channel'
      ),
      [
        { value: '3', label: 'Channel 3 #3' },
        { value: '19', label: 'Unavailable channel #19' },
      ]
    )
  })

  test('keeps missing selected channel tags visible so they can be removed', () => {
    assert.deepEqual(
      includeMissingSensitiveTagOptions(
        [{ value: 'primary', label: 'primary (2)' }],
        ['backup', 'primary'],
        'Unavailable channel group'
      ),
      [
        { value: 'primary', label: 'primary (2)' },
        {
          value: 'backup',
          label: 'Unavailable channel group: backup',
        },
      ]
    )
  })
})
