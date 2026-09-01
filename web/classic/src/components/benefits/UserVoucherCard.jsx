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
import { Card, Tag, Typography } from '@douyinfe/semi-ui';
import { TicketCheck, Users } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { renderQuota, timestamp2string } from '../../helpers';
import {
  benefitVoucherStatusColor,
  benefitVoucherStatusLabel,
  isBenefitVoucherExpiringSoon,
} from './benefitLabels';

const { Title, Text } = Typography;

const usedPercent = (voucher) => {
  const original = Number(voucher.original_quota || 0);
  if (original <= 0) return 0;
  const used = Number(voucher.used_quota || 0);
  return Math.min(100, Math.max(0, Math.round((used / original) * 100)));
};

export default function UserVoucherCard(props) {
  const { t } = useTranslation();
  const { voucher, activity, now } = props;
  const percent = usedPercent(voucher);
  const expiringSoon = isBenefitVoucherExpiringSoon(voucher, now);
  // The voucher itself now carries activity_name/group_name_snapshot
  // (joined server-side with Unscoped(), so it survives the owning activity
  // being soft-deleted). The `activities` list lookup is only a fallback
  // for older cached data and for fields that only ever lived on the
  // activity (e.g. the per-user concurrency limit).
  const activityName = voucher.activity_name || activity?.name;
  const groupName =
    voucher.group_name_snapshot || activity?.group_name_snapshot;

  return (
    <Card
      className='!rounded-lg border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] shadow-sm'
      bodyStyle={{ padding: 16 }}
    >
      <div className='flex items-start justify-between gap-3'>
        <span className='inline-flex items-center gap-2 text-lg font-bold'>
          <TicketCheck size={18} />
          {renderQuota(voucher.remaining_quota)}
        </span>
        <div className='flex flex-col items-end gap-1'>
          <Tag color={benefitVoucherStatusColor(voucher.status)}>
            {benefitVoucherStatusLabel(t, voucher.status)}
          </Tag>
          {expiringSoon && (
            <Tag color='orange' size='small'>
              {t('Expiring soon')}
            </Tag>
          )}
        </div>
      </div>

      <Title heading={6} className='!mb-0 !mt-2 truncate'>
        {activityName || t('Benefit voucher')}
      </Title>

      <div className='mt-3'>
        <div className='mb-1 flex items-center justify-between text-xs text-[var(--semi-color-text-2)]'>
          <span>{t('Used')}</span>
          <span>{percent}%</span>
        </div>
        <div className='h-2 overflow-hidden rounded-full bg-[var(--semi-color-fill-0)]'>
          <div
            className='h-full rounded-full bg-blue-600 transition-[width]'
            style={{ width: `${percent}%` }}
          />
        </div>
        <div className='mt-1 flex justify-between text-xs text-[var(--semi-color-text-2)]'>
          <span>
            {t('Used')} {renderQuota(voucher.used_quota)}
          </span>
          <span>
            {t('Original amount')} {renderQuota(voucher.original_quota)}
          </span>
        </div>
      </div>

      <div className='mt-3 grid gap-1.5 border-t border-[var(--semi-color-border)] pt-3 text-sm'>
        <div className='flex items-center justify-between gap-3'>
          <Text type='tertiary'>{t('Claimed at')}</Text>
          <span>{timestamp2string(voucher.claimed_at)}</span>
        </div>
        <div className='flex items-center justify-between gap-3'>
          <Text type='tertiary'>{t('Expires at')}</Text>
          <span>{timestamp2string(voucher.expires_at)}</span>
        </div>
        <div className='flex items-center justify-between gap-3'>
          <Text type='tertiary'>{t('Bound group')}</Text>
          <span className='truncate'>{groupName || t('Unknown')}</span>
        </div>
        {Number(activity?.single_user_concurrency_limit || 0) > 0 && (
          <div className='flex items-center justify-between gap-3'>
            <Text type='tertiary' className='inline-flex items-center gap-1'>
              <Users size={12} />
              {t('Per-user concurrency limit')}
            </Text>
            <span>{activity.single_user_concurrency_limit}</span>
          </div>
        )}
        {voucher.status === 'voided' && voucher.void_reason && (
          <div className='flex items-start justify-between gap-3'>
            <Text type='tertiary'>{t('Void reason')}</Text>
            <span className='max-w-[60%] text-right'>
              {voucher.void_reason}
            </span>
          </div>
        )}
      </div>
    </Card>
  );
}
