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
import { describe, expect, test } from 'vitest'

import type { DynamicRoutingRule } from '../../types'
import {
  buildDynamicRoutingChannelConfig,
  parseDynamicRoutingRules,
  validateDynamicRoutingRules,
} from '../rules'

function rule(overrides: Partial<DynamicRoutingRule> = {}): DynamicRoutingRule {
  return {
    id: 'gemini-high',
    enabled: true,
    action: 'model_redirect',
    source_model: 'gemini-3.7-flash',
    target_model: 'gemini-3.7-flash-high',
    priority: 10,
    ...overrides,
  }
}

describe('dynamic routing rules', () => {
  test('preserves inherited channel activation when a channel supplies rules', () => {
    expect(buildDynamicRoutingChannelConfig('inherit', [rule()])).toEqual({
      rules: [rule()],
    })
    expect(buildDynamicRoutingChannelConfig('inherit', [])).toBeUndefined()
    expect(buildDynamicRoutingChannelConfig('disabled', [])).toEqual({
      enabled: false,
    })
  })

  test('rejects duplicate IDs among enabled rules but allows a disabled draft', () => {
    expect(validateDynamicRoutingRules([rule(), rule()])).toBe(
      'Enabled dynamic routing rule IDs must be unique.'
    )
    expect(
      validateDynamicRoutingRules([
        rule(),
        rule({ id: '', enabled: false, source_model: '', target_model: '' }),
      ])
    ).toBeNull()
  })

  test('validates endpoint and request-field contracts before save', () => {
    expect(
      validateDynamicRoutingRules([rule({ request_paths: ['v1/responses'] })])
    ).toBe(
      'Dynamic routing request paths must start with "/" and cannot contain query strings.'
    )
    expect(
      validateDynamicRoutingRules([
        rule({
          conditions: [
            {
              field: 'request.metadata[0]',
              operator: 'equals',
              value: 'high',
            },
          ],
        }),
      ])
    ).toBe(
      'Dynamic routing condition fields must be reasoning_effort or request.<simple_json_path>.'
    )
  })

  test('normalizes persisted rule arrays without trusting malformed values', () => {
    expect(
      parseDynamicRoutingRules([
        {
          id: '  gemini-high  ',
          enabled: true,
          source_model: ' gemini-3.7-flash ',
          target_model: ' gemini-3.7-flash-high ',
          channel_types: [24, 24, 'not-a-type'],
          request_paths: ['/v1/responses', '/v1/responses'],
          conditions: [
            {
              field: ' reasoning_effort ',
              operator: 'exists',
              value: 'ignored',
            },
          ],
        },
        null,
      ])
    ).toEqual([
      {
        id: 'gemini-high',
        enabled: true,
        action: 'model_redirect',
        source_model: 'gemini-3.7-flash',
        target_model: 'gemini-3.7-flash-high',
        channel_types: [24],
        request_paths: ['/v1/responses'],
        conditions: [{ field: 'reasoning_effort', operator: 'exists' }],
        priority: 0,
      },
    ])
  })
})
