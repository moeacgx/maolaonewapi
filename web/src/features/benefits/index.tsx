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
import { Gift } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Empty, EmptyDescription, EmptyTitle } from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'

import {
  claimBenefitActivity,
  getBenefitActivities,
  getBenefitVouchers,
} from './api'
import { BenefitActivitiesPanel } from './components/benefit-activities-panel'
import { BenefitSummary } from './components/benefit-summary'
import { ClaimableActivityCard } from './components/claimable-activity-card'
import { UserVoucherCard } from './components/user-voucher-card'

export function BenefitActivities() {
  return <BenefitActivitiesPanel />
}

export function UserBenefits() {
  const { t } = useTranslation()
  const [claimingId, setClaimingId] = useState<number | null>(null)
  const activities = useQuery({
    queryKey: ['benefit', 'activities'],
    queryFn: getBenefitActivities,
  })
  const vouchers = useQuery({
    queryKey: ['benefit', 'vouchers'],
    queryFn: getBenefitVouchers,
  })

  const claim = async (id: number) => {
    setClaimingId(id)
    try {
      const response = await claimBenefitActivity(id)
      if (!response.success) {
        toast.error(response.message ?? t('Unable to claim benefit'))
        return
      }
      toast.success(t('Benefit claimed'))
      await activities.refetch()
      await vouchers.refetch()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Unable to claim benefit')
      )
    } finally {
      setClaimingId(null)
    }
  }

  const activityById = new Map((activities.data ?? []).map((a) => [a.id, a]))
  const isLoading = activities.isLoading || vouchers.isLoading
  const hasError = activities.isError || vouchers.isError

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='inline-flex items-center gap-2'>
          <Gift className='size-5' />
          {t('Activity Benefits')}
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='grid min-w-0 gap-6'>
          {hasError ? (
            <Empty className='border'>
              <EmptyTitle>{t('Unable to load benefit activities')}</EmptyTitle>
              <EmptyDescription>{t('Please try again')}</EmptyDescription>
              <Button
                type='button'
                size='sm'
                variant='outline'
                onClick={() => {
                  void activities.refetch()
                  void vouchers.refetch()
                }}
              >
                {t('Retry')}
              </Button>
            </Empty>
          ) : (
            <BenefitSummary
              vouchers={vouchers.data ?? []}
              activities={activities.data ?? []}
            />
          )}
          <section className='grid gap-3'>
            <h2 className='text-lg font-semibold'>{t('My vouchers')}</h2>
            {isLoading ? (
              <div className='grid gap-3 md:grid-cols-2'>
                <Skeleton className='h-40 w-full' />
                <Skeleton className='h-40 w-full' />
              </div>
            ) : null}
            {!isLoading && (vouchers.data ?? []).length === 0 ? (
              <Empty className='border'>
                <EmptyTitle>{t('No vouchers yet')}</EmptyTitle>
              </Empty>
            ) : null}
            {!isLoading && (vouchers.data ?? []).length > 0 ? (
              <div className='grid gap-3 md:grid-cols-2'>
                {(vouchers.data ?? []).map((voucher) => (
                  <UserVoucherCard
                    key={voucher.id}
                    voucher={voucher}
                    activity={activityById.get(voucher.activity_id)}
                  />
                ))}
              </div>
            ) : null}
          </section>
          <section className='grid gap-3'>
            <h2 className='text-lg font-semibold'>
              {t('Available activities')}
            </h2>
            {isLoading ? (
              <div className='grid gap-3 md:grid-cols-2'>
                <Skeleton className='h-40 w-full' />
                <Skeleton className='h-40 w-full' />
              </div>
            ) : null}
            {!isLoading && (activities.data ?? []).length === 0 ? (
              <Empty className='border'>
                <EmptyTitle>{t('No benefit activities')}</EmptyTitle>
              </Empty>
            ) : null}
            {!isLoading && (activities.data ?? []).length > 0 ? (
              <div className='grid gap-3 md:grid-cols-2'>
                {(activities.data ?? []).map((activity) => (
                  <ClaimableActivityCard
                    key={activity.id}
                    activity={activity}
                    claiming={claimingId === activity.id}
                    onClaim={(id) => void claim(id)}
                  />
                ))}
              </div>
            ) : null}
          </section>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
