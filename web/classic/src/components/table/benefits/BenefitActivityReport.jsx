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
import { useTranslation } from 'react-i18next';
import { getCurrencyConfig, renderQuota } from '../../../helpers';
import { formatDisplayAmount } from '../../benefits/benefitLabels';

const formatQuota = (quota) => renderQuota(Number(quota || 0));

const reportPercentage = (value, total) => {
  const numericValue = Number(value || 0);
  const numericTotal = Number(total || 0);
  if (
    !Number.isFinite(numericValue) ||
    !Number.isFinite(numericTotal) ||
    numericTotal <= 0
  ) {
    return 0;
  }
  return Math.min(
    100,
    Math.max(0, Math.round((numericValue / numericTotal) * 100)),
  );
};

const ReportMetric = ({ label, value, note, tone = 'neutral' }) => (
  <div
    className={`min-h-[118px] rounded-lg border p-4 ${
      tone === 'primary'
        ? 'border-blue-200 bg-blue-50/80'
        : tone === 'success'
          ? 'border-emerald-200 bg-emerald-50/80'
          : 'border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)]'
    }`}
  >
    <div className='text-[var(--semi-color-text-2)] text-xs'>{label}</div>
    <div className='mt-3 text-2xl font-bold leading-none text-[var(--semi-color-text-0)]'>
      {value}
    </div>
    <div className='mt-2 text-xs text-[var(--semi-color-text-2)]'>{note}</div>
  </div>
);

const ReportDetailRow = ({ label, value }) => (
  <div className='flex items-center justify-between gap-4 border-t border-[var(--semi-color-border)] py-2.5 text-sm first:border-t-0 first:pt-0 last:pb-0'>
    <span className='text-[var(--semi-color-text-2)]'>{label}</span>
    <strong className='text-right tabular-nums text-[var(--semi-color-text-0)]'>
      {value}
    </strong>
  </div>
);

// `report` is the authoritative aggregate from GET .../activities/:id/report
// (model.BenefitActivityReport): every count and quota total shown here
// comes directly from that response. This view must never re-paginate the
// voucher list or recompute totals client-side — the backend already
// aggregates issued/used/expired counts server-side for exactly this view.
export default function BenefitActivityReport({ activity, report }) {
  const { t } = useTranslation();
  const currency = getCurrencyConfig();
  const isDraft = activity?.status === 'draft';
  const totalQuota = Number(report.total_quota || activity?.total_quota || 0);
  const undistributedQuota = isDraft
    ? totalQuota
    : Number(report.undistributed_quota || 0);
  const distributedQuota = Number(report.distributed_quota || 0);
  const usedQuota = Number(report.used_quota || 0);
  const expiredUnusedQuota = Number(report.expired_unused_quota || 0);
  const availableQuota = Math.max(
    0,
    totalQuota - usedQuota - expiredUnusedQuota,
  );
  const usedPercent = reportPercentage(usedQuota, totalQuota);
  const totalCount = Number(report.total_count || activity?.total_count || 0);
  const distributedCount = Number(report.distributed_count || 0);
  const usedCount = Number(report.used_count || 0);
  const expiredCount = Number(report.expired_count || 0);
  const activityGroup = activity?.group_name_snapshot || t('Unknown group');
  // claim_paid_threshold is already expressed in the site's current
  // quota_display_type by the backend (CNYCentsToDisplayAmount), the same
  // as every other amount field here — it is never run through
  // renderQuota() because it is not a quota value, but it otherwise follows
  // the exact same display-type formatting as the rest of the report.
  const claimThreshold = Number(activity?.claim_paid_threshold || 0);

  return (
    <div className='grid gap-6 border-t border-[var(--semi-color-border)] pt-5'>
      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        <ReportMetric
          label={t('Total budget')}
          value={formatQuota(totalQuota)}
          note={t('Total quota this activity can distribute')}
          tone='primary'
        />
        <ReportMetric
          label={t('Issued')}
          value={
            <>
              {distributedCount}{' '}
              <span className='text-sm font-normal'>
                / {totalCount} {t('shares')}
              </span>
            </>
          }
          note={`${t('Issued amount')} ${formatQuota(distributedQuota)}`}
        />
        <ReportMetric
          label={t('Used')}
          value={formatQuota(usedQuota)}
          note={`${t('Of total budget')} ${usedPercent}%`}
        />
        <ReportMetric
          label={t('Available')}
          value={formatQuota(availableQuota)}
          note={t('Not yet issued or used')}
          tone='success'
        />
      </div>

      <section className='grid gap-3'>
        <div className='flex items-baseline justify-between gap-3'>
          <h3 className='m-0 text-sm font-bold'>{t('Spend progress')}</h3>
          <span className='text-xs text-[var(--semi-color-text-2)]'>
            {t('Used')} {formatQuota(usedQuota)} / {formatQuota(totalQuota)}
          </span>
        </div>
        <div className='rounded-lg border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] p-4'>
          <div className='flex items-center justify-between gap-3'>
            <strong className='text-sm'>
              {usedPercent > 0
                ? `${t('Used')} ${usedPercent}%`
                : t('No usage yet')}
            </strong>
            <span className='text-xs font-bold text-emerald-600'>
              {usedPercent > 0 ? t('In progress') : t('Not started')}
            </span>
          </div>
          <div className='mt-3 h-2.5 overflow-hidden rounded-full bg-slate-200'>
            <div
              className='h-full rounded-full bg-blue-600 transition-[width]'
              style={{ width: `${usedPercent}%` }}
            />
          </div>
          <div className='mt-2 flex justify-between gap-3 text-xs text-[var(--semi-color-text-2)]'>
            <span>
              {formatQuota(usedQuota)} {t('Used')}
            </span>
            <span>
              {formatQuota(totalQuota)} {t('Total budget')}
            </span>
          </div>
        </div>
      </section>

      <section className='grid gap-3'>
        <h3 className='m-0 text-sm font-bold'>{t('Where the budget went')}</h3>
        <div className='grid overflow-hidden rounded-lg border border-[var(--semi-color-border)] sm:grid-cols-2 xl:grid-cols-4'>
          <div className='grid gap-2 border-b border-[var(--semi-color-border)] p-4 sm:border-r xl:border-b-0'>
            <span className='text-xs text-[var(--semi-color-text-2)]'>
              {t('Not yet issued')}
            </span>
            <strong className='text-lg'>
              {formatQuota(undistributedQuota)}
            </strong>
            <span className='text-xs text-[var(--semi-color-text-2)]'>
              {Math.max(0, totalCount - distributedCount)} {t('shares')}
            </span>
          </div>
          <div className='grid gap-2 border-b border-[var(--semi-color-border)] p-4 xl:border-b-0 xl:border-r'>
            <span className='text-xs text-[var(--semi-color-text-2)]'>
              {t('Issued')}
            </span>
            <strong className='text-lg'>{formatQuota(distributedQuota)}</strong>
            <span className='text-xs text-[var(--semi-color-text-2)]'>
              {distributedCount} {t('shares')}
            </span>
          </div>
          <div className='grid gap-2 border-b border-[var(--semi-color-border)] p-4 sm:border-r xl:border-b-0'>
            <span className='text-xs text-[var(--semi-color-text-2)]'>
              {t('Used')}
            </span>
            <strong className='text-lg'>{formatQuota(usedQuota)}</strong>
            <span className='text-xs text-[var(--semi-color-text-2)]'>
              {usedCount} {t('shares')}
            </span>
          </div>
          <div className='grid gap-2 p-4'>
            <span className='text-xs text-[var(--semi-color-text-2)]'>
              {t('Expired, unused')}
            </span>
            <strong className='text-lg'>
              {formatQuota(expiredUnusedQuota)}
            </strong>
            <span className='text-xs text-[var(--semi-color-text-2)]'>
              {expiredCount} {t('shares/vouchers')}
            </span>
          </div>
        </div>
      </section>

      <div className='grid gap-4 lg:grid-cols-[1.1fr_0.9fr]'>
        <div className='rounded-lg border border-[var(--semi-color-border)] p-4'>
          <h4 className='mb-3 text-sm font-bold'>{t('Issuance status')}</h4>
          <ReportDetailRow
            label={t('Issuance progress')}
            value={`${distributedCount} / ${totalCount} ${t('shares')}`}
          />
          <ReportDetailRow
            label={t('Expired shares/vouchers')}
            value={`${expiredCount} ${t('shares/vouchers')}`}
          />
        </div>
        <div className='rounded-lg border border-[var(--semi-color-border)] p-4'>
          <h4 className='mb-3 text-sm font-bold'>{t('Activity settings')}</h4>
          <ReportDetailRow label={t('Group')} value={activityGroup} />
          <ReportDetailRow
            label={t('Personal validity')}
            value={t('{{hours}}h', {
              hours: Number(activity?.personal_valid_hours || 0),
            })}
          />
          <ReportDetailRow
            label={t('Claim threshold')}
            value={formatDisplayAmount(t, claimThreshold, currency)}
          />
        </div>
      </div>
    </div>
  );
}
