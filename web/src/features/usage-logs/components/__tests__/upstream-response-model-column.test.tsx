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
*/
import { renderHook } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import type { UsageLog } from '../../data/schema'
import { useCommonLogsColumns } from '../columns/common-logs-columns'

vi.mock('@lobehub/icons', () => ({}))
vi.mock('@/assets/custom/icon-sub2api', () => ({ IconSub2api: () => null }))

const log: UsageLog = {
  id: 1,
  user_id: 2,
  created_at: 1,
  type: 2,
  content: '',
  username: 'alice',
  token_name: 'token',
  model_name: 'requested-model',
  quota: 1,
  prompt_tokens: 1,
  completion_tokens: 1,
  use_time: 1,
  is_stream: false,
  channel: 3,
  channel_name: 'channel',
  token_id: 4,
  group: 'default',
  group_name: 'Default',
  ip: '',
  other: '{"upstream_response_model_name":"provider-actual"}',
  request_id: 'request-1',
  upstream_request_id: 'upstream-1',
}

describe('usage log upstream response model column', () => {
  test('is present only in the administrator column set and reads the response model', () => {
    const { result: adminResult } = renderHook(() => useCommonLogsColumns(true))
    const adminColumn = adminResult.current.find(
      (column) => column.id === 'upstream_response_model_name'
    )

    expect(adminColumn).toBeDefined()
    expect(adminColumn?.header).toBe('Upstream Response Model')
    if (!adminColumn || !('accessorFn' in adminColumn)) {
      throw new Error('upstream response model column must expose accessorFn')
    }
    expect(adminColumn.accessorFn(log, 0)).toBe('provider-actual')

    const { result: userResult } = renderHook(() => useCommonLogsColumns(false))
    expect(
      userResult.current.some(
        (column) => column.id === 'upstream_response_model_name'
      )
    ).toBe(false)
  })
})
