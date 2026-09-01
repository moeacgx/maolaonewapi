import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import type { BenefitActivity, BenefitReport } from '../../types'
import { BenefitActivityReport } from '../benefit-activity-report'

type GetMethod = (url: string) => Promise<{ data: unknown }>
type MockableApi = { get: GetMethod }

const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get

function activity(overrides: Partial<BenefitActivity> = {}): BenefitActivity {
  return {
    id: 1,
    name: 'Weekend Boost',
    description: '',
    group_id: 7,
    group_code_snapshot: 'default',
    group_name_snapshot: 'Default',
    status: 'published',
    amount_mode: 'fixed',
    amount_display_type: 'USD',
    total_amount: 10,
    total_quota: 5000000,
    fixed_quota: 500000,
    min_quota: 0,
    max_quota: 0,
    total_count: 10,
    fixed_amount: 1,
    min_amount: 0,
    max_amount: 0,
    claim_paid_threshold: 0,
    personal_valid_hours: 24,
    starts_at: 1,
    ends_at: 9999999999,
    published_at: 1,
    ...overrides,
  }
}

function report(overrides: Partial<BenefitReport> = {}): BenefitReport {
  return {
    total_quota: 5000000,
    undistributed_quota: 1000000,
    distributed_quota: 4000000,
    used_quota: 2000000,
    expired_unused_quota: 500000,
    total_count: 10,
    distributed_count: 8,
    used_count: 5,
    expired_count: 1,
    ...overrides,
  }
}

function renderReport(props: {
  activity: BenefitActivity | null
  open: boolean
}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <BenefitActivityReport
        activity={props.activity}
        open={props.open}
        onOpenChange={() => undefined}
      />
    </QueryClientProvider>
  )
}

afterEach(() => {
  apiClient.get = originalGet
})

describe('benefit activity report', () => {
  it('renders the real backend count fields (total/distributed/used/expired_count)', async () => {
    apiClient.get = async (url) => {
      if (url === '/api/benefit/admin/activities/1/report') {
        return { data: { success: true, data: report() } }
      }
      throw new Error(`Unexpected GET ${url}`)
    }

    renderReport({ activity: activity(), open: true })

    await waitFor(() =>
      expect(screen.getByText('Total shares: 10')).toBeTruthy()
    )
    expect(screen.getByText('Distributed shares: 8')).toBeTruthy()
    expect(screen.getByText('Used-up shares: 5')).toBeTruthy()
    expect(screen.getByText('Expired shares: 1')).toBeTruthy()
  })

  it('never renders raw quota digits for the report amounts', async () => {
    apiClient.get = async () => ({ data: { success: true, data: report() } })

    renderReport({ activity: activity(), open: true })

    await waitFor(() =>
      expect(screen.getByText('Total shares: 10')).toBeTruthy()
    )
    expect(screen.queryByText('5000000')).toBeNull()
    expect(screen.queryByText('4000000')).toBeNull()
    expect(screen.queryByText('2000000')).toBeNull()
  })

  it('shows a retry action when the report request fails', async () => {
    let attempts = 0
    apiClient.get = async () => {
      attempts += 1
      if (attempts === 1) throw new Error('network down')
      return { data: { success: true, data: report() } }
    }
    const user = userEvent.setup()

    renderReport({ activity: activity(), open: true })

    await waitFor(() =>
      expect(screen.getByText('Failed to load benefit report')).toBeTruthy()
    )
    await user.click(screen.getByRole('button', { name: 'Retry' }))

    await waitFor(() =>
      expect(screen.getByText('Total shares: 10')).toBeTruthy()
    )
    expect(attempts).toBe(2)
  })

  it('does not fetch the report when there is no activity selected', () => {
    const get = vi
      .fn()
      .mockResolvedValue({ data: { success: true, data: report() } })
    apiClient.get = get

    renderReport({ activity: null, open: false })

    expect(get).not.toHaveBeenCalled()
  })
})
