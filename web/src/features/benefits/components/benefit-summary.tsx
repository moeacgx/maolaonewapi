import { Clock, Gift, TicketCheck, Wallet } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatQuota } from '@/lib/format'

import type { BenefitActivityUserView, BenefitVoucher } from '../types'

const EXPIRING_SOON_WINDOW_SECONDS = 3 * 24 * 60 * 60

type BenefitSummaryProps = {
  vouchers: BenefitVoucher[]
  activities: BenefitActivityUserView[]
}

type SummaryTile = {
  key: string
  label: string
  value: string
  icon: typeof Wallet
}

export function BenefitSummary(props: BenefitSummaryProps) {
  const { t } = useTranslation()

  const stats = useMemo(() => {
    // eslint-disable-next-line react-hooks/purity
    const now = Math.floor(Date.now() / 1000)
    const availableQuota = props.vouchers
      .filter((voucher) => voucher.status === 'active')
      .reduce((sum, voucher) => sum + voucher.remaining_quota, 0)
    const usedQuota = props.vouchers.reduce(
      (sum, voucher) => sum + voucher.used_quota,
      0
    )
    const expiringCount = props.vouchers.filter(
      (voucher) =>
        voucher.status === 'active' &&
        voucher.remaining_quota > 0 &&
        voucher.expires_at > now &&
        voucher.expires_at - now <= EXPIRING_SOON_WINDOW_SECONDS
    ).length
    const claimableCount = props.activities.filter(
      (activity) => activity.eligible && !activity.has_claimed
    ).length
    return { availableQuota, usedQuota, expiringCount, claimableCount }
  }, [props.vouchers, props.activities])

  const tiles: SummaryTile[] = [
    {
      key: 'available',
      label: t('Available benefit'),
      value: formatQuota(stats.availableQuota),
      icon: Wallet,
    },
    {
      key: 'used',
      label: t('Used'),
      value: formatQuota(stats.usedQuota),
      icon: TicketCheck,
    },
    {
      key: 'expiring',
      label: t('Expiring soon'),
      value: String(stats.expiringCount),
      icon: Clock,
    },
    {
      key: 'claimable',
      label: t('Claimable activities'),
      value: String(stats.claimableCount),
      icon: Gift,
    },
  ]

  return (
    <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
      {tiles.map((tile) => (
        <Card key={tile.key} size='sm'>
          <CardHeader>
            <CardTitle className='text-muted-foreground flex items-center gap-2 text-sm font-normal'>
              <tile.icon className='size-4' aria-hidden='true' />
              {tile.label}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <span className='text-xl font-semibold tabular-nums'>
              {tile.value}
            </span>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
