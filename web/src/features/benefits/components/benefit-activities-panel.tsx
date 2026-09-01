import { useQuery } from '@tanstack/react-query'
import {
  BarChart3,
  Eye,
  FilePenLine,
  Plus,
  RefreshCw,
  SquareX,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

import {
  createAdminBenefitActivity,
  getAdminBenefitReport,
  getAdminBenefitActivities,
  getAdminBenefitVoucherLedger,
  getAdminBenefitVouchers,
  publishAdminBenefitActivity,
  terminateAdminBenefitActivity,
  transitionAdminBenefitActivity,
  updateAdminBenefitActivity,
  voidAdminBenefitVoucher,
} from '../api'
import type {
  BenefitActivity,
  BenefitLedgerEntry,
  BenefitReport,
  BenefitVoucher,
} from '../types'
import { BenefitActivityForm } from './benefit-activity-form'
import { BenefitTerminateDialog } from './benefit-terminate-dialog'

function displayActivityAmount(activity: BenefitActivity) {
  if (typeof activity.total_amount === 'number') {
    return activity.total_amount.toFixed(2)
  }
  return ((activity.total_amount_cents ?? 0) / 100).toFixed(2)
}

export function BenefitActivitiesPanel() {
  const { t } = useTranslation()
  const [showForm, setShowForm] = useState(false)
  const [terminateID, setTerminateID] = useState<number | null>(null)
  const [editActivity, setEditActivity] = useState<BenefitActivity | null>(null)
  const [detail, setDetail] = useState<{
    activityID: number
    kind: 'report' | 'vouchers'
  } | null>(null)
  const [ledgerVoucherID, setLedgerVoucherID] = useState<number | null>(null)
  const query = useQuery({
    queryKey: ['benefit', 'admin', 'activities'],
    queryFn: getAdminBenefitActivities,
  })

  const reportQuery = useQuery<BenefitReport>({
    queryKey: ['benefit', 'admin', 'report', detail?.activityID],
    queryFn: () => {
      if (!detail?.activityID) {
        throw new Error(t('Benefit activity operation failed'))
      }
      return getAdminBenefitReport(detail.activityID)
    },
    enabled: detail?.kind === 'report',
  })
  const vouchersQuery = useQuery<BenefitVoucher[]>({
    queryKey: ['benefit', 'admin', 'vouchers', detail?.activityID],
    queryFn: () => {
      if (!detail?.activityID) {
        throw new Error(t('Benefit activity operation failed'))
      }
      return getAdminBenefitVouchers(detail.activityID)
    },
    enabled: detail?.kind === 'vouchers',
  })
  const ledgerQuery = useQuery<BenefitLedgerEntry[]>({
    queryKey: ['benefit', 'admin', 'ledger', ledgerVoucherID],
    queryFn: () => {
      if (!ledgerVoucherID) {
        throw new Error(t('Benefit activity operation failed'))
      }
      return getAdminBenefitVoucherLedger(ledgerVoucherID)
    },
    enabled: ledgerVoucherID !== null,
  })

  const refresh = async () => {
    await query.refetch()
  }
  const create = async (
    input: Parameters<typeof createAdminBenefitActivity>[0]
  ) => {
    const response = await createAdminBenefitActivity(input)
    if (!response.success) {
      toast.error(response.message ?? t('Failed to save benefit activity'))
      return
    }
    toast.success(t('Benefit activity draft created'))
    setShowForm(false)
    await refresh()
  }

  const save = async (
    input: Parameters<typeof updateAdminBenefitActivity>[0]
  ) => {
    const response = await updateAdminBenefitActivity(input)
    if (!response.success) {
      toast.error(response.message ?? t('Failed to save benefit activity'))
      return
    }
    toast.success(t('Benefit activity saved'))
    setEditActivity(null)
    await refresh()
  }

  const runAction = async (
    action: () => Promise<{ success: boolean; message?: string }>
  ) => {
    try {
      const response = await action()
      if (!response.success) {
        toast.error(response.message ?? t('Benefit activity operation failed'))
        return
      }
      await refresh()
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Benefit activity operation failed')
      )
    }
  }

  const voidVoucher = async (voucher: BenefitVoucher) => {
    if (!window.confirm(t('Void this voucher?'))) return
    const reason = window.prompt(t('Reason'), '')?.trim()
    if (!reason) return
    const response = await voidAdminBenefitVoucher(voucher.id, reason)
    if (!response.success) {
      toast.error(response.message ?? t('Failed to void voucher'))
      return
    }
    toast.success(t('Voucher voided'))
    await vouchersQuery.refetch()
  }

  return (
    <div className='grid gap-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <p className='text-muted-foreground text-sm'>
          {t('Time-limited vouchers stay separate from wallet balance.')}
        </p>
        <div className='flex gap-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => void refresh()}
          >
            <RefreshCw />
            {t('Refresh')}
          </Button>
          <Button
            type='button'
            size='sm'
            onClick={() => setShowForm((value) => !value)}
          >
            <Plus />
            {t('Create activity')}
          </Button>
        </div>
      </div>
      {showForm ? (
        <BenefitActivityForm
          onSubmit={create}
          onCancel={() => setShowForm(false)}
        />
      ) : null}
      {editActivity ? (
        <BenefitActivityForm
          key={editActivity.id}
          initial={editActivity}
          onSubmit={(input) => save({ ...input, id: editActivity.id })}
          onCancel={() => setEditActivity(null)}
        />
      ) : null}
      {query.isError ? (
        <p className='text-destructive text-sm'>
          {query.error instanceof Error
            ? query.error.message
            : t('Benefit activity operation failed')}
        </p>
      ) : null}
      <div className='grid gap-3'>
        {(query.data ?? []).map((activity) => (
          <Card key={activity.id} size='sm'>
            <CardHeader>
              <CardTitle>{activity.name}</CardTitle>
              <span className='text-muted-foreground text-xs'>
                {activity.group_name_snapshot} ·{' '}
                ¥{displayActivityAmount(activity)}
              </span>
            </CardHeader>
            <CardContent className='flex flex-wrap items-center gap-2'>
              <StatusBadge
                label={activity.status}
                variant={
                  activity.status === 'published' ? 'success' : 'neutral'
                }
                size='sm'
                copyable={false}
              />
              <span className='text-muted-foreground text-xs'>
                {t('Total count')}: {activity.total_count}
              </span>
              {activity.status === 'draft' ? (
                <Button
                  type='button'
                  size='sm'
                  onClick={() =>
                    void runAction(() =>
                      publishAdminBenefitActivity(activity.id)
                    )
                  }
                >
                  {t('Publish')}
                </Button>
              ) : null}
              {activity.status === 'published' ? (
                <Button
                  type='button'
                  size='sm'
                  variant='outline'
                  onClick={() =>
                    void runAction(() =>
                      transitionAdminBenefitActivity(activity.id, 'pause')
                    )
                  }
                >
                  {t('Pause')}
                </Button>
              ) : null}
              {activity.status === 'paused' ? (
                <Button
                  type='button'
                  size='sm'
                  variant='outline'
                  onClick={() =>
                    void runAction(() =>
                      transitionAdminBenefitActivity(activity.id, 'resume')
                    )
                  }
                >
                  {t('Resume')}
                </Button>
              ) : null}
              {activity.status === 'published' ||
              activity.status === 'paused' ? (
                <Button
                  type='button'
                  size='sm'
                  variant='outline'
                  onClick={() =>
                    void runAction(() =>
                      transitionAdminBenefitActivity(activity.id, 'end')
                    )
                  }
                >
                  {t('End now')}
                </Button>
              ) : null}
              <Button
                type='button'
                size='sm'
                variant='ghost'
                onClick={() => setEditActivity(activity)}
              >
                <FilePenLine />
                {t('Edit')}
              </Button>
              <Button
                type='button'
                size='sm'
                variant='ghost'
                onClick={() =>
                  setDetail({ activityID: activity.id, kind: 'report' })
                }
              >
                <BarChart3 />
                {t('Report')}
              </Button>
              <Button
                type='button'
                size='sm'
                variant='ghost'
                onClick={() =>
                  setDetail({ activityID: activity.id, kind: 'vouchers' })
                }
              >
                <Eye />
                {t('Vouchers')}
              </Button>
              {activity.status === 'published' ||
              activity.status === 'paused' ? (
                <Button
                  type='button'
                  size='sm'
                  variant='destructive'
                  onClick={() => setTerminateID(activity.id)}
                >
                  {t('Terminate')}
                </Button>
              ) : null}
            </CardContent>
            {detail?.activityID === activity.id ? (
              <div className='border-border mx-4 mb-4 grid gap-3 border-t pt-3 text-sm'>
                {detail.kind === 'report' ? (
                  <div className='grid gap-1 sm:grid-cols-2'>
                    {Object.entries(reportQuery.data ?? {}).map(
                      ([key, value]) => (
                        <div key={key} className='flex justify-between gap-3'>
                          <span className='text-muted-foreground'>
                            {t(key)}
                          </span>
                          <span className='tabular-nums'>{String(value)}</span>
                        </div>
                      )
                    )}
                  </div>
                ) : (
                  <div className='grid gap-2'>
                    {(vouchersQuery.data ?? []).map((voucher) => (
                      <div
                        key={voucher.id}
                        className='flex flex-wrap items-center justify-between gap-2 border-b pb-2'
                      >
                        <span>
                          #{voucher.id} · {t('Remaining')}:{' '}
                          {voucher.remaining_quota}
                        </span>
                        <span className='flex gap-2'>
                          <Button
                            type='button'
                            size='sm'
                            variant='outline'
                            onClick={() => setLedgerVoucherID(voucher.id)}
                          >
                            {t('Ledger')}
                          </Button>
                          <Button
                            type='button'
                            size='sm'
                            variant='destructive'
                            disabled={voucher.status === 'voided'}
                            onClick={() => void voidVoucher(voucher)}
                          >
                            <SquareX />
                            {t('Void')}
                          </Button>
                        </span>
                      </div>
                    ))}
                  </div>
                )}
                {ledgerVoucherID ? (
                  <div className='bg-muted/30 grid gap-1 p-2'>
                    {(ledgerQuery.data ?? []).map((entry) => (
                      <div
                        key={entry.id}
                        className='flex justify-between gap-2 text-xs'
                      >
                        <span>{entry.type}</span>
                        <span>
                          {entry.quota_delta} · {entry.balance_after}
                        </span>
                      </div>
                    ))}
                  </div>
                ) : null}
              </div>
            ) : null}
          </Card>
        ))}
        {!query.isLoading && (query.data ?? []).length === 0 ? (
          <p className='text-muted-foreground py-8 text-center'>
            {t('No benefit activities')}
          </p>
        ) : null}
      </div>
      {terminateID ? (
        <BenefitTerminateDialog
          onCancel={() => setTerminateID(null)}
          onConfirm={async (mode, reason) => {
            const response = await terminateAdminBenefitActivity(
              terminateID,
              mode,
              reason
            )
            if (!response.success) {
              toast.error(response.message ?? t('Failed to terminate activity'))
              return
            }
            setTerminateID(null)
            await refresh()
          }}
        />
      ) : null}
    </div>
  )
}
