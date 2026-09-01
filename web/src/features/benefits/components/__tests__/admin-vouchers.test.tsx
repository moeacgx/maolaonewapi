import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it } from 'vitest'

import { api } from '@/lib/api'

import type { BenefitActivity, BenefitVoucherAdminView } from '../../types'
import { BenefitVouchersSheet } from '../benefit-vouchers-sheet'

const { Toaster, toast } = await import('sonner')

type GetMethod = (
  url: string,
  config?: { params?: Record<string, unknown> }
) => Promise<{ data: unknown }>
type PostMethod = (url: string, body?: unknown) => Promise<{ data: unknown }>
type MockableApi = { get: GetMethod; post: PostMethod }

const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get
const originalPost = apiClient.post

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

function voucher(
  overrides: Partial<BenefitVoucherAdminView> = {}
): BenefitVoucherAdminView {
  return {
    id: 1,
    activity_id: 1,
    user_id: 10,
    username: 'alice',
    activity_name: 'Weekend Boost',
    group_name_snapshot: 'Default',
    original_quota: 500000,
    used_quota: 125000,
    remaining_quota: 375000,
    status: 'active',
    claimed_at: 1000,
    expires_at: 9999999999,
    ...overrides,
  }
}

function renderSheet(target: BenefitActivity) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <BenefitVouchersSheet
        activity={target}
        open
        onOpenChange={() => undefined}
      />
      <Toaster duration={60_000} />
    </QueryClientProvider>
  )
}

afterEach(() => {
  apiClient.get = originalGet
  apiClient.post = originalPost
  toast.dismiss()
})

describe('admin voucher list sheet', () => {
  it('renders voucher amounts through formatQuota and never as raw quota digits', async () => {
    apiClient.get = async (url) => {
      if (url === '/api/benefit/admin/activities/1/vouchers') {
        return {
          data: {
            success: true,
            data: { items: [voucher()], total: 1, page: 1, page_size: 20 },
          },
        }
      }
      throw new Error(`Unexpected GET ${url}`)
    }

    renderSheet(activity())

    await waitFor(() => expect(screen.getByText('alice')).toBeTruthy())
    expect(screen.queryByText('500000')).toBeNull()
    expect(screen.queryByText('125000')).toBeNull()
    expect(screen.queryByText('375000')).toBeNull()
  })

  it('sends the keyword and status filters to the admin voucher list route', async () => {
    const requests: Array<Record<string, unknown> | undefined> = []
    apiClient.get = async (url, config) => {
      if (url === '/api/benefit/admin/activities/1/vouchers') {
        requests.push(config?.params)
        return {
          data: {
            success: true,
            data: { items: [voucher()], total: 1, page: 1, page_size: 20 },
          },
        }
      }
      throw new Error(`Unexpected GET ${url}`)
    }
    const user = userEvent.setup()

    renderSheet(activity())
    await waitFor(() => expect(requests.length).toBeGreaterThan(0))

    await user.type(screen.getByLabelText('Search by username'), 'alice')

    await waitFor(() =>
      expect(requests.at(-1)).toMatchObject({ keyword: 'alice' })
    )
  })

  it('only offers batch void for active vouchers and requires a reason', async () => {
    apiClient.get = async (url) => {
      if (url === '/api/benefit/admin/activities/1/vouchers') {
        return {
          data: {
            success: true,
            data: {
              items: [
                voucher({ id: 1, username: 'alice', status: 'active' }),
                voucher({ id: 2, username: 'bob', status: 'exhausted' }),
              ],
              total: 2,
              page: 1,
              page_size: 20,
            },
          },
        }
      }
      throw new Error(`Unexpected GET ${url}`)
    }
    const voidRequests: unknown[] = []
    apiClient.post = async (url, body) => {
      voidRequests.push({ url, body })
      return {
        data: {
          success: true,
          data: { updated_ids: [1], skipped: [] },
        },
      }
    }
    const user = userEvent.setup()

    renderSheet(activity())
    await waitFor(() => expect(screen.getByText('alice')).toBeTruthy())

    expect(
      screen.queryByRole('checkbox', { name: 'Select voucher #2' })
    ).toBeNull()

    await user.click(
      screen.getByRole('checkbox', { name: 'Select voucher #1' })
    )
    await user.click(screen.getByRole('button', { name: /Void selected/ }))

    const confirmButton = screen.getByRole('button', { name: 'Void' })
    expect(confirmButton).toBeDisabled()

    await user.type(screen.getByLabelText('Reason'), 'campaign cleanup')
    expect(confirmButton).toBeEnabled()
    await user.click(confirmButton)

    await waitFor(() => expect(voidRequests).toHaveLength(1))
    expect(voidRequests[0]).toEqual({
      url: '/api/benefit/admin/vouchers/batch-void',
      body: { ids: [1], reason: 'campaign cleanup', confirm: true },
    })
  })

  it('shows an error toast and re-enables the confirm button when batch void throws', async () => {
    apiClient.get = async (url) => {
      if (url === '/api/benefit/admin/activities/1/vouchers') {
        return {
          data: {
            success: true,
            data: {
              items: [voucher({ id: 1, username: 'alice', status: 'active' })],
              total: 1,
              page: 1,
              page_size: 20,
            },
          },
        }
      }
      throw new Error(`Unexpected GET ${url}`)
    }
    apiClient.post = async () => {
      throw new Error('network down')
    }
    const user = userEvent.setup()

    renderSheet(activity())
    await waitFor(() => expect(screen.getByText('alice')).toBeTruthy())

    await user.click(
      screen.getByRole('checkbox', { name: 'Select voucher #1' })
    )
    await user.click(screen.getByRole('button', { name: /Void selected/ }))
    await user.type(screen.getByLabelText('Reason'), 'campaign cleanup')
    const confirmButton = screen.getByRole('button', { name: 'Void' })
    await user.click(confirmButton)

    await waitFor(() => expect(screen.getByText('network down')).toBeTruthy())
    await waitFor(() => expect(confirmButton).toBeEnabled())
  })

  it('shows admin metadata in the voucher ledger, unlike the user-facing ledger', async () => {
    apiClient.get = async (url) => {
      if (url === '/api/benefit/admin/activities/1/vouchers') {
        return {
          data: {
            success: true,
            data: { items: [voucher()], total: 1, page: 1, page_size: 20 },
          },
        }
      }
      if (url === '/api/benefit/admin/vouchers/1/ledger') {
        return {
          data: {
            success: true,
            data: [
              {
                id: 1,
                activity_id: 1,
                voucher_id: 1,
                user_id: 10,
                request_id: '',
                log_id: 0,
                type: 'void',
                quota_delta: -375000,
                balance_after: 0,
                created_at: 5000,
                metadata: JSON.stringify({
                  operator_id: 99,
                  reason: 'campaign cleanup',
                }),
              },
            ],
          },
        }
      }
      throw new Error(`Unexpected GET ${url}`)
    }
    const user = userEvent.setup()

    renderSheet(activity())
    await waitFor(() => expect(screen.getByText('alice')).toBeTruthy())
    await user.click(screen.getByRole('button', { name: 'Ledger' }))

    await waitFor(() =>
      expect(screen.getByText(/campaign cleanup/)).toBeTruthy()
    )
    expect(screen.getByText(/operator_id/)).toBeTruthy()
  })
})
