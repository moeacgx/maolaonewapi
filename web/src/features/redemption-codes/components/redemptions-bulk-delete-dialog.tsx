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
import type { Table } from '@tanstack/react-table'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'

import { deleteRedemptions } from '../api'
import { ERROR_MESSAGES } from '../constants'
import { getBatchDeleteSkipReasonMessage } from '../lib'
import type { Redemption } from '../types'
import { useRedemptions } from './redemptions-provider'

type RedemptionsBulkDeleteDialogProps<TData> = {
  open: boolean
  onOpenChange: (open: boolean) => void
  table: Table<TData>
}

export function RedemptionsBulkDeleteDialog<TData>({
  open,
  onOpenChange,
  table,
}: RedemptionsBulkDeleteDialogProps<TData>) {
  const { t } = useTranslation()
  const { triggerRefresh } = useRedemptions()
  const [isDeleting, setIsDeleting] = useState(false)
  const selectedRows = table.getFilteredSelectedRowModel().rows

  const handleConfirm = async () => {
    const ids = selectedRows.map((row) => (row.original as Redemption).id)
    if (ids.length === 0) return

    setIsDeleting(true)
    try {
      const result = await deleteRedemptions(ids)
      if (!result.success) {
        toast.error(result.message || t(ERROR_MESSAGES.BATCH_DELETE_FAILED))
        return
      }

      const deletedIds = result.data?.deleted_ids ?? []
      const skipped = result.data?.skipped ?? []

      if (deletedIds.length > 0) {
        table.setRowSelection((current) => {
          const next = { ...current }
          for (const id of deletedIds) {
            delete next[String(id)]
          }
          return next
        })
      }

      if (skipped.length > 0) {
        const skippedList = skipped
          .map(
            (entry) =>
              `ID ${entry.id}: ${getBatchDeleteSkipReasonMessage(entry.reason, t)}`
          )
          .join('; ')
        toast.warning(
          `${t(
            'Deleted {{deletedCount}} redemption code(s), skipped {{skippedCount}}',
            { deletedCount: deletedIds.length, skippedCount: skipped.length }
          )}. ${t('Skipped')}: ${skippedList}`
        )
      } else {
        toast.success(
          t('Successfully deleted {{count}} redemption code(s)', {
            count: deletedIds.length,
          })
        )
      }

      triggerRefresh()
      onOpenChange(false)
    } catch {
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <ConfirmDialog
      destructive
      open={open}
      onOpenChange={onOpenChange}
      handleConfirm={handleConfirm}
      isLoading={isDeleting}
      className='max-w-md'
      title={t('Delete {{count}} redemption code(s)?', {
        count: selectedRows.length,
      })}
      desc={
        <>
          {t('You are about to delete {{count}} redemption code(s).', {
            count: selectedRows.length,
          })}{' '}
          <br />
          {t(
            'Related top-up logs, delivered amounts, and affiliate sources will not be affected.'
          )}{' '}
          <br />
          {t('This action cannot be undone.')}
        </>
      }
      confirmText={t('Delete')}
    />
  )
}
