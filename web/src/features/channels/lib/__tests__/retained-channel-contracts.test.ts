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

import { channelSchema } from '../../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from '../channel-form'
import {
  aggregateChannelsByTag,
  buildGroupDisplayNameMap,
  parseGroupsList,
} from '../channel-utils'
import { categorizeModels, getModelCategory } from '../model-categories'

const channel = channelSchema.parse({
  id: 7,
  type: 14,
  vendor_id: 3,
  key: '',
  status: 1,
  name: 'Anthropic retained',
  weight: 0,
  concurrency_limit: 12,
  created_time: 1,
  test_time: 0,
  response_time: 0,
  balance_updated_time: 0,
  group: 'default,vip',
  group_ids: [1, 9],
  group_details: [
    { id: 1, code: 'default', name: 'Default' },
    { id: 9, code: 'vip', name: 'Premium' },
  ],
  settings: JSON.stringify({
    unrelated: 'preserved',
    claude_code_fingerprint_enabled: true,
    claude_code_transport_fingerprint_enabled: true,
    claude_code_version: '2.1.0',
    claude_code_entrypoint: 'cli',
    monitor_enabled: false,
    monitor_test_interval_minutes: 5,
    monitor_auto_disable_enabled: false,
    monitor_disable_threshold: 3,
  }),
})

describe('retained channel contracts', () => {
  test('maps stable identities, concurrency, fingerprints, and tri-state monitors', () => {
    expect(transformChannelToFormDefaults(channel)).toMatchObject({
      vendor_id: 3,
      group: ['default', 'vip'],
      group_ids: [1, 9],
      concurrency_limit: 12,
      claude_code_fingerprint_enabled: true,
      claude_code_transport_fingerprint_enabled: true,
      claude_code_version: '2.1.0',
      claude_code_entrypoint: 'cli',
      monitor_enabled: 'disabled',
      monitor_test_interval_minutes: '5',
      monitor_auto_disable_enabled: 'disabled',
      monitor_disable_threshold: '3',
    })
  })

  test('preserves unrelated settings and removes inherited monitor overrides', () => {
    const defaults = transformChannelToFormDefaults(channel)
    const payload = transformFormDataToUpdatePayload(
      {
        ...defaults,
        monitor_enabled: 'inherit',
        monitor_auto_disable_enabled: 'disabled',
        monitor_test_interval_minutes: '',
        concurrency_limit: 0,
      },
      channel.id
    )
    const settings = JSON.parse(payload.settings || '{}')

    expect(payload).toMatchObject({
      vendor_id: 3,
      group: 'default,vip',
      group_ids: [1, 9],
      concurrency_limit: 0,
    })
    expect(settings.unrelated).toBe('preserved')
    expect(settings.monitor_enabled).toBeUndefined()
    expect(settings.monitor_test_interval_minutes).toBeUndefined()
    expect(settings.monitor_auto_disable_enabled).toBe(false)
    expect(settings.claude_code_version).toBe('2.1.0')
  })

  test('serializes a selected vendor when creating a channel', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Vendor channel',
      type: 14,
      vendor_id: 3,
      key: 'secret',
      models: 'claude-3-7-sonnet',
      group: ['default'],
      status: 1,
    })

    expect(payload.channel.vendor_id).toBe(3)
  })

  test('serializes an empty vendor selection as null when updating a channel', () => {
    const payload = transformFormDataToUpdatePayload(
      {
        ...CHANNEL_FORM_DEFAULT_VALUES,
        name: 'Unassigned channel',
        type: 14,
        vendor_id: undefined,
      },
      7
    )

    expect(payload.vendor_id).toBeNull()
  })

  test('classifies Qwen TTS before the catch-all category', () => {
    expect(getModelCategory('qwen3-tts-flash')).toBe('Qwen')
    expect(categorizeModels(['qwen-tts', 'gpt-4.1'])).toEqual({
      Qwen: ['qwen-tts'],
      OpenAI: ['gpt-4.1'],
    })
  })

  test('keeps zero concurrency as an intentional unlimited value', () => {
    expect(CHANNEL_FORM_DEFAULT_VALUES.concurrency_limit).toBe(0)
  })

  test('maps each channel group to display name and falls back to code', () => {
    const labels = buildGroupDisplayNameMap([
      { id: 1, code: 'default', name: '默认分组' },
      { id: 2, code: 'group_2', name: 'Tokens-Pro 生图专用' },
      { id: 3, code: 'missing-name', name: 'missing-name' },
    ])
    const rendered = parseGroupsList(
      'group_2,missing-name,deleted-code,default'
    ).map((group) => labels.get(group) || group)

    expect(rendered).toEqual([
      '默认分组',
      'deleted-code',
      'Tokens-Pro 生图专用',
      'missing-name',
    ])
    expect(labels.get('group_2')).toBe('Tokens-Pro 生图专用')
    expect(labels.get('missing-name')).toBeUndefined()
  })

  test('tag aggregate rows keep display names for groups from every child', () => {
    const imageChannel = channelSchema.parse({
      ...channel,
      id: 8,
      tag: 'shared',
      group: 'image',
      group_ids: [2],
      group_details: [{ id: 2, code: 'image', name: '生图专用' }],
    })
    const audioChannel = channelSchema.parse({
      ...channel,
      id: 9,
      tag: 'shared',
      group: 'audio',
      group_ids: [3],
      group_details: [{ id: 3, code: 'audio', name: '音频专用' }],
    })

    const [tagRow] = aggregateChannelsByTag([imageChannel, audioChannel])
    const labels = buildGroupDisplayNameMap(tagRow.group_details)
    const rendered = parseGroupsList(tagRow.group).map(
      (group) => labels.get(group) || group
    )

    expect(tagRow.group).toBe('image,audio')
    expect(rendered).toEqual(['音频专用', '生图专用'])
    expect(tagRow.group_ids).toEqual([2, 3])
  })
})
