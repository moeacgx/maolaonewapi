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
} from './event-routing-display.ts'

const event = {
  stage: 'response',
  channel_id: 12,
  channel_name: 'Primary',
  channel_groups: [
    { id: 4, code: 'vip', name: 'VIP' },
    { id: 4, code: 'vip', name: 'VIP' },
  ],
  group_id: 9,
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
      code: '',
      name: 'Legacy',
      source: 'route',
    })
    assert.equal(
      formatAuditGroupReference(getAuditRouteGroupReference(event)!),
      'Legacy (#9)'
    )
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
      'Legacy (#9)'
    )
  })
})
