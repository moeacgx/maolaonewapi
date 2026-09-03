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
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Empty, EmptyDescription, EmptyTitle } from '@/components/ui/empty'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import {
  formatLogQuota,
  formatQuota,
  formatTimestampToDate,
} from '@/lib/format'

import { getAdminBenefitVoucherLedger } from '../api'
import { ledgerEntryTypeLabel } from '../lib/labels'

type AdminVoucherLedgerProps = {
  voucherId: number | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

function readableMetadata(metadata: string | undefined): string | null {
  if (!metadata || !metadata.trim()) return null
  try {
    const parsed = JSON.parse(metadata) as Record<string, unknown>
    return Object.entries(parsed)
      .map(([key, value]) => `${key}: ${String(value)}`)
      .join(' · ')
  } catch {
    return metadata
  }
}

export function AdminVoucherLedger(props: AdminVoucherLedgerProps) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['benefit', 'admin', 'ledger', props.voucherId],
    queryFn: () => getAdminBenefitVoucherLedger(props.voucherId as number),
    enabled: props.open && props.voucherId != null,
  })
  const entries = [...(query.data ?? [])].sort(
    (a, b) => b.created_at - a.created_at
  )

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className='sm:max-w-lg'>
        <SheetHeader>
          <SheetTitle>{t('Voucher ledger')}</SheetTitle>
        </SheetHeader>
        <div className='grid gap-3 overflow-y-auto px-4 pb-4'>
          {query.isLoading ? (
            <div className='grid gap-2'>
              <Skeleton className='h-16 w-full' />
              <Skeleton className='h-16 w-full' />
            </div>
          ) : null}
          {query.isError ? (
            <Empty>
              <EmptyTitle>{t('Unable to load voucher ledger')}</EmptyTitle>
              <EmptyDescription>
                {query.error instanceof Error
                  ? query.error.message
                  : t('Please try again')}
              </EmptyDescription>
              <Button
                type='button'
                size='sm'
                variant='outline'
                onClick={() => void query.refetch()}
              >
                {t('Retry')}
              </Button>
            </Empty>
          ) : null}
          {!query.isLoading && !query.isError && entries.length === 0 ? (
            <Empty>
              <EmptyTitle>{t('No ledger entries yet')}</EmptyTitle>
            </Empty>
          ) : null}
          {entries.map((entry) => {
            const metadata = readableMetadata(entry.metadata)
            return (
              <div
                key={entry.id}
                className='border-border grid gap-1 rounded-md border p-3 text-sm'
              >
                <div className='flex items-center justify-between gap-2'>
                  <span className='font-medium'>
                    {ledgerEntryTypeLabel(entry.type, t)}
                  </span>
                  <span
                    className={
                      entry.quota_delta < 0
                        ? 'text-destructive tabular-nums'
                        : 'text-success tabular-nums'
                    }
                  >
                    {entry.quota_delta >= 0 ? '+' : ''}
                    {formatLogQuota(entry.quota_delta)}
                  </span>
                </div>
                <div className='text-muted-foreground flex items-center justify-between gap-2 text-xs'>
                  <span>
                    {t('Balance after')}: {formatQuota(entry.balance_after)}
                  </span>
                  <span>{formatTimestampToDate(entry.created_at)}</span>
                </div>
                {entry.request_id || entry.log_id ? (
                  <div className='text-muted-foreground text-xs'>
                    {entry.request_id ? (
                      <span>
                        {t('Request')}: {entry.request_id}
                      </span>
                    ) : null}
                    {entry.log_id ? (
                      <span className='ml-2'>
                        {t('Log')}: #{entry.log_id}
                      </span>
                    ) : null}
                  </div>
                ) : null}
                {metadata ? (
                  <div className='text-muted-foreground border-border border-t pt-1 text-xs'>
                    {t('Admin details')}: {metadata}
                  </div>
                ) : null}
              </div>
            )
          })}
        </div>
      </SheetContent>
    </Sheet>
  )
}
