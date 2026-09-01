import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it } from 'vitest'

import { api } from '@/lib/api'

import { UserBenefits } from '../../index'
import type { BenefitActivityUserView, BenefitVoucher } from '../../types'

type ApiMethod = (url: string, data?: unknown) => Promise<{ data: unknown }>
type MockableApi = { get: ApiMethod; post: ApiMethod }

const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get
const originalPost = apiClient.post

function activity(
  overrides: Partial<BenefitActivityUserView> = {}
): BenefitActivityUserView {
  return {
    id: 1,
    name: 'Weekend Boost',
    description: 'Extra credit for the weekend',
    group_id: 7,
    group_code_snapshot: 'default',
    group_name_snapshot: 'Default',
    status: 'published',
    amount_mode: 'fixed',
    amount_display_type: 'USD',
    total_amount: 10,
    total_quota: 5000000,
    total_count: 10,
    fixed_amount: 1,
    min_amount: 0,
    max_amount: 0,
    claim_paid_threshold: 0,
    personal_valid_hours: 24,
    starts_at: 1,
    ends_at: 9999999999,
    published_at: 1,
    eligible: true,
    has_claimed: false,
    single_user_concurrency_limit: 1,
    ...overrides,
  }
}

function voucher(overrides: Partial<BenefitVoucher> = {}): BenefitVoucher {
  return {
    id: 1,
    activity_id: 1,
    user_id: 1,
    original_quota: 500000,
    used_quota: 125000,
    remaining_quota: 375000,
    status: 'active',
    claimed_at: 1,
    expires_at: 9999999999,
    ...overrides,
  }
}

function installApiFixtures(
  activities: BenefitActivityUserView[],
  vouchers: BenefitVoucher[]
) {
  apiClient.get = async (url) => {
    if (url === '/api/benefit/activities') {
      return { data: { success: true, data: activities } }
    }
    if (url === '/api/benefit/vouchers') {
      return { data: { success: true, data: vouchers } }
    }
    throw new Error(`Unexpected GET ${url}`)
  }
}

function renderUserBenefits() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <UserBenefits />
    </QueryClientProvider>
  )
}

afterEach(() => {
  apiClient.get = originalGet
  apiClient.post = originalPost
})

describe('user benefits page', () => {
  it('renders voucher amounts through formatQuota and never as raw quota digits', async () => {
    installApiFixtures([activity({ has_claimed: true })], [voucher()])
    renderUserBenefits()

    await waitFor(() =>
      expect(screen.getAllByText('Weekend Boost').length).toBeGreaterThan(0)
    )

    expect(screen.queryByText('500000')).toBeNull()
    expect(screen.queryByText('125000')).toBeNull()
    expect(screen.queryByText('375000')).toBeNull()
    expect(screen.getAllByText(/\$/).length).toBeGreaterThan(0)
  })

  it('shows an enabled claim button only when eligible and not yet claimed', async () => {
    installApiFixtures(
      [
        activity({
          id: 1,
          name: 'Eligible activity',
          eligible: true,
          has_claimed: false,
        }),
        activity({
          id: 2,
          name: 'Sold out activity',
          eligible: false,
          eligibility_reason: 'sold_out',
          has_claimed: false,
        }),
      ],
      []
    )
    renderUserBenefits()

    await waitFor(() =>
      expect(screen.getByText('Eligible activity')).toBeTruthy()
    )

    const claimButtons = screen.getAllByRole('button', { name: 'Claim' })
    expect(claimButtons).toHaveLength(2)
    const enabledButtons = claimButtons.filter(
      (button) => !button.hasAttribute('disabled')
    )
    expect(enabledButtons).toHaveLength(1)
    expect(screen.getByText('Fully claimed')).toBeTruthy()
  })

  it('shows an already-claimed badge instead of a claim button once claimed', async () => {
    installApiFixtures([activity({ has_claimed: true })], [])
    renderUserBenefits()

    await waitFor(() =>
      expect(screen.getByText('Already claimed')).toBeTruthy()
    )
    expect(screen.queryByRole('button', { name: 'Claim' })).toBeNull()
  })

  it('submits a claim request for the clicked activity', async () => {
    installApiFixtures([activity()], [])
    const claimed: string[] = []
    apiClient.post = async (url) => {
      claimed.push(url)
      return { data: { success: true, data: voucher() } }
    }
    const user = userEvent.setup()
    renderUserBenefits()

    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Claim' })).toBeEnabled()
    )
    await user.click(screen.getByRole('button', { name: 'Claim' }))

    await waitFor(() =>
      expect(claimed).toContain('/api/benefit/activities/1/claim')
    )
  })

  it('shows an empty state when there are no vouchers or activities', async () => {
    installApiFixtures([], [])
    renderUserBenefits()

    await waitFor(() =>
      expect(screen.getByText('No vouchers yet')).toBeTruthy()
    )
    expect(screen.getByText('No benefit activities')).toBeTruthy()
  })

  it('shows a retry action when loading activities or vouchers fails', async () => {
    apiClient.get = async () => ({
      data: { success: false, message: 'boom' },
    })
    renderUserBenefits()

    await waitFor(() =>
      expect(screen.getByText('Unable to load benefit activities')).toBeTruthy()
    )
    expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy()
  })
})
