import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import type { BenefitLedgerEntry } from '../../types'
import { UserVoucherLedgerSheet } from '../user-voucher-ledger-sheet'

type ApiMethod = (url: string) => Promise<{ data: unknown }>
type MockableApi = { get: ApiMethod }

const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get

function entry(
  overrides: Partial<BenefitLedgerEntry> = {}
): BenefitLedgerEntry {
  return {
    id: 1,
    activity_id: 1,
    voucher_id: 5,
    user_id: 1,
    request_id: 'req-1',
    log_id: 100,
    type: 'pre_consume',
    quota_delta: -1000,
    balance_after: 499000,
    created_at: 1000,
    ...overrides,
  }
}

function renderSheet(props: {
  voucherId: number | null
  open: boolean
  onOpenChange?: (open: boolean) => void
}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <UserVoucherLedgerSheet
        voucherId={props.voucherId}
        open={props.open}
        onOpenChange={props.onOpenChange ?? (() => undefined)}
      />
    </QueryClientProvider>
  )
}

afterEach(() => {
  apiClient.get = originalGet
})

describe('user voucher ledger sheet', () => {
  it('fetches and renders ledger entries from the user-owned ledger route', async () => {
    const requested: string[] = []
    apiClient.get = async (url) => {
      requested.push(url)
      return {
        data: {
          success: true,
          data: [
            entry({ id: 1, type: 'pre_consume', created_at: 1000 }),
            entry({
              id: 2,
              type: 'refund',
              quota_delta: 200,
              balance_after: 499200,
              created_at: 2000,
            }),
          ],
        },
      }
    }

    renderSheet({ voucherId: 5, open: true })

    await waitFor(() =>
      expect(requested).toContain('/api/benefit/vouchers/5/ledger')
    )
    await waitFor(() => expect(screen.getByText('Refund')).toBeTruthy())
    expect(screen.getByText('Pre-consume')).toBeTruthy()
    expect(screen.queryByText('-1000')).toBeNull()
    expect(screen.queryByText('499000')).toBeNull()
  })

  it('orders entries by most recent first', async () => {
    apiClient.get = async () => ({
      data: {
        success: true,
        data: [
          entry({ id: 1, type: 'pre_consume', created_at: 1000 }),
          entry({ id: 2, type: 'refund', created_at: 3000 }),
          entry({ id: 3, type: 'void', created_at: 2000 }),
        ],
      },
    })

    renderSheet({ voucherId: 5, open: true })

    await waitFor(() => expect(screen.getByText('Refund')).toBeTruthy())
    const labels = screen
      .getAllByText(/Refund|Voided|Pre-consume/)
      .map((el) => el.textContent)
    expect(labels).toEqual(['Refund', 'Voided', 'Pre-consume'])
  })

  it('shows a retry action when the ledger request fails', async () => {
    let attempts = 0
    apiClient.get = async () => {
      attempts += 1
      if (attempts === 1) throw new Error('network down')
      return { data: { success: true, data: [entry()] } }
    }

    const user = userEvent.setup()
    renderSheet({ voucherId: 5, open: true })

    await waitFor(() =>
      expect(screen.getByText('Unable to load voucher ledger')).toBeTruthy()
    )
    await user.click(screen.getByRole('button', { name: 'Retry' }))

    await waitFor(() => expect(screen.getByText('Pre-consume')).toBeTruthy())
    expect(attempts).toBe(2)
  })

  it('shows an empty state when the voucher has no ledger entries', async () => {
    apiClient.get = async () => ({ data: { success: true, data: [] } })

    renderSheet({ voucherId: 5, open: true })

    await waitFor(() =>
      expect(screen.getByText('No ledger entries yet')).toBeTruthy()
    )
  })

  it('does not fetch the ledger while the sheet is closed', async () => {
    const get = vi.fn().mockResolvedValue({ data: { success: true, data: [] } })
    apiClient.get = get

    renderSheet({ voucherId: 5, open: false })

    expect(get).not.toHaveBeenCalled()
  })

  it('never renders admin-only ledger metadata', async () => {
    apiClient.get = async () => ({
      data: {
        success: true,
        data: [
          entry({
            id: 1,
            metadata: JSON.stringify({ operator_id: 99, reason: 'fraud' }),
          }),
        ],
      },
    })

    renderSheet({ voucherId: 5, open: true })

    await waitFor(() => expect(screen.getByText('Pre-consume')).toBeTruthy())
    expect(screen.queryByText(/operator_id/)).toBeNull()
    expect(screen.queryByText(/fraud/)).toBeNull()
  })
})
