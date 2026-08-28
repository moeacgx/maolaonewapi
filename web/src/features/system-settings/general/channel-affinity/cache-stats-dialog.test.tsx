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
import { render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getAffinityUsageCache } = vi.hoisted(() => ({
  getAffinityUsageCache: vi.fn(),
}))

vi.mock('./api', () => ({ getAffinityUsageCache }))
vi.mock('@/components/dialog', () => ({
  Dialog: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}))
vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

import { CacheStatsDialog } from './cache-stats-dialog'

const target = {
  rule_name: 'cache-rule',
  using_group: 'default',
  key_hint: 'se...on',
  key_fp: 'a1b2c3d4',
}

describe('渠道亲和性缓存命中详情弹窗', () => {
  beforeEach(() => {
    getAffinityUsageCache.mockReset()
  })

  it('保留明确的零值并忽略缺失 token 字段', async () => {
    getAffinityUsageCache.mockResolvedValue({
      success: true,
      data: {
        rule_name: 'cache-rule',
        using_group: 'default',
        key_fp: 'a1b2c3d4',
        cached_token_rate_mode: 'cached_over_prompt',
        prompt_tokens: 0,
        cached_tokens: 0,
      },
    })

    render(<CacheStatsDialog open onOpenChange={vi.fn()} target={target} />)

    await waitFor(() => expect(screen.getByText('Prompt tokens')).toBeVisible())
    expect(screen.getByText('Cached tokens')).toBeVisible()
    expect(screen.queryByText('Completion tokens')).not.toBeInTheDocument()
  })

  it('关闭弹窗时清理 loading，并忽略旧请求结果', async () => {
    let resolveRequest: (value: unknown) => void = () => undefined
    getAffinityUsageCache.mockReturnValue(
      new Promise((resolve) => {
        resolveRequest = resolve
      })
    )

    const rendered = render(
      <CacheStatsDialog open onOpenChange={vi.fn()} target={target} />
    )
    expect(screen.getByText('Loading...')).toBeVisible()

    rendered.rerender(
      <CacheStatsDialog open={false} onOpenChange={vi.fn()} target={null} />
    )
    expect(screen.queryByText('Loading...')).not.toBeInTheDocument()
    expect(screen.getByText('No data available')).toBeVisible()

    resolveRequest({
      success: true,
      data: { rule_name: 'stale', key_fp: 'stale' },
    })
    await waitFor(() =>
      expect(screen.queryByText('stale')).not.toBeInTheDocument()
    )
  })
})
