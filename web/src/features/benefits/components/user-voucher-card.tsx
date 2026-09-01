import { FileClock } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Progress,
  ProgressTrack,
  ProgressIndicator,
} from '@/components/ui/progress'
import { formatQuota, formatTimestampToDate } from '@/lib/format'

import { voucherStatusLabel } from '../lib/labels'
import type { BenefitActivityUserView, BenefitVoucher } from '../types'

const STATUS_VARIANT: Record<BenefitVoucher['status'], StatusVariant> = {
  active: 'success',
  exhausted: 'neutral',
  expired: 'neutral',
  voided: 'danger',
}

type UserVoucherCardProps = {
  voucher: BenefitVoucher
  activity?: BenefitActivityUserView
  onViewLedger: (voucherId: number) => void
}

export function UserVoucherCard(props: UserVoucherCardProps) {
  const { t } = useTranslation()
  const voucher = props.voucher
  const usedRatio =
    voucher.original_quota > 0
      ? Math.min(100, (voucher.used_quota / voucher.original_quota) * 100)
      : 0

  return (
    <Card size='sm'>
      <CardHeader>
        <CardTitle className='flex items-center justify-between gap-2'>
          <span className='truncate'>
            {voucher.activity_name ||
              props.activity?.name ||
              t('Benefit voucher')}
          </span>
          <StatusBadge
            label={voucherStatusLabel(voucher.status, t)}
            variant={STATUS_VARIANT[voucher.status]}
            copyable={false}
          />
        </CardTitle>
      </CardHeader>
      <CardContent className='grid gap-3'>
        <div>
          <span className='text-2xl font-semibold tabular-nums'>
            {formatQuota(voucher.remaining_quota)}
          </span>
          <p className='text-muted-foreground text-xs'>
            {t('Remaining balance')}
          </p>
        </div>
        <Progress value={usedRatio}>
          <div className='text-muted-foreground flex justify-between text-xs'>
            <span>
              {t('Used')} {formatQuota(voucher.used_quota)}
            </span>
            <span>
              {t('Original')} {formatQuota(voucher.original_quota)}
            </span>
          </div>
          <ProgressTrack>
            <ProgressIndicator />
          </ProgressTrack>
        </Progress>
        <div className='text-muted-foreground grid gap-1 text-xs'>
          <span>
            {t('Bound group')}:{' '}
            {voucher.group_name_snapshot ||
              props.activity?.group_name_snapshot ||
              t('Unknown')}
          </span>
          <span>
            {t('Claimed at')}: {formatTimestampToDate(voucher.claimed_at)}
          </span>
          <span>
            {t('Expires')}: {formatTimestampToDate(voucher.expires_at)}
          </span>
          {voucher.status === 'voided' && voucher.void_reason ? (
            <span>
              {t('Void reason')}: {voucher.void_reason}
            </span>
          ) : null}
        </div>
        <Button
          type='button'
          size='sm'
          variant='outline'
          onClick={() => props.onViewLedger(voucher.id)}
        >
          <FileClock />
          {t('View ledger')}
        </Button>
      </CardContent>
    </Card>
  )
}
