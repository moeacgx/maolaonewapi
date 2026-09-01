/*
Copyright (C) 2025 QuantumNous

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

import React from 'react';
import { Button, Card, Tag, Typography } from '@douyinfe/semi-ui';
import { Gift } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { renderQuota } from '../../helpers';
import {
  benefitClaimReasonLabel,
  isBenefitActivityClaimable,
} from './benefitLabels';

const { Title, Text } = Typography;

// For a fixed-amount activity every share is identical, so total/count is
// exact. For a random-amount activity individual shares vary, so this is
// only an average — real per-share min/max is admin-only information.
const averageSharePerCount = (activity) => {
  const count = Number(activity.total_count || 0);
  if (count <= 0) return 0;
  return Number(activity.total_quota || 0) / count;
};

export default function ClaimableActivityCard(props) {
  const { t } = useTranslation();
  const { activity, onClaim, claiming } = props;
  const claimable = isBenefitActivityClaimable(activity);

  return (
    <Card
      className='!rounded-xl border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] shadow-sm'
      bodyStyle={{ padding: 16 }}
    >
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div className='min-w-0'>
          <Title heading={6} className='!mb-1 truncate'>
            {activity.name}
          </Title>
          <Text type='tertiary' size='small' className='truncate'>
            {activity.group_name_snapshot}
          </Text>
        </div>
        {activity.has_claimed ? (
          <Tag color='green'>{t('Already claimed')}</Tag>
        ) : claimable ? (
          <Button
            theme='solid'
            type='primary'
            icon={<Gift size={14} />}
            loading={claiming}
            onClick={() => onClaim(activity.id)}
          >
            {t('Claim')}
          </Button>
        ) : (
          <Tag color='grey'>
            {benefitClaimReasonLabel(t, activity.eligibility_reason)}
          </Tag>
        )}
      </div>

      <div className='mt-3 grid grid-cols-2 gap-3 border-t border-[var(--semi-color-border)] pt-3 text-sm sm:grid-cols-3'>
        <div>
          <Text type='tertiary' size='small'>
            {t('Shares')}
          </Text>
          <div className='font-semibold'>{activity.total_count || 0}</div>
        </div>
        <div>
          <Text type='tertiary' size='small'>
            {t('Avg. amount per share')}
          </Text>
          <div className='font-semibold'>
            {renderQuota(averageSharePerCount(activity))}
          </div>
        </div>
        <div>
          <Text type='tertiary' size='small'>
            {t('Personal validity')}
          </Text>
          <div className='font-semibold'>
            {t('{{hours}}h', {
              hours: Number(activity.personal_valid_hours || 0),
            })}
          </div>
        </div>
      </div>
    </Card>
  );
}
