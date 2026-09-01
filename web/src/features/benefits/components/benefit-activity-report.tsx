import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Empty, EmptyDescription, EmptyTitle } from '@/components/ui/empty'
import {
  Progress,
  ProgressIndicator,
  ProgressTrack,
} from '@/components/ui/progress'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { formatQuota } from '@/lib/format'

import { getAdminBenefitReport } from '../api'
import type { BenefitActivity } from '../types'

type BenefitActivityReportProps = {
  activity: BenefitActivity | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function BenefitActivityReport(props: BenefitActivityReportProps) {
  const { t } = useTranslation()
  const activityId = props.activity?.id ?? null
  const query = useQuery({
    queryKey: ['benefit', 'admin', 'report', activityId],
    queryFn: () => getAdminBenefitReport(activityId as number),
    enabled: props.open && activityId != null,
  })

  const report = query.data
  const usedRatio =
    report && report.total_quota > 0
      ? Math.min(100, (report.used_quota / report.total_quota) * 100)
      : 0

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className='sm:max-w-lg'>
        <SheetHeader>
          <SheetTitle>{t('Activity report')}</SheetTitle>
        </SheetHeader>
        <div className='grid gap-4 overflow-y-auto px-4 pb-4'>
          {query.isLoading ? (
            <div className='grid gap-2'>
              <Skeleton className='h-6 w-full' />
              <Skeleton className='h-24 w-full' />
            </div>
          ) : null}
          {query.isError ? (
            <Empty>
              <EmptyTitle>{t('Failed to load benefit report')}</EmptyTitle>
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
          {report ? (
            <>
              <div>
                <span className='text-2xl font-semibold tabular-nums'>
                  {formatQuota(report.total_quota)}
                </span>
                <p className='text-muted-foreground text-xs'>
                  {t('Total budget')}
                </p>
              </div>
              <Progress value={usedRatio}>
                <div className='text-muted-foreground flex justify-between text-xs'>
                  <span>
                    {t('Used')} {formatQuota(report.used_quota)}
                  </span>
                  <span>
                    {t('Total')} {formatQuota(report.total_quota)}
                  </span>
                </div>
                <ProgressTrack>
                  <ProgressIndicator />
                </ProgressTrack>
              </Progress>
              <div className='grid grid-cols-2 gap-3 text-sm'>
                <div className='border-border rounded-md border p-3'>
                  <p className='text-muted-foreground text-xs'>
                    {t('Undistributed')}
                  </p>
                  <p className='font-medium tabular-nums'>
                    {formatQuota(report.undistributed_quota)}
                  </p>
                </div>
                <div className='border-border rounded-md border p-3'>
                  <p className='text-muted-foreground text-xs'>
                    {t('Distributed')}
                  </p>
                  <p className='font-medium tabular-nums'>
                    {formatQuota(report.distributed_quota)}
                  </p>
                </div>
                <div className='border-border rounded-md border p-3'>
                  <p className='text-muted-foreground text-xs'>{t('Used')}</p>
                  <p className='font-medium tabular-nums'>
                    {formatQuota(report.used_quota)}
                  </p>
                </div>
                <div className='border-border rounded-md border p-3'>
                  <p className='text-muted-foreground text-xs'>
                    {t('Expired unused')}
                  </p>
                  <p className='font-medium tabular-nums'>
                    {formatQuota(report.expired_unused_quota)}
                  </p>
                </div>
              </div>
              <div className='text-muted-foreground grid grid-cols-2 gap-2 text-xs'>
                <span>
                  {t('Total shares')}: {report.total_count}
                </span>
                <span>
                  {t('Distributed shares')}: {report.distributed_count}
                </span>
                <span>
                  {t('Used-up shares')}: {report.used_count}
                </span>
                <span>
                  {t('Expired shares')}: {report.expired_count}
                </span>
              </div>
            </>
          ) : null}
        </div>
      </SheetContent>
    </Sheet>
  )
}
