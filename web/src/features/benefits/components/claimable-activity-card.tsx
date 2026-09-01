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
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatQuota, formatTimestampToDate } from '@/lib/format'

import { claimEligibilityLabel } from '../lib/labels'
import type { BenefitActivityUserView } from '../types'

type ClaimableActivityCardProps = {
  activity: BenefitActivityUserView
  onClaim: (id: number) => void
  claiming: boolean
}

export function ClaimableActivityCard(props: ClaimableActivityCardProps) {
  const { t } = useTranslation()
  const activity = props.activity
  const perShareQuota =
    activity.amount_mode === 'fixed'
      ? formatQuota(activity.fixed_quota)
      : `${formatQuota(activity.min_quota)} ~ ${formatQuota(activity.max_quota)}`
  const canClaim = activity.eligible && !activity.has_claimed

  return (
    <Card size='sm'>
      <CardHeader>
        <CardTitle className='truncate'>{activity.name}</CardTitle>
        <span className='text-muted-foreground text-sm'>
          {activity.group_name_snapshot}
        </span>
      </CardHeader>
      <CardContent className='grid gap-3'>
        {activity.description ? (
          <p className='text-muted-foreground text-sm'>
            {activity.description}
          </p>
        ) : null}
        <div className='grid grid-cols-2 gap-2 text-sm'>
          <div>
            <p className='text-muted-foreground text-xs'>
              {t('Remaining shares')}
            </p>
            <p className='font-medium tabular-nums'>
              {activity.remaining_count}
            </p>
          </div>
          <div>
            <p className='text-muted-foreground text-xs'>
              {t('Per-voucher amount')}
            </p>
            <p className='font-medium tabular-nums'>{perShareQuota}</p>
          </div>
          <div>
            <p className='text-muted-foreground text-xs'>
              {t('Personal validity')}
            </p>
            <p className='font-medium tabular-nums'>
              {t('{{hours}}h', { hours: activity.personal_valid_hours })}
            </p>
          </div>
          <div>
            <p className='text-muted-foreground text-xs'>{t('Ends')}</p>
            <p className='font-medium tabular-nums'>
              {formatTimestampToDate(activity.ends_at)}
            </p>
          </div>
        </div>
        <div className='flex flex-wrap items-center gap-2'>
          {activity.has_claimed ? (
            <StatusBadge
              label={t('Already claimed')}
              variant='neutral'
              copyable={false}
            />
          ) : (
            <>
              <StatusBadge
                label={
                  activity.eligible
                    ? t('Eligible')
                    : claimEligibilityLabel(activity.eligibility_reason, t)
                }
                variant={activity.eligible ? 'success' : 'neutral'}
                copyable={false}
              />
              <Button
                type='button'
                size='sm'
                disabled={!canClaim || props.claiming}
                onClick={() => props.onClaim(activity.id)}
              >
                {props.claiming ? t('Claiming...') : t('Claim')}
              </Button>
            </>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
