import { useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, FileClock, SquareX } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Textarea } from '@/components/ui/textarea'
import { formatQuota, formatTimestampToDate } from '@/lib/format'

import { getAdminBenefitVouchers, voidAdminBenefitVouchers } from '../api'
import { voucherStatusLabel } from '../lib/labels'
import type { BenefitActivity, BenefitVoucherStatus } from '../types'
import { AdminVoucherLedger } from './admin-voucher-ledger'

const VOUCHER_STATUSES: BenefitVoucherStatus[] = [
  'active',
  'exhausted',
  'expired',
  'voided',
]
const PAGE_SIZE = 20

type BenefitVouchersSheetProps = {
  activity: BenefitActivity | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function BenefitVouchersSheet(props: BenefitVouchersSheetProps) {
  const { t } = useTranslation()
  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className='sm:max-w-3xl'>
        <SheetHeader>
          <SheetTitle>
            {props.activity
              ? t('Vouchers for {{name}}', { name: props.activity.name })
              : t('Vouchers')}
          </SheetTitle>
        </SheetHeader>
        {props.activity ? (
          <BenefitVouchersSheetContent
            key={props.activity.id}
            activity={props.activity}
          />
        ) : null}
      </SheetContent>
    </Sheet>
  )
}

function BenefitVouchersSheetContent(props: { activity: BenefitActivity }) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [selectedIds, setSelectedIds] = useState<ReadonlySet<number>>(new Set())
  const [ledgerVoucherId, setLedgerVoucherId] = useState<number | null>(null)
  const [confirmVoid, setConfirmVoid] = useState(false)
  const [voidReason, setVoidReason] = useState('')
  const [voiding, setVoiding] = useState(false)

  const query = useQuery({
    queryKey: [
      'benefit',
      'admin',
      'vouchers',
      props.activity.id,
      page,
      keyword,
      statusFilter,
    ],
    queryFn: () =>
      getAdminBenefitVouchers({
        activityId: props.activity.id,
        page,
        pageSize: PAGE_SIZE,
        filter: { keyword, status: statusFilter },
      }),
  })

  const items = query.data?.items ?? []
  const total = query.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const voidableIds = items
    .filter((voucher) => voucher.status === 'active')
    .map((voucher) => voucher.id)
  const allVoidableSelected =
    voidableIds.length > 0 && voidableIds.every((id) => selectedIds.has(id))

  const toggleSelectAllVoidable = (checked: boolean) => {
    setSelectedIds(checked ? new Set(voidableIds) : new Set())
  }

  const toggleSelected = (id: number, checked: boolean) => {
    setSelectedIds((current) => {
      const next = new Set(current)
      if (checked) next.add(id)
      else next.delete(id)
      return next
    })
  }

  const changePage = (next: number) => {
    setPage(Math.min(totalPages, Math.max(1, next)))
    setSelectedIds(new Set())
  }

  const confirmVoidSelected = async () => {
    if (!voidReason.trim()) {
      toast.error(t('A reason is required to void vouchers'))
      return
    }
    setVoiding(true)
    try {
      const response = await voidAdminBenefitVouchers(
        [...selectedIds],
        voidReason.trim()
      )
      if (!response.success) {
        toast.error(response.message ?? t('Failed to void vouchers'))
        return
      }
      const updated = response.data?.updated_ids.length ?? 0
      const skipped = response.data?.skipped.length ?? 0
      toast.success(
        skipped > 0
          ? t('Voided {{updated}} vouchers; {{skipped}} were skipped', {
              updated,
              skipped,
            })
          : t('Voided {{count}} vouchers', { count: updated })
      )
      setSelectedIds(new Set())
      setVoidReason('')
      setConfirmVoid(false)
      await query.refetch()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to void vouchers')
      )
    } finally {
      setVoiding(false)
    }
  }

  return (
    <div className='flex min-h-0 flex-1 flex-col gap-3 overflow-hidden px-4 pb-4'>
      <div className='flex flex-wrap items-center gap-2'>
        <Input
          value={keyword}
          onChange={(event) => {
            setKeyword(event.target.value)
            setPage(1)
            setSelectedIds(new Set())
          }}
          placeholder={t('Search by username')}
          className='w-48'
          aria-label={t('Search by username')}
        />
        <Select
          items={[
            { value: '', label: t('All statuses') },
            ...VOUCHER_STATUSES.map((status) => ({
              value: status,
              label: voucherStatusLabel(status, t),
            })),
          ]}
          value={statusFilter}
          onValueChange={(value) => {
            setStatusFilter(value ?? '')
            setPage(1)
            setSelectedIds(new Set())
          }}
        >
          <SelectTrigger className='w-36'>
            <SelectValue placeholder={t('All statuses')} />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              <SelectItem value=''>{t('All statuses')}</SelectItem>
              {VOUCHER_STATUSES.map((status) => (
                <SelectItem key={status} value={status}>
                  {voucherStatusLabel(status, t)}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        {selectedIds.size > 0 ? (
          <Button
            type='button'
            size='sm'
            variant='destructive'
            onClick={() => setConfirmVoid(true)}
          >
            <SquareX />
            {t('Void selected ({{count}})', { count: selectedIds.size })}
          </Button>
        ) : null}
      </div>

      <div className='border-border min-h-0 flex-1 overflow-y-auto rounded-md border'>
        <StaticDataTable
          data={items}
          getRowKey={(voucher) => voucher.id}
          emptyClassName={
            query.isLoading ? 'py-8' : 'text-muted-foreground py-8'
          }
          emptyContent={
            query.isLoading ? t('Loading...') : t('No vouchers found')
          }
          columns={[
            {
              id: 'select',
              header: (
                <Checkbox
                  checked={allVoidableSelected}
                  onCheckedChange={(checked) =>
                    toggleSelectAllVoidable(checked === true)
                  }
                  disabled={voidableIds.length === 0}
                  aria-label={t('Select all active vouchers on this page')}
                />
              ),
              cell: (voucher) =>
                voucher.status === 'active' ? (
                  <Checkbox
                    checked={selectedIds.has(voucher.id)}
                    onCheckedChange={(checked) =>
                      toggleSelected(voucher.id, checked === true)
                    }
                    aria-label={t('Select voucher #{{id}}', { id: voucher.id })}
                  />
                ) : null,
            },
            {
              id: 'id',
              header: t('ID'),
              cell: (voucher) => <TableId value={voucher.id} />,
            },
            {
              id: 'user',
              header: t('User'),
              cell: (voucher) => voucher.username || `#${voucher.user_id}`,
            },
            {
              id: 'quota',
              header: t('Original / Used / Remaining'),
              cell: (voucher) =>
                `${formatQuota(voucher.original_quota)} / ${formatQuota(voucher.used_quota)} / ${formatQuota(voucher.remaining_quota)}`,
            },
            {
              id: 'status',
              header: t('Status'),
              cell: (voucher) => (
                <StatusBadge
                  label={voucherStatusLabel(voucher.status, t)}
                  variant={voucher.status === 'active' ? 'success' : 'neutral'}
                  copyable={false}
                />
              ),
            },
            {
              id: 'claimed',
              header: t('Claimed / Expires'),
              cell: (voucher) => (
                <div className='text-xs'>
                  <div>{formatTimestampToDate(voucher.claimed_at)}</div>
                  <div className='text-muted-foreground'>
                    {formatTimestampToDate(voucher.expires_at)}
                  </div>
                </div>
              ),
            },
            {
              id: 'actions',
              header: t('Actions'),
              className: 'text-right',
              cellClassName: 'text-right',
              cell: (voucher) => (
                <Button
                  type='button'
                  size='sm'
                  variant='ghost'
                  onClick={() => setLedgerVoucherId(voucher.id)}
                >
                  <FileClock />
                  {t('Ledger')}
                </Button>
              ),
            },
          ]}
        />
      </div>

      <div className='flex items-center justify-between gap-2 text-sm'>
        <span className='text-muted-foreground'>
          {t('{{total}} vouchers', { total })}
        </span>
        <div className='flex items-center gap-2'>
          <Button
            type='button'
            size='sm'
            variant='outline'
            onClick={() => changePage(page - 1)}
            disabled={page <= 1}
          >
            <ChevronLeft className='size-4' />
          </Button>
          <span className='text-muted-foreground'>
            {page} / {totalPages}
          </span>
          <Button
            type='button'
            size='sm'
            variant='outline'
            onClick={() => changePage(page + 1)}
            disabled={page >= totalPages}
          >
            <ChevronRight className='size-4' />
          </Button>
        </div>
      </div>

      <AdminVoucherLedger
        voucherId={ledgerVoucherId}
        open={ledgerVoucherId !== null}
        onOpenChange={(open) => {
          if (!open) setLedgerVoucherId(null)
        }}
      />

      <ConfirmDialog
        destructive
        open={confirmVoid}
        onOpenChange={(open) => {
          setConfirmVoid(open)
          if (!open) setVoidReason('')
        }}
        handleConfirm={() => void confirmVoidSelected()}
        isLoading={voiding}
        disabled={!voidReason.trim()}
        className='max-w-md'
        title={t('Void {{count}} vouchers?', { count: selectedIds.size })}
        desc={t(
          'Voided vouchers can no longer be used. This action cannot be undone.'
        )}
        confirmText={t('Void')}
      >
        <Textarea
          aria-label={t('Reason')}
          placeholder={t('Reason')}
          value={voidReason}
          onChange={(event) => setVoidReason(event.target.value)}
        />
      </ConfirmDialog>
    </div>
  )
}
