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
import {
  Button,
  Empty,
  SideSheet,
  Spin,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { History, RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { renderQuota, timestamp2string } from '../../helpers';
import {
  benefitLedgerTypeLabel,
  benefitVoucherStatusColor,
  benefitVoucherStatusLabel,
} from './benefitLabels';

const { Title, Text } = Typography;

// Renders a single ledger row's quota delta with an explicit sign so a
// refund and a deduction are visually distinguishable at a glance.
const LedgerDelta = ({ delta }) => {
  const numeric = Number(delta || 0);
  const tone =
    numeric > 0
      ? 'text-[var(--semi-color-success)]'
      : numeric < 0
        ? 'text-[var(--semi-color-danger)]'
        : 'text-[var(--semi-color-text-1)]';
  const sign = numeric > 0 ? '+' : '';
  return (
    <span className={`font-semibold tabular-nums ${tone}`}>
      {sign}
      {renderQuota(numeric)}
    </span>
  );
};

export default function UserVoucherLedgerSheet(props) {
  const { t } = useTranslation();
  const { visible, voucher, entries, loading, error, onRetry, onCancel } =
    props;

  return (
    <SideSheet
      title={
        <span className='inline-flex items-center gap-2'>
          <History size={16} />
          {t('Voucher ledger')}
        </span>
      }
      visible={visible}
      onCancel={onCancel}
      width='min(620px, 100vw)'
    >
      {voucher && (
        <div className='mb-4 grid gap-3 rounded-xl border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] p-4 sm:grid-cols-2'>
          <div>
            <Text type='tertiary' size='small'>
              {t('Remaining balance')}
            </Text>
            <div className='text-lg font-bold'>
              {renderQuota(voucher.remaining_quota)}
            </div>
          </div>
          <div>
            <Text type='tertiary' size='small'>
              {t('Status')}
            </Text>
            <div>
              <Tag color={benefitVoucherStatusColor(voucher.status)}>
                {benefitVoucherStatusLabel(t, voucher.status)}
              </Tag>
            </div>
          </div>
          <div>
            <Text type='tertiary' size='small'>
              {t('Original amount')}
            </Text>
            <div>{renderQuota(voucher.original_quota)}</div>
          </div>
          <div>
            <Text type='tertiary' size='small'>
              {t('Used amount')}
            </Text>
            <div>{renderQuota(voucher.used_quota)}</div>
          </div>
        </div>
      )}

      <div className='mb-3 flex items-center justify-between'>
        <Title heading={6} className='!mb-0'>
          {t('Transaction history')}
        </Title>
        <Button
          theme='borderless'
          size='small'
          icon={<RefreshCw size={14} />}
          onClick={onRetry}
        >
          {t('Refresh')}
        </Button>
      </div>

      {loading ? (
        <Spin spinning style={{ width: '100%', padding: 32 }} />
      ) : error ? (
        <div className='rounded-lg border border-[var(--semi-color-danger-light-default)] bg-[var(--semi-color-danger-light-default)] p-4 text-sm text-[var(--semi-color-danger)]'>
          <div className='mb-2'>{error}</div>
          <Button size='small' onClick={onRetry}>
            {t('Retry')}
          </Button>
        </div>
      ) : entries.length === 0 ? (
        <Empty description={t('No transactions yet')} />
      ) : (
        <ul className='grid gap-2'>
          {entries.map((entry) => (
            <li
              key={entry.id}
              className='rounded-lg border border-[var(--semi-color-border)] p-3'
            >
              <div className='flex items-center justify-between gap-3'>
                <span className='font-medium'>
                  {benefitLedgerTypeLabel(t, entry.type)}
                </span>
                <LedgerDelta delta={entry.quota_delta} />
              </div>
              <div className='mt-1 flex flex-wrap items-center justify-between gap-2 text-xs text-[var(--semi-color-text-2)]'>
                <span>
                  {t('Balance after')}: {renderQuota(entry.balance_after)}
                </span>
                <span>{timestamp2string(entry.created_at)}</span>
              </div>
              {(entry.request_id || entry.log_id > 0) && (
                <div className='mt-1 truncate text-xs text-[var(--semi-color-text-3)]'>
                  {entry.request_id && (
                    <span>
                      {t('Request')}: {entry.request_id}
                    </span>
                  )}
                  {entry.request_id && entry.log_id > 0 && ' · '}
                  {entry.log_id > 0 && (
                    <span>
                      {t('Log')}: #{entry.log_id}
                    </span>
                  )}
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
    </SideSheet>
  );
}
