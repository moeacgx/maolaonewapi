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
import { Typography } from '@douyinfe/semi-ui';
import { Clock, Gift, TrendingUp, Wallet } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { renderQuota } from '../../helpers';
import {
  isBenefitActivityClaimable,
  isBenefitVoucherExpiringSoon,
} from './benefitLabels';

const { Text } = Typography;

const SummaryStat = ({ icon, label, value, note }) => (
  <div className='rounded-xl border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] p-4'>
    <div className='flex items-center gap-2 text-[var(--semi-color-text-2)]'>
      {icon}
      <Text type='tertiary' size='small'>
        {label}
      </Text>
    </div>
    <div className='mt-2 text-xl font-bold leading-none'>{value}</div>
    {note && (
      <div className='mt-1.5 text-xs text-[var(--semi-color-text-2)]'>
        {note}
      </div>
    )}
  </div>
);

export default function BenefitSummary(props) {
  const { t } = useTranslation();
  const { vouchers, activities, now } = props;

  const availableQuota = vouchers
    .filter((voucher) => voucher.status === 'active')
    .reduce(
      (total, voucher) => total + Number(voucher.remaining_quota || 0),
      0,
    );

  const usedQuota = vouchers.reduce(
    (total, voucher) => total + Number(voucher.used_quota || 0),
    0,
  );

  const expiringVouchers = vouchers.filter((voucher) =>
    isBenefitVoucherExpiringSoon(voucher, now),
  );
  const expiringQuota = expiringVouchers.reduce(
    (total, voucher) => total + Number(voucher.remaining_quota || 0),
    0,
  );

  const claimableCount = activities.filter(isBenefitActivityClaimable).length;

  return (
    <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-4'>
      <SummaryStat
        icon={<Wallet size={16} />}
        label={t('Available benefit')}
        value={renderQuota(availableQuota)}
      />
      <SummaryStat
        icon={<TrendingUp size={16} />}
        label={t('Used')}
        value={renderQuota(usedQuota)}
      />
      <SummaryStat
        icon={<Clock size={16} />}
        label={t('Expiring soon')}
        value={renderQuota(expiringQuota)}
        note={t('{{count}} voucher(s)', { count: expiringVouchers.length })}
      />
      <SummaryStat
        icon={<Gift size={16} />}
        label={t('Claimable activities')}
        value={claimableCount}
      />
    </div>
  );
}
