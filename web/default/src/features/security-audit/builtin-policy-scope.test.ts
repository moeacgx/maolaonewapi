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
  getBuiltinPolicyScopeValidationError,
  normalizeBuiltinPolicyScope,
  setBuiltinPolicyTargetType,
} from './builtin-policy-scope.ts'

describe('cyber_policy audit target scope', () => {
  test('defaults legacy or invalid target types to all channels', () => {
    assert.deepEqual(normalizeBuiltinPolicyScope({}), {
      upstream_policy_target_type: 'all',
      upstream_policy_channel_ids: [],
      upstream_policy_group_codes: [],
    })
    assert.equal(
      normalizeBuiltinPolicyScope({
        upstream_policy_target_type: 'invalid' as 'all',
      }).upstream_policy_target_type,
      'all'
    )
  })

  test('normalizes duplicate channel IDs and stable group codes', () => {
    assert.deepEqual(
      normalizeBuiltinPolicyScope({
        upstream_policy_target_type: 'groups',
        upstream_policy_channel_ids: [9, 3, 9, 0],
        upstream_policy_group_codes: [' vip ', 'default', 'vip', '', 'auto'],
      }),
      {
        upstream_policy_target_type: 'groups',
        upstream_policy_channel_ids: [3, 9],
        upstream_policy_group_codes: ['default', 'vip'],
      }
    )
  })

  test('keeps inactive selections when switching target types', () => {
    const initial = normalizeBuiltinPolicyScope({
      upstream_policy_target_type: 'channels',
      upstream_policy_channel_ids: [3, 9],
      upstream_policy_group_codes: ['vip'],
    })
    const groups = setBuiltinPolicyTargetType(initial, 'groups')
    const all = setBuiltinPolicyTargetType(groups, 'all')

    assert.deepEqual(groups.upstream_policy_channel_ids, [3, 9])
    assert.deepEqual(groups.upstream_policy_group_codes, ['vip'])
    assert.deepEqual(all.upstream_policy_channel_ids, [3, 9])
    assert.deepEqual(all.upstream_policy_group_codes, ['vip'])
  })

  test('requires a target for channel and group modes', () => {
    const emptyChannels = normalizeBuiltinPolicyScope({
      upstream_policy_target_type: 'channels',
    })
    const emptyGroups = normalizeBuiltinPolicyScope({
      upstream_policy_target_type: 'groups',
    })
    const all = normalizeBuiltinPolicyScope({
      upstream_policy_target_type: 'all',
    })

    assert.equal(
      getBuiltinPolicyScopeValidationError(emptyChannels),
      'channels'
    )
    assert.equal(getBuiltinPolicyScopeValidationError(emptyGroups), 'groups')
    assert.equal(getBuiltinPolicyScopeValidationError(all), null)
  })
})
