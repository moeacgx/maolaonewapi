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
import { flexRender, type ColumnDef } from '@tanstack/react-table'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import type { Redemption } from '../../types'

const i18n = (await import('i18next')).default
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { Toaster, toast } = await import('sonner')
const { api } = await import('@/lib/api')
const { useDataTable } = await import('@/components/data-table')
const { RedemptionsProvider } = await import('../redemptions-provider')
const { DataTableBulkActions } = await import('../data-table-bulk-actions')

await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

type ApiMethod = (url: string, config?: unknown) => Promise<{ data: unknown }>
type MockableApi = {
  delete: ApiMethod
}

const apiClient = api as unknown as MockableApi
const originalDelete = apiClient.delete

function redemption(id: number, name = `code-${id}`): Redemption {
  return {
    id,
    user_id: 1,
    name,
    key: `key-${id}`,
    status: 1,
    quota: 500000,
    created_time: 1,
    redeemed_time: 0,
    expired_time: 0,
    used_user_id: 0,
  }
}

const columns: ColumnDef<Redemption>[] = [
  {
    id: 'select',
    header: () => null,
    cell: ({ row }) => (
      <input
        type='checkbox'
        checked={row.getIsSelected()}
        onChange={(event) => row.toggleSelected(event.target.checked)}
        aria-label={`select-${(row.original as Redemption).id}`}
      />
    ),
  },
  { accessorKey: 'id', header: 'ID' },
  { accessorKey: 'name', header: 'Name' },
]

function TableHarness({ rows }: { rows: Redemption[] }) {
  const { table } = useDataTable({
    data: rows,
    columns,
    enableRowSelection: true,
  })

  return (
    <I18nextProvider i18n={i18n}>
      <RedemptionsProvider>
        <table>
          <tbody>
            {table.getRowModel().rows.map((row) => (
              <tr key={row.id}>
                {row.getVisibleCells().map((cell) => (
                  <td key={cell.id}>
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
        <DataTableBulkActions table={table} />
      </RedemptionsProvider>
      <Toaster duration={60_000} />
    </I18nextProvider>
  )
}

async function selectRow(id: number): Promise<void> {
  fireEvent.click(screen.getByLabelText(`select-${id}`))
}

afterEach(() => {
  apiClient.delete = originalDelete
  toast.dismiss()
})

describe('redemption bulk delete', () => {
  test('deletes selected redemption ids via the registered batch route', async () => {
    const calls: Array<{ url: string; config: unknown }> = []
    apiClient.delete = async (url, config) => {
      calls.push({ url, config })
      return {
        data: {
          success: true,
          data: { deleted_ids: [1, 2], skipped: [] },
        },
      }
    }

    render(<TableHarness rows={[redemption(1), redemption(2)]} />)

    // Need to render a real table row to select via checkbox; render inline table.
    // The bulk actions bar only appears once rows are selected via the table
    // instance itself, so select through the harness checkboxes below.
    await selectRow(1)
    await selectRow(2)

    fireEvent.click(
      screen.getByRole('button', { name: /delete selected codes/i })
    )
    fireEvent.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() => expect(calls).toHaveLength(1))
    expect(calls[0]?.url).toBe('/api/redemption/batch')
    expect(calls[0]?.config).toEqual({ data: { ids: [1, 2] } })
  })

  test('reports deleted and skipped counts instead of the requested id count', async () => {
    apiClient.delete = async () => ({
      data: {
        success: true,
        data: {
          deleted_ids: [1],
          skipped: [{ id: 2, reason: 'used' }],
        },
      },
    })

    render(<TableHarness rows={[redemption(1), redemption(2)]} />)
    await selectRow(1)
    await selectRow(2)

    fireEvent.click(
      screen.getByRole('button', { name: /delete selected codes/i })
    )
    fireEvent.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() =>
      expect(document.body).toHaveTextContent(
        'Deleted 1 redemption code(s), skipped 1'
      )
    )
  })

  test('shows zero deleted when the backend returns an empty deleted_ids array', async () => {
    apiClient.delete = async () => ({
      data: {
        success: true,
        data: { deleted_ids: [], skipped: [{ id: 1, reason: 'used' }] },
      },
    })

    render(<TableHarness rows={[redemption(1)]} />)
    await selectRow(1)

    fireEvent.click(
      screen.getByRole('button', { name: /delete selected codes/i })
    )
    fireEvent.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() =>
      expect(document.body).toHaveTextContent(
        'Deleted 0 redemption code(s), skipped 1'
      )
    )
  })

  test('lists the skipped id with a readable message for the not_found reason', async () => {
    apiClient.delete = async () => ({
      data: {
        success: true,
        data: { deleted_ids: [1], skipped: [{ id: 2, reason: 'not_found' }] },
      },
    })

    render(<TableHarness rows={[redemption(1), redemption(2)]} />)
    await selectRow(1)
    await selectRow(2)

    fireEvent.click(
      screen.getByRole('button', { name: /delete selected codes/i })
    )
    fireEvent.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() =>
      expect(document.body).toHaveTextContent('ID 2: Code not found')
    )
    // The raw skip reason code must never reach the admin verbatim.
    expect(document.body).not.toHaveTextContent('not_found')
  })

  test('falls back to a readable message for a skip reason the frontend does not recognize', async () => {
    apiClient.delete = async () => ({
      data: {
        success: true,
        data: { deleted_ids: [], skipped: [{ id: 9, reason: 'weird_state' }] },
      },
    })

    render(<TableHarness rows={[redemption(9)]} />)
    await selectRow(9)

    fireEvent.click(
      screen.getByRole('button', { name: /delete selected codes/i })
    )
    fireEvent.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() =>
      expect(document.body).toHaveTextContent('ID 9: Unknown reason')
    )
    // The raw backend reason code must never reach the admin verbatim.
    expect(document.body).not.toHaveTextContent('weird_state')
  })

  test('surfaces an error toast and keeps the dialog usable when the request fails', async () => {
    apiClient.delete = async () => ({
      data: { success: false, message: 'boom' },
    })
    const errorSpy = vi.spyOn(toast, 'error')

    render(<TableHarness rows={[redemption(1)]} />)
    await selectRow(1)

    fireEvent.click(
      screen.getByRole('button', { name: /delete selected codes/i })
    )
    fireEvent.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() => expect(errorSpy).toHaveBeenCalledWith('boom'))
  })
})
