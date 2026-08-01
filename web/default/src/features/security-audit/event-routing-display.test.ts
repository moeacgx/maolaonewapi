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
  formatAuditGroupReference,
  getAuditChannelGroupReferences,
  getAuditChannelReference,
  getAuditRouteGroupReference,
  getAuditTokenGroupReference,
} from './event-routing-display.ts'

const event = {
  stage: 'response',
  channel_id: 12,
  channel_name: 'Primary',
  channel_groups: [
    { id: 4, code: 'vip', name: 'VIP' },
    { id: 4, code: 'vip', name: 'VIP' },
  ],
  token_group_mode: 'explicit',
  token_groups: [
    { id: 7, code: 'hack', name: 'Hack' },
    { id: 8, code: 'value', name: 'Value' },
    { id: 7, code: 'hack', name: 'Hack' },
  ],
  group_id: 9,
  group_code: 'legacy',
  group_name: 'Legacy',
}

describe('security audit event routing display', () => {
  test('shows an assigned channel with its name and ID', () => {
    assert.deepEqual(getAuditChannelReference(event), {
      kind: 'assigned',
      id: 12,
      name: 'Primary',
    })
  })

  test('distinguishes pre-routing events from historical missing data', () => {
    assert.deepEqual(
      getAuditChannelReference({
        ...event,
        stage: 'http',
        channel_id: 0,
        channel_name: '',
      }),
      { kind: 'unassigned' }
    )
    assert.deepEqual(
      getAuditChannelReference({
        ...event,
        stage: 'request',
        channel_id: 0,
        channel_name: '',
      }),
      { kind: 'unassigned' }
    )
    assert.deepEqual(
      getAuditChannelReference({
        ...event,
        stage: 'response',
        channel_id: 0,
        channel_name: '',
      }),
      { kind: 'historical' }
    )
  })

  test('shows the actual route group independently from channel groups', () => {
    assert.deepEqual(getAuditRouteGroupReference(event), {
      id: 9,
      code: 'legacy',
      name: 'Legacy',
      source: 'route',
    })
    assert.equal(
      formatAuditGroupReference(getAuditRouteGroupReference(event)!),
      'Legacy (legacy · #9)'
    )
  })

  test('shows a stable route code when name and ID are unavailable', () => {
    const routeGroup = getAuditRouteGroupReference({
      ...event,
      group_id: 0,
      group_code: 'vip',
      group_name: '',
    })
    assert.deepEqual(routeGroup, {
      id: 0,
      code: 'vip',
      name: '',
      source: 'route',
    })
    assert.equal(formatAuditGroupReference(routeGroup!), 'vip')
  })

  test('removes duplicate channel-group snapshots', () => {
    const groups = getAuditChannelGroupReferences(event)
    assert.equal(groups.length, 1)
    assert.equal(groups[0]?.source, 'channel')
    assert.equal(formatAuditGroupReference(groups[0]!), 'VIP (vip · #4)')
  })

  test('does not replace the route group with channel-group snapshots', () => {
    const withoutChannelGroups = {
      ...event,
      channel_groups: [],
    }
    assert.deepEqual(getAuditChannelGroupReferences(withoutChannelGroups), [])
    assert.equal(
      formatAuditGroupReference(
        getAuditRouteGroupReference(withoutChannelGroups)!
      ),
      'Legacy (legacy · #9)'
    )
  })

  test('shows every explicitly bound token group and removes duplicates', () => {
    assert.deepEqual(getAuditTokenGroupReference(event), {
      kind: 'configured',
      mode: 'explicit',
      groups: [
        { id: 7, code: 'hack', name: 'Hack', source: 'token' },
        { id: 8, code: 'value', name: 'Value', source: 'token' },
      ],
    })
  })

  test('shows auto without inferring groups from the final route', () => {
    assert.deepEqual(
      getAuditTokenGroupReference({
        token_group_mode: 'auto',
        token_groups: [],
      }),
      { kind: 'auto' }
    )
  })

  test('shows requests without a real token as unbound', () => {
    assert.deepEqual(
      getAuditTokenGroupReference({
        token_group_mode: 'none',
        token_groups: [],
      }),
      { kind: 'unbound' }
    )
  })

  test('keeps inherited groups as an event-time snapshot', () => {
    assert.deepEqual(
      getAuditTokenGroupReference({
        token_group_mode: 'inherit',
        token_groups: [{ id: 3, code: 'default', name: 'Default' }],
      }),
      {
        kind: 'configured',
        mode: 'inherit',
        groups: [{ id: 3, code: 'default', name: 'Default', source: 'token' }],
      }
    )
  })

  test('marks historical events without token-group snapshots', () => {
    assert.deepEqual(
      getAuditTokenGroupReference({
        token_group_mode: '',
        token_groups: [],
      }),
      { kind: 'historical' }
    )
    assert.deepEqual(
      getAuditTokenGroupReference({
        token_group_mode: 'future-mode',
        token_groups: [],
      }),
      { kind: 'historical' }
    )
  })
})
