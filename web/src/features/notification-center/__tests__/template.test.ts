/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import { describe, expect, test } from 'vitest'

import { shouldReplaceNotificationTemplate } from '../template'

describe('notification template event transitions', () => {
  test('replaces an empty or unchanged invoice default when switching events', () => {
    const invoiceEvent = {
      default_template: '{{mention}} {{invoice_id}} {{total_amount}}',
    }
    const channelEvent = {
      default_template: '{{mention}} {{channel_name}}',
    }

    expect(shouldReplaceNotificationTemplate('', invoiceEvent)).toBe(true)
    expect(
      shouldReplaceNotificationTemplate(
        ' {{mention}} {{invoice_id}} {{total_amount}}\n',
        invoiceEvent
      )
    ).toBe(true)

    const nextTemplate = shouldReplaceNotificationTemplate(
      invoiceEvent.default_template,
      invoiceEvent
    )
      ? channelEvent.default_template
      : invoiceEvent.default_template
    expect(nextTemplate).toBe(channelEvent.default_template)
  })

  test('preserves a customized template', () => {
    expect(
      shouldReplaceNotificationTemplate('{{mention}} custom', {
        default_template: '{{mention}} invoice',
      })
    ).toBe(false)
  })
})
