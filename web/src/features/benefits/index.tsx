import { useQuery } from '@tanstack/react-query'
import { Gift, TicketCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatLogQuota } from '@/lib/format'

import {
  claimBenefitActivity,
  getBenefitActivities,
  getBenefitVouchers,
} from './api'
import { BenefitActivitiesPanel } from './components/benefit-activities-panel'

function displayVoucherAmount(
  voucher: { original_amount?: number; original_amount_cents?: number }
) {
  if (typeof voucher.original_amount === 'number') {
    return voucher.original_amount.toFixed(2)
  }
  return ((voucher.original_amount_cents ?? 0) / 100).toFixed(2)
}

export function BenefitActivities() {
  return <BenefitActivitiesPanel />
}

export function UserBenefits() {
  const { t } = useTranslation()
  const activities = useQuery({
    queryKey: ['benefit', 'activities'],
    queryFn: getBenefitActivities,
  })
  const vouchers = useQuery({
    queryKey: ['benefit', 'vouchers'],
    queryFn: getBenefitVouchers,
  })
  const claim = async (id: number) => {
    const response = await claimBenefitActivity(id)
    if (!response.success) {
      toast.error(response.message ?? t('Unable to claim benefit'))
      return
    }
    toast.success(t('Benefit claimed'))
    await activities.refetch()
    await vouchers.refetch()
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='inline-flex items-center gap-2'>
          <Gift className='size-5' />
          {t('Activity Benefits')}
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='grid gap-6'>
          {activities.isError || vouchers.isError ? (
            <p className='text-destructive text-sm'>
              {t('Unable to load benefit activities')}
            </p>
          ) : null}
          <section className='grid gap-3'>
            <h2 className='text-lg font-semibold'>{t('My vouchers')}</h2>
            <div className='grid gap-3 md:grid-cols-2'>
              {(vouchers.data ?? []).map((voucher) => {
                const activity = (activities.data ?? []).find(
                  (item) => item.id === voucher.activity_id
                )
                return (
                  <Card key={voucher.id} size='sm'>
                    <CardHeader>
                      <CardTitle>
                        <span className='inline-flex items-center gap-2'>
                          <TicketCheck className='size-4' />
                          {formatLogQuota(voucher.remaining_quota)}
                        </span>
                      </CardTitle>
                    </CardHeader>
                    <CardContent className='text-muted-foreground grid gap-1 text-sm'>
                      <span>
                        {t('Status')}: {t(voucher.status)}
                      </span>
                      <span>
                        {t('Original amount')}:{' '}
                        {displayVoucherAmount(voucher)}
                      </span>
                      <span>
                        {t('Used amount')}: {formatLogQuota(voucher.used_quota)}
                      </span>
                      <span>
                        {t('Bound group')}:{' '}
                        {activity?.group_name_snapshot ?? t('Unknown')}
                      </span>
                      <span>
                        {t('Expires')}:{' '}
                        {new Date(voucher.expires_at * 1000).toLocaleString()}
                      </span>
                      {activity ? (
                        <span>
                          {t('Per-user concurrency')}:{' '}
                          {activity.single_user_concurrency_limit || 0}
                        </span>
                      ) : null}
                    </CardContent>
                  </Card>
                )
              })}
              {!vouchers.isLoading && (vouchers.data ?? []).length === 0 ? (
                <p className='text-muted-foreground'>{t('No vouchers yet')}</p>
              ) : null}
            </div>
          </section>
          <section className='grid gap-3'>
            <h2 className='text-lg font-semibold'>
              {t('Available activities')}
            </h2>
            <div className='grid gap-3'>
              {(activities.data ?? []).map((activity) => (
                <Card key={activity.id} size='sm'>
                  <CardHeader>
                    <CardTitle>{activity.name}</CardTitle>
                    <span className='text-muted-foreground text-sm'>
                      {activity.group_name_snapshot}
                    </span>
                  </CardHeader>
                  <CardContent className='flex flex-wrap items-center gap-2'>
                    <StatusBadge
                      label={activity.status}
                      variant='neutral'
                      size='sm'
                      copyable={false}
                    />
                    {activity.has_claimed ? (
                      <span className='text-muted-foreground text-sm'>
                        {t('Already claimed')}
                      </span>
                    ) : (
                      <Button
                        type='button'
                        disabled={!activity.eligible}
                        size='sm'
                        onClick={() => void claim(activity.id)}
                      >
                        {t('Claim')}
                      </Button>
                    )}
                  </CardContent>
                </Card>
              ))}
            </div>
          </section>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
