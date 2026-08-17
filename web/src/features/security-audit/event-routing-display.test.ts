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
  formatAuditGroupReference,
  getAuditChannelGroupReferences,
  getAuditChannelReference,
  getAuditRouteGroupReference,
  getAuditTokenGroupReference,
} from './event-routing-display'

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
  it('shows an assigned channel with its name and ID', () => {
    expect(getAuditChannelReference(event)).toEqual({
      kind: 'assigned',
      id: 12,
      name: 'Primary',
    })
  })

  it('distinguishes pre-routing events from historical missing data', () => {
    expect(
      getAuditChannelReference({
        ...event,
        stage: 'http',
        channel_id: 0,
        channel_name: '',
      })
    ).toEqual({ kind: 'unassigned' })
    expect(
      getAuditChannelReference({
        ...event,
        stage: 'request',
        channel_id: 0,
        channel_name: '',
      })
    ).toEqual({ kind: 'unassigned' })
    expect(
      getAuditChannelReference({
        ...event,
        stage: 'response',
        channel_id: 0,
        channel_name: '',
      })
    ).toEqual({ kind: 'historical' })
  })

  it('shows the actual route group independently from channel groups', () => {
    const routeGroup = getAuditRouteGroupReference(event)
    expect(routeGroup).toEqual({
      id: 9,
      code: 'legacy',
      name: 'Legacy',
      source: 'route',
    })
    if (!routeGroup) throw new Error('Expected the route group snapshot')
    expect(formatAuditGroupReference(routeGroup)).toBe('Legacy (legacy · #9)')
  })

  it('shows a stable route code when name and ID are unavailable', () => {
    const routeGroup = getAuditRouteGroupReference({
      ...event,
      group_id: 0,
      group_code: 'vip',
      group_name: '',
    })
    expect(routeGroup).toEqual({
      id: 0,
      code: 'vip',
      name: '',
      source: 'route',
    })
    if (!routeGroup) throw new Error('Expected the route group code snapshot')
    expect(formatAuditGroupReference(routeGroup)).toBe('vip')
  })

  it('removes duplicate channel-group snapshots', () => {
    const groups = getAuditChannelGroupReferences(event)
    expect(groups.length).toBe(1)
    const group = groups[0]
    expect(group?.source).toBe('channel')
    if (!group) throw new Error('Expected one channel-group snapshot')
    expect(formatAuditGroupReference(group)).toBe('VIP (vip · #4)')
  })

  it('does not replace the route group with channel-group snapshots', () => {
    const withoutChannelGroups = {
      ...event,
      channel_groups: [],
    }
    expect(getAuditChannelGroupReferences(withoutChannelGroups)).toEqual([])
    const routeGroup = getAuditRouteGroupReference(withoutChannelGroups)
    if (!routeGroup) throw new Error('Expected the historical route snapshot')
    expect(formatAuditGroupReference(routeGroup)).toBe('Legacy (legacy · #9)')
  })

  it('shows every explicitly bound token group and removes duplicates', () => {
    expect(getAuditTokenGroupReference(event)).toEqual({
      kind: 'configured',
      mode: 'explicit',
      groups: [
        { id: 7, code: 'hack', name: 'Hack', source: 'token' },
        { id: 8, code: 'value', name: 'Value', source: 'token' },
      ],
    })
  })

  it('shows auto without inferring groups from the final route', () => {
    expect(
      getAuditTokenGroupReference({
        token_group_mode: 'auto',
        token_groups: [],
      })
    ).toEqual({ kind: 'auto' })
  })

  it('shows requests without a real token as unbound', () => {
    expect(
      getAuditTokenGroupReference({
        token_group_mode: 'none',
        token_groups: [],
      })
    ).toEqual({ kind: 'unbound' })
  })

  it('keeps inherited groups as an event-time snapshot', () => {
    expect(
      getAuditTokenGroupReference({
        token_group_mode: 'inherit',
        token_groups: [{ id: 3, code: 'default', name: 'Default' }],
      })
    ).toEqual({
      kind: 'configured',
      mode: 'inherit',
      groups: [{ id: 3, code: 'default', name: 'Default', source: 'token' }],
    })
  })

  it('marks historical events without token-group snapshots', () => {
    expect(
      getAuditTokenGroupReference({
        token_group_mode: '',
        token_groups: [],
      })
    ).toEqual({ kind: 'historical' })
    expect(
      getAuditTokenGroupReference({
        token_group_mode: 'future-mode',
        token_groups: [],
      })
    ).toEqual({ kind: 'historical' })
  })
})
