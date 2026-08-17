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
import { describe, expect, it } from 'vitest'

import {
  getBuiltinPolicyScopeValidationError,
  normalizeBuiltinPolicyScope,
  setBuiltinPolicyTargetType,
} from './builtin-policy-scope'

describe('cyber_policy audit target scope', () => {
  it('defaults legacy or invalid target types to all channels', () => {
    expect(normalizeBuiltinPolicyScope({})).toEqual({
      upstream_policy_target_type: 'all',
      upstream_policy_channel_ids: [],
      upstream_policy_group_codes: [],
    })
    expect(
      normalizeBuiltinPolicyScope({
        upstream_policy_target_type: 'invalid' as 'all',
      }).upstream_policy_target_type
    ).toBe('all')
  })

  it('normalizes duplicate channel IDs and stable group codes', () => {
    expect(
      normalizeBuiltinPolicyScope({
        upstream_policy_target_type: 'groups',
        upstream_policy_channel_ids: [9, 3, 9, 0],
        upstream_policy_group_codes: [' vip ', 'default', 'vip', '', 'auto'],
      })
    ).toEqual({
      upstream_policy_target_type: 'groups',
      upstream_policy_channel_ids: [3, 9],
      upstream_policy_group_codes: ['default', 'vip'],
    })
  })

  it('keeps inactive selections when switching target types', () => {
    const initial = normalizeBuiltinPolicyScope({
      upstream_policy_target_type: 'channels',
      upstream_policy_channel_ids: [3, 9],
      upstream_policy_group_codes: ['vip'],
    })
    const groups = setBuiltinPolicyTargetType(initial, 'groups')
    const all = setBuiltinPolicyTargetType(groups, 'all')

    expect(groups.upstream_policy_channel_ids).toEqual([3, 9])
    expect(groups.upstream_policy_group_codes).toEqual(['vip'])
    expect(all.upstream_policy_channel_ids).toEqual([3, 9])
    expect(all.upstream_policy_group_codes).toEqual(['vip'])
  })

  it('requires a target for channel and group modes', () => {
    const emptyChannels = normalizeBuiltinPolicyScope({
      upstream_policy_target_type: 'channels',
    })
    const emptyGroups = normalizeBuiltinPolicyScope({
      upstream_policy_target_type: 'groups',
    })
    const all = normalizeBuiltinPolicyScope({
      upstream_policy_target_type: 'all',
    })

    expect(getBuiltinPolicyScopeValidationError(emptyChannels)).toBe('channels')
    expect(getBuiltinPolicyScopeValidationError(emptyGroups)).toBe('groups')
    expect(getBuiltinPolicyScopeValidationError(all)).toBe(null)
  })
})
