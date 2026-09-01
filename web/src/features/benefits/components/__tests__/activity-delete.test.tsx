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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it } from 'vitest'

import { api } from '@/lib/api'

import type { BenefitActivity } from '../../types'
import { BenefitActivitiesPanel } from '../benefit-activities-panel'

const { Toaster, toast } = await import('sonner')

type ApiMethod = (url: string, data?: unknown) => Promise<{ data: unknown }>
type MockableApi = { get: ApiMethod; delete: ApiMethod }

const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get
const originalDelete = apiClient.delete

function activity(overrides: Partial<BenefitActivity> = {}): BenefitActivity {
  return {
    id: 1,
    name: 'Weekend Boost',
    description: '',
    group_id: 7,
    group_code_snapshot: 'default',
    group_name_snapshot: 'Default',
    status: 'draft',
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
    published_at: 0,
    ...overrides,
  }
}

function installApiFixtures(activities: BenefitActivity[]) {
  apiClient.get = async (url) => {
    if (url === '/api/benefit/admin/activities?p=1&page_size=100') {
      return {
        data: {
          success: true,
          data: { items: activities, total: activities.length },
        },
      }
    }
    if (url === '/api/group/details') {
      return { data: { success: true, data: [] } }
    }
    throw new Error(`Unexpected GET ${url}`)
  }
}

function renderPanel() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <BenefitActivitiesPanel />
      <Toaster duration={60_000} />
    </QueryClientProvider>
  )
}

function checkboxForRow(name: string): HTMLElement {
  const row = screen.getByText(name).closest('tr')
  if (!row) throw new Error(`Expected a table row for "${name}"`)
  return within(row).getByRole('checkbox')
}

afterEach(() => {
  apiClient.get = originalGet
  apiClient.delete = originalDelete
  toast.dismiss()
})

describe('benefit activities panel deletion', () => {
  it('keeps running activities unselectable and allows selecting historical ones', async () => {
    installApiFixtures([
      activity({ id: 1, name: 'Published activity', status: 'published' }),
      activity({ id: 2, name: 'Ended activity', status: 'ended' }),
    ])
    renderPanel()

    await waitFor(() =>
      expect(screen.getByText('Published activity')).toBeTruthy()
    )

    expect(checkboxForRow('Published activity')).toHaveAttribute(
      'aria-disabled',
      'true'
    )
    expect(checkboxForRow('Ended activity')).not.toHaveAttribute(
      'aria-disabled',
      'true'
    )
  })

  it('selects only deletable activities through the header checkbox', async () => {
    installApiFixtures([
      activity({ id: 1, name: 'Published activity', status: 'published' }),
      activity({ id: 2, name: 'Draft activity', status: 'draft' }),
      activity({ id: 3, name: 'Ended activity', status: 'ended' }),
    ])
    const user = userEvent.setup()
    renderPanel()

    await waitFor(() => expect(screen.getByText('Draft activity')).toBeTruthy())

    const headerCheckbox = screen.getByRole('checkbox', {
      name: 'Select all deletable activities',
    })
    await user.click(headerCheckbox)

    expect(checkboxForRow('Draft activity')).toBeChecked()
    expect(checkboxForRow('Ended activity')).toBeChecked()
    expect(screen.getByText('2 activities selected')).toBeTruthy()
  })

  it('deletes selected activities and reports the actual deleted ids and skip reasons', async () => {
    installApiFixtures([
      activity({ id: 2, name: 'Draft activity', status: 'draft' }),
      activity({ id: 3, name: 'Ended activity', status: 'ended' }),
    ])
    const deleteCalls: unknown[] = []
    apiClient.delete = async (url, config) => {
      deleteCalls.push({ url, body: (config as { data?: unknown })?.data })
      return {
        data: {
          success: true,
          data: {
            deleted_ids: [2],
            skipped: [{ id: 3, reason: 'active_voucher' }],
          },
        },
      }
    }
    const user = userEvent.setup()
    renderPanel()

    await waitFor(() => expect(screen.getByText('Draft activity')).toBeTruthy())
    await user.click(checkboxForRow('Draft activity'))
    await user.click(checkboxForRow('Ended activity'))
    await user.click(screen.getByRole('button', { name: /Delete selected/ }))
    await user.click(screen.getByRole('button', { name: 'Delete' }))

    await waitFor(() => expect(deleteCalls).toHaveLength(1))
    expect(deleteCalls[0]).toEqual({
      url: '/api/benefit/admin/activities/batch',
      body: { ids: [2, 3] },
    })
    await waitFor(() =>
      expect(screen.queryByText('2 activities selected')).toBeNull()
    )
    expect(
      screen.getByText(
        /Deleted 1 activities; 1 were skipped: Activity still has active vouchers/
      )
    ).toBeTruthy()
  })

  it('maps the not_deletable skip reason to readable text', async () => {
    installApiFixtures([
      activity({ id: 2, name: 'Ended activity', status: 'ended' }),
    ])
    apiClient.delete = async () => ({
      data: {
        success: true,
        data: {
          deleted_ids: [],
          skipped: [{ id: 2, reason: 'not_deletable' }],
        },
      },
    })
    const user = userEvent.setup()
    renderPanel()

    await waitFor(() => expect(screen.getByText('Ended activity')).toBeTruthy())
    await user.click(checkboxForRow('Ended activity'))
    await user.click(screen.getByRole('button', { name: /Delete selected/ }))
    await user.click(screen.getByRole('button', { name: 'Delete' }))

    await waitFor(() =>
      expect(
        screen.getByText(
          /Deleted 0 activities; 1 were skipped: Activity is still active or not eligible for deletion/
        )
      ).toBeTruthy()
    )
  })

  it('shows a fixed unknown-reason label and never leaks the raw backend code', async () => {
    installApiFixtures([
      activity({ id: 2, name: 'Ended activity', status: 'ended' }),
    ])
    apiClient.delete = async () => ({
      data: {
        success: true,
        data: {
          deleted_ids: [],
          skipped: [{ id: 2, reason: 'unknown_internal_code' }],
        },
      },
    })
    const user = userEvent.setup()
    renderPanel()

    await waitFor(() => expect(screen.getByText('Ended activity')).toBeTruthy())
    await user.click(checkboxForRow('Ended activity'))
    await user.click(screen.getByRole('button', { name: /Delete selected/ }))
    await user.click(screen.getByRole('button', { name: 'Delete' }))

    await waitFor(() =>
      expect(
        screen.getByText(/Deleted 0 activities; 1 were skipped: Unknown reason/)
      ).toBeTruthy()
    )
    expect(screen.queryByText(/unknown_internal_code/)).toBeNull()
  })

  it('shows zero deleted when the batch response reports no successes', async () => {
    installApiFixtures([
      activity({ id: 2, name: 'Draft activity', status: 'draft' }),
    ])
    apiClient.delete = async () => ({
      data: {
        success: true,
        data: {
          deleted_ids: [],
          skipped: [{ id: 2, reason: 'has_claim_data' }],
        },
      },
    })
    const user = userEvent.setup()
    renderPanel()

    await waitFor(() => expect(screen.getByText('Draft activity')).toBeTruthy())
    await user.click(checkboxForRow('Draft activity'))
    await user.click(screen.getByRole('button', { name: /Delete selected/ }))
    await user.click(screen.getByRole('button', { name: 'Delete' }))

    await waitFor(() =>
      expect(
        screen.getByText(
          /Deleted 0 activities; 1 were skipped: Draft activity already has claim data/
        )
      ).toBeTruthy()
    )
  })

  it('shows an error toast and resets the deleting state when the delete request throws', async () => {
    installApiFixtures([
      activity({ id: 2, name: 'Draft activity', status: 'draft' }),
    ])
    apiClient.delete = async () => {
      throw new Error('network down')
    }
    const user = userEvent.setup()
    renderPanel()

    await waitFor(() => expect(screen.getByText('Draft activity')).toBeTruthy())
    await user.click(checkboxForRow('Draft activity'))
    await user.click(screen.getByRole('button', { name: /Delete selected/ }))
    const confirmButton = screen.getByRole('button', { name: 'Delete' })
    await user.click(confirmButton)

    await waitFor(() => expect(screen.getByText('network down')).toBeTruthy())
    await waitFor(() => expect(confirmButton).toBeEnabled())
  })
})
