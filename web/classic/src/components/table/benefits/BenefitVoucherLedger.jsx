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

import React, { useEffect, useState } from 'react';
import { Button, Empty, SideSheet, Spin } from '@douyinfe/semi-ui';
import { RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, renderQuota, timestamp2string } from '../../../helpers';
import { benefitLedgerTypeLabel } from '../../benefits/benefitLabels';

const parseMetadata = (raw) => {
  if (!raw) return null;
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
};

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

// Admin-only view of a single voucher's ledger: unlike the user-facing
// sheet, this may render operator/reason metadata (e.g. who voided the
// voucher and why), which must never leak to the voucher's owner.
export default function BenefitVoucherLedger({ voucherId, onClose }) {
  const { t } = useTranslation();
  const [entries, setEntries] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const load = async () => {
    if (!voucherId) return;
    setLoading(true);
    setError('');
    try {
      const response = await API.get(
        `/api/benefit/admin/vouchers/${voucherId}/ledger`,
      );
      if (!response.data?.success) {
        setEntries([]);
        setError(response.data?.message || t('Failed to load voucher ledger'));
        return;
      }
      setEntries(response.data?.data || []);
    } catch (err) {
      setEntries([]);
      setError(
        err?.response?.data?.message ||
          err?.message ||
          t('Failed to load voucher ledger'),
      );
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [voucherId]);

  return (
    <SideSheet
      title={t('Voucher ledger (admin)')}
      visible={voucherId != null}
      onCancel={onClose}
      width='min(620px, 100vw)'
    >
      <div className='mb-3 flex items-center justify-end'>
        <Button
          theme='borderless'
          size='small'
          icon={<RefreshCw size={14} />}
          onClick={load}
        >
          {t('Refresh')}
        </Button>
      </div>

      {loading ? (
        <Spin spinning style={{ width: '100%', padding: 32 }} />
      ) : error ? (
        <div className='rounded-lg border border-[var(--semi-color-danger-light-default)] bg-[var(--semi-color-danger-light-default)] p-4 text-sm text-[var(--semi-color-danger)]'>
          <div className='mb-2'>{error}</div>
          <Button size='small' onClick={load}>
            {t('Retry')}
          </Button>
        </div>
      ) : entries.length === 0 ? (
        <Empty description={t('No transactions yet')} />
      ) : (
        <ul className='grid gap-2'>
          {entries.map((entry) => {
            const metadata = parseMetadata(entry.metadata);
            return (
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
                <div className='mt-1 flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-[var(--semi-color-text-3)]'>
                  {entry.request_id && (
                    <span className='truncate'>
                      {t('Request')}: {entry.request_id}
                    </span>
                  )}
                  {entry.log_id > 0 && (
                    <span>
                      {t('Log')}: #{entry.log_id}
                    </span>
                  )}
                </div>
                {metadata?.operator_id > 0 && (
                  <div className='mt-1 text-xs text-[var(--semi-color-text-3)]'>
                    {t('Operator')}: #{metadata.operator_id}
                  </div>
                )}
                {metadata?.reason && (
                  <div className='mt-1 text-xs text-[var(--semi-color-text-3)]'>
                    {t('Reason')}: {metadata.reason}
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </SideSheet>
  );
}
