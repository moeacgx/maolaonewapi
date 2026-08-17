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
  transformFormDataToUpdatePayload,
} from '../channel-form'
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
})
