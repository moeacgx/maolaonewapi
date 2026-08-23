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
  transformChannelToFormDefaults,
  transformFormDataToUpdatePayload,
} from '../channel-form'

const channel = channelSchema.parse({
  id: 420,
  type: 24,
  key: '',
  status: 1,
  name: 'Gemini dynamic route',
  created_time: 1,
  test_time: 0,
  response_time: 0,
  balance_updated_time: 0,
  group: 'default',
  setting: JSON.stringify({
    dynamic_routing: {
      rules: [
        {
          id: 'gemini-high',
          enabled: true,
          source_model: 'gemini-3.7-flash',
          target_model: 'gemini-3.7-flash-high',
          conditions: [
            {
              field: 'reasoning_effort',
              operator: 'equals',
              value: 'high',
            },
          ],
        },
      ],
    },
  }),
})

describe('channel dynamic routing settings', () => {
  test('round-trips the channel override through the form setting JSON', () => {
    const defaults = transformChannelToFormDefaults(channel)

    expect(defaults.dynamic_routing).toEqual({
      rules: [
        {
          id: 'gemini-high',
          enabled: true,
          action: 'model_redirect',
          source_model: 'gemini-3.7-flash',
          target_model: 'gemini-3.7-flash-high',
          conditions: [
            {
              field: 'reasoning_effort',
              operator: 'equals',
              value: 'high',
            },
          ],
          priority: 0,
        },
      ],
    })

    const payload = transformFormDataToUpdatePayload(defaults, channel.id)
    expect(JSON.parse(payload.setting || '{}')).toMatchObject({
      dynamic_routing: defaults.dynamic_routing,
    })
  })

  test('removes an empty inherited override instead of saving a shadow config', () => {
    const defaults = transformChannelToFormDefaults(channel)
    const payload = transformFormDataToUpdatePayload(
      { ...defaults, dynamic_routing: undefined },
      channel.id
    )

    expect(JSON.parse(payload.setting || '{}').dynamic_routing).toBeUndefined()
  })
})
