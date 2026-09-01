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

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import {
  Button,
  Card,
  Dropdown,
  Empty,
  Form,
  Input,
  Modal,
  Select,
  SideSheet,
  Space,
  Spin,
  Table,
  Tag,
  Toast,
  Typography,
} from '@douyinfe/semi-ui';
import {
  BarChart3,
  ChevronDown,
  Eye,
  FilePenLine,
  Plus,
  RefreshCw,
  SquareX,
  Trash2,
} from 'lucide-react';
import {
  API,
  createGroupOptions,
  extractGroupDetailsResponse,
  renderQuota,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

const { Text, Title } = Typography;

const defaultFormValues = {
  name: '',
  description: '',
  group_id: 0,
  amount_mode: 'fixed',
  total_amount: 10,
  total_count: 10,
  fixed_amount: 1,
  min_amount: 0.5,
  max_amount: 2,
  claim_paid_threshold: 0,
  personal_valid_hours: 24,
  starts_at_text: '',
  ends_at_text: '',
};

const formatBeijingDateTime = (timestamp) =>
  new Date(timestamp * 1000)
    .toLocaleString('sv-SE', { timeZone: 'Asia/Shanghai' })
    .slice(0, 16);

const parseBeijingDateTime = (value) => {
  const timestamp = Date.parse(`${value}:00+08:00`);
  return Number.isFinite(timestamp) ? Math.floor(timestamp / 1000) : 0;
};

const formatAmount = (amount) => `¥${Number(amount || 0).toFixed(2)}`;

const amountFromActivity = (amount, legacyAmount, fallback) => {
  if (Number.isFinite(Number(amount))) return Number(amount);
  if (Number.isFinite(Number(legacyAmount))) return Number(legacyAmount) / 100;
  return fallback;
};

const fixedTotalAmount = (amount, count) => {
  const amountMinor = amountInMinorUnits(amount);
  if (amountMinor === null || !Number.isInteger(count) || count <= 0) {
    return 0;
  }
  return (amountMinor * count) / 100;
};

const amountRange = (minimum, maximum, count) => {
  const minimumMinor = amountInMinorUnits(minimum);
  const maximumMinor = amountInMinorUnits(maximum);
  if (
    minimumMinor === null ||
    maximumMinor === null ||
    !Number.isInteger(count) ||
    count <= 0
  ) {
    return null;
  }
  return {
    minimum: (minimumMinor * count) / 100,
    maximum: (maximumMinor * count) / 100,
  };
};

const validityHoursFromActivity = (hours, legacySeconds, fallback) => {
  if (Number.isFinite(Number(hours))) return Number(hours);
  if (Number.isFinite(Number(legacySeconds)))
    return Number(legacySeconds) / 3600;
  return fallback;
};

const toFormValues = (activity) => ({
  ...defaultFormValues,
  ...activity,
  fixed_amount: amountFromActivity(
    activity.fixed_amount,
    activity.fixed_amount_cents,
    defaultFormValues.fixed_amount,
  ),
  min_amount: amountFromActivity(
    activity.min_amount,
    activity.min_amount_cents,
    defaultFormValues.min_amount,
  ),
  max_amount: amountFromActivity(
    activity.max_amount,
    activity.max_amount_cents,
    defaultFormValues.max_amount,
  ),
  claim_paid_threshold: amountFromActivity(
    activity.claim_paid_threshold,
    activity.claim_paid_threshold_cents,
    defaultFormValues.claim_paid_threshold,
  ),
  personal_valid_hours: validityHoursFromActivity(
    activity.personal_valid_hours,
    activity.personal_valid_seconds,
    defaultFormValues.personal_valid_hours,
  ),
  total_amount:
    activity.amount_mode === 'fixed'
      ? fixedTotalAmount(
          amountFromActivity(
            activity.fixed_amount,
            activity.fixed_amount_cents,
            defaultFormValues.fixed_amount,
          ),
          Number(activity.total_count || defaultFormValues.total_count),
        )
      : amountFromActivity(
          activity.total_amount,
          activity.total_amount_cents,
          defaultFormValues.total_amount,
        ),
  starts_at_text: formatBeijingDateTime(activity.starts_at),
  ends_at_text: formatBeijingDateTime(activity.ends_at),
});

const amountInMinorUnits = (value) => {
  const number = Number(value);
  if (!Number.isFinite(number) || number < 0) return null;
  const minor = Math.round(number * 100);
  return Math.abs(number * 100 - minor) < 1e-7 ? minor : null;
};

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
    className={`min-h-[118px] rounded-xl border p-4 ${
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

const BenefitActivityReportView = ({ activity, report, vouchers, t }) => {
  const voucherList = Array.isArray(vouchers) ? vouchers : [];
  const isDraft = activity?.status === 'draft';
  const totalQuota = Number(report.total_quota || activity?.total_quota || 0);
  const undistributedQuota = isDraft
    ? totalQuota
    : Number(report.undistributed_quota || 0);
  const distributedQuota = Number(report.distributed_quota || 0);
  const usedQuota = Number(report.used_quota || 0);
  const expiredUnusedQuota = Number(report.expired_unused_quota || 0);
  const expiredVoucherUnusedQuota = voucherList.reduce((total, voucher) => {
    if (!['expired', 'voided'].includes(voucher.status)) return total;
    return (
      total +
      Math.max(
        0,
        Number(voucher.original_quota || 0) - Number(voucher.used_quota || 0),
      )
    );
  }, 0);
  const availableQuota = Math.max(
    0,
    totalQuota - usedQuota - expiredUnusedQuota,
  );
  const usedPercent = reportPercentage(usedQuota, totalQuota);
  const count = Number(activity?.total_count || 0);
  const issuedCount = voucherList.length;
  const issuedUnspentCount = voucherList.filter(
    (voucher) =>
      voucher.status === 'active' && Number(voucher.remaining_quota || 0) > 0,
  ).length;
  const usedCount = voucherList.filter(
    (voucher) => Number(voucher.used_quota || 0) > 0,
  ).length;
  const expiredCount = voucherList.filter((voucher) =>
    ['expired', 'voided'].includes(voucher.status),
  ).length;
  const claimedUserCount = new Set(
    voucherList.map((voucher) => voucher.user_id),
  ).size;
  const issuedUnspentQuota = Math.max(
    0,
    distributedQuota - usedQuota - expiredVoucherUnusedQuota,
  );
  const activityGroup = activity?.group_name_snapshot || t('未知分组');

  return (
    <div className='grid gap-6 border-t border-[var(--semi-color-border)] pt-5'>
      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        <ReportMetric
          label={t('总预算')}
          value={formatQuota(totalQuota)}
          note={t('活动可发放的全部额度')}
          tone='primary'
        />
        <ReportMetric
          label={t('已发放')}
          value={
            <>
              {issuedCount}{' '}
              <span className='text-sm font-normal'>
                / {count} {t('份')}
              </span>
            </>
          }
          note={`${t('已发放额度')} ${formatQuota(distributedQuota)}`}
        />
        <ReportMetric
          label={t('已使用')}
          value={formatQuota(usedQuota)}
          note={`${t('占总预算')} ${usedPercent}%`}
        />
        <ReportMetric
          label={t('剩余可用')}
          value={formatQuota(availableQuota)}
          note={t('未发放和未使用额度')}
          tone='success'
        />
      </div>

      <section className='grid gap-3'>
        <div className='flex items-baseline justify-between gap-3'>
          <h3 className='m-0 text-sm font-bold'>{t('资金使用进度')}</h3>
          <span className='text-xs text-[var(--semi-color-text-2)]'>
            {t('已使用')} {formatQuota(usedQuota)} / {formatQuota(totalQuota)}
          </span>
        </div>
        <div className='rounded-xl border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] p-4'>
          <div className='flex items-center justify-between gap-3'>
            <strong className='text-sm'>
              {usedPercent > 0
                ? `${t('已使用')} ${usedPercent}%`
                : t('还没有产生使用记录')}
            </strong>
            <span className='text-xs font-bold text-emerald-600'>
              {usedPercent > 0 ? t('进行中') : t('未开始')}
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
              {formatQuota(usedQuota)} {t('已使用')}
            </span>
            <span>
              {formatQuota(totalQuota)} {t('总预算')}
            </span>
          </div>
        </div>
      </section>

      <section className='grid gap-3'>
        <div className='flex items-baseline justify-between gap-3'>
          <h3 className='m-0 text-sm font-bold'>{t('金额去向')}</h3>
          <span className='text-xs text-[var(--semi-color-text-2)]'>
            {t('额度按系统展示类型显示')}
          </span>
        </div>
        <div className='grid overflow-hidden rounded-xl border border-[var(--semi-color-border)] sm:grid-cols-2 xl:grid-cols-4'>
          <div className='grid gap-2 border-b border-[var(--semi-color-border)] p-4 sm:border-r xl:border-b-0'>
            <span className='text-xs text-[var(--semi-color-text-2)]'>
              {t('待发放')}
            </span>
            <strong className='text-lg'>
              {formatQuota(undistributedQuota)}
            </strong>
            <span className='text-xs text-[var(--semi-color-text-2)]'>
              {Math.max(0, count - issuedCount)} {t('份')}
            </span>
          </div>
          <div className='grid gap-2 border-b border-[var(--semi-color-border)] p-4 xl:border-b-0 xl:border-r'>
            <span className='text-xs text-[var(--semi-color-text-2)]'>
              {t('已发放未使用')}
            </span>
            <strong className='text-lg'>
              {formatQuota(issuedUnspentQuota)}
            </strong>
            <span className='text-xs text-[var(--semi-color-text-2)]'>
              {issuedUnspentCount} {t('份')}
            </span>
          </div>
          <div className='grid gap-2 border-b border-[var(--semi-color-border)] p-4 sm:border-r xl:border-b-0'>
            <span className='text-xs text-[var(--semi-color-text-2)]'>
              {t('已使用')}
            </span>
            <strong className='text-lg'>{formatQuota(usedQuota)}</strong>
            <span className='text-xs text-[var(--semi-color-text-2)]'>
              {usedCount} {t('份')}
            </span>
          </div>
          <div className='grid gap-2 p-4'>
            <span className='text-xs text-[var(--semi-color-text-2)]'>
              {t('过期未使用')}
            </span>
            <strong className='text-lg'>
              {formatQuota(expiredUnusedQuota)}
            </strong>
            <span className='text-xs text-[var(--semi-color-text-2)]'>
              {expiredCount} {t('张')}
            </span>
          </div>
        </div>
      </section>

      <div className='grid gap-4 lg:grid-cols-[1.1fr_0.9fr]'>
        <div className='rounded-xl border border-[var(--semi-color-border)] p-4'>
          <h4 className='mb-3 text-sm font-bold'>{t('发放状态')}</h4>
          <ReportDetailRow
            label={t('发放进度')}
            value={`${issuedCount} / ${count} ${t('份')}`}
          />
          <ReportDetailRow
            label={t('已领取用户')}
            value={`${claimedUserCount} ${t('人')}`}
          />
          <ReportDetailRow
            label={t('已过期券')}
            value={`${expiredCount} ${t('张')}`}
          />
        </div>
        <div className='rounded-xl border border-[var(--semi-color-border)] p-4'>
          <h4 className='mb-3 text-sm font-bold'>{t('活动设置')}</h4>
          <ReportDetailRow label={t('分组')} value={activityGroup} />
          <ReportDetailRow
            label={t('个人有效期')}
            value={`${Number(activity?.personal_valid_hours || 0)} ${t('小时')}`}
          />
          <ReportDetailRow
            label={t('领取门槛')}
            value={formatQuota(
              totalQuota > 0 && Number(activity?.total_amount || 0) > 0
                ? (totalQuota *
                    amountFromActivity(
                      activity?.claim_paid_threshold,
                      activity?.claim_paid_threshold_cents,
                      0,
                    )) /
                    Number(activity.total_amount)
                : 0,
            )}
          />
        </div>
      </div>

      <div className='flex items-start gap-2 rounded-lg border border-blue-200 bg-blue-50 px-3.5 py-3 text-xs leading-5 text-blue-900'>
        <strong className='shrink-0 text-blue-600'>{t('提示')}</strong>
        <span>
          {t(
            '报表额度会按系统当前的额度展示类型显示；表单中的金额仍按元填写。',
          )}
        </span>
      </div>
    </div>
  );
};

export default function BenefitActivitiesPanel() {
  const { t } = useTranslation();
  const [activities, setActivities] = useState([]);
  const [selectedActivities, setSelectedActivities] = useState([]);
  const [loading, setLoading] = useState(true);
  const [editorVisible, setEditorVisible] = useState(false);
  const [editorSessionKey, setEditorSessionKey] = useState(0);
  const [editing, setEditing] = useState(null);
  const [detail, setDetail] = useState(null);
  const [detailData, setDetailData] = useState(null);
  const [reportVouchers, setReportVouchers] = useState([]);
  const [ledger, setLedger] = useState(null);
  const [terminateMode, setTerminateMode] = useState('unused');
  const [terminateReason, setTerminateReason] = useState('');
  const [groupOptions, setGroupOptions] = useState([]);
  const [groupLoading, setGroupLoading] = useState(false);
  const formApiRef = useRef(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const response = await API.get(
        '/api/benefit/admin/activities?p=1&page_size=100',
      );
      setActivities(response.data?.data?.items || []);
    } catch (error) {
      Toast.error(error?.response?.data?.message || t('无法加载福利活动'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    load();
  }, [load]);

  const loadGroupOptions = useCallback(async () => {
    setGroupLoading(true);
    try {
      const response = await API.get('/api/group/details');
      if (response?.data?.success === false) {
        throw new Error(response.data.message || t('获取分组失败'));
      }
      const groups = extractGroupDetailsResponse(response?.data);
      if (groups === null) {
        throw new Error(response?.data?.message || t('获取分组失败'));
      }
      const activeGroups = createGroupOptions(groups).filter(
        (group) =>
          group.status === 1 &&
          Number.isInteger(Number(group.id)) &&
          group.id > 0,
      );
      const nameCounts = activeGroups.reduce((counts, group) => {
        const name = String(group.name || group.code || '').trim();
        counts.set(name, (counts.get(name) || 0) + 1);
        return counts;
      }, new Map());
      setGroupOptions(
        activeGroups.map((group) => {
          const name = String(group.name || group.code || '').trim();
          const code = String(group.code || '').trim();
          return {
            value: group.id,
            label:
              nameCounts.get(name) > 1 && code ? `${name} · ${code}` : name,
            code,
            name,
            ratio: group.ratio,
          };
        }),
      );
    } catch (error) {
      setGroupOptions([]);
      Toast.error(error?.message || t('获取分组失败'));
    } finally {
      setGroupLoading(false);
    }
  }, [t]);

  useEffect(() => {
    if (editorVisible) loadGroupOptions();
  }, [editorVisible, loadGroupOptions]);

  const closeEditor = () => {
    formApiRef.current = null;
    setEditorVisible(false);
  };

  const openCreate = () => {
    setEditing(null);
    setEditorSessionKey((key) => key + 1);
    setEditorVisible(true);
  };

  const openEdit = (activity) => {
    setEditing(activity);
    setEditorSessionKey((key) => key + 1);
    setEditorVisible(true);
  };

  const editorGroupOptions = useMemo(() => {
    const groupId = Number(editing?.group_id || 0);
    if (groupId <= 0) return groupOptions;
    if (groupOptions.some((option) => option.value === groupId)) {
      return groupOptions;
    }
    return [
      {
        value: groupId,
        label: editing.group_name_snapshot || `#${groupId}`,
      },
      ...groupOptions,
    ];
  }, [editing, groupOptions]);

  const saveForm = async (values) => {
    const amountMode = values.amount_mode;
    const totalCount = Number(values.total_count || 0);
    const fixedAmount = Number(values.fixed_amount || 0);
    const totalAmount =
      amountMode === 'fixed'
        ? fixedTotalAmount(fixedAmount, totalCount)
        : Number(values.total_amount || 0);
    const payload = {
      name: values.name,
      description: values.description,
      group_id: Number(values.group_id || 0),
      amount_mode: amountMode,
      total_amount: totalAmount,
      total_count: totalCount,
      fixed_amount: amountMode === 'fixed' ? fixedAmount : 0,
      min_amount: amountMode === 'random' ? Number(values.min_amount || 0) : 0,
      max_amount: amountMode === 'random' ? Number(values.max_amount || 0) : 0,
      claim_paid_threshold: Number(values.claim_paid_threshold || 0),
      personal_valid_hours: Number(values.personal_valid_hours || 0),
      starts_at: parseBeijingDateTime(values.starts_at_text),
      ends_at: parseBeijingDateTime(values.ends_at_text),
    };
    const amounts = [
      ...(payload.amount_mode === 'fixed'
        ? [payload.fixed_amount]
        : [payload.total_amount, payload.min_amount, payload.max_amount]),
      payload.claim_paid_threshold,
    ];
    if (
      !payload.name?.trim() ||
      payload.group_id <= 0 ||
      payload.total_count <= 0
    ) {
      Toast.error(t('请填写活动名称、分组、总预算和总份数'));
      return;
    }
    if (
      amounts.some((amount) => amountInMinorUnits(amount) === null) ||
      payload.total_amount <= 0 ||
      (payload.amount_mode === 'fixed' && payload.fixed_amount <= 0) ||
      (payload.amount_mode === 'random' &&
        (payload.min_amount <= 0 || payload.max_amount <= 0)) ||
      payload.claim_paid_threshold < 0
    ) {
      Toast.error(t('金额最多只能保留两位小数且必须有效'));
      return;
    }
    if (
      payload.personal_valid_hours <= 0 ||
      payload.starts_at <= 0 ||
      payload.ends_at <= payload.starts_at
    ) {
      Toast.error(t('请填写有效的活动时间和个人券有效期'));
      return;
    }
    const randomRange = amountRange(
      payload.min_amount,
      payload.max_amount,
      payload.total_count,
    );
    if (
      payload.amount_mode === 'random' &&
      (!randomRange ||
        payload.total_amount < randomRange.minimum ||
        payload.total_amount > randomRange.maximum)
    ) {
      Toast.error(t('随机面额范围无法覆盖总预算'));
      return;
    }
    try {
      const response = editing
        ? await API.put(`/api/benefit/admin/activities/${editing.id}`, {
            ...payload,
            id: editing.id,
          })
        : await API.post('/api/benefit/admin/activities', payload);
      if (!response.data?.success) {
        Toast.error(response.data?.message || t('福利活动操作失败'));
        return;
      }
      Toast.success(t('操作成功'));
      closeEditor();
      await load();
    } catch (error) {
      Toast.error(error?.response?.data?.message || t('福利活动操作失败'));
    }
  };

  const runAction = async (activity, action, payload = {}) => {
    try {
      const response = await API.post(
        `/api/benefit/admin/activities/${activity.id}/${action}`,
        payload,
      );
      if (!response.data?.success) {
        Toast.error(response.data?.message || t('福利活动操作失败'));
        return;
      }
      Toast.success(t('操作成功'));
      await load();
    } catch (error) {
      Toast.error(error?.response?.data?.message || t('福利活动操作失败'));
    }
  };

  const deleteSelectedActivities = () => {
    const ids = selectedActivities
      .filter((activity) => ['ended', 'terminated'].includes(activity.status))
      .map((activity) => activity.id)
      .filter(Boolean);
    if (ids.length === 0) {
      Toast.error(t('请至少选择一个已结束或已终止的活动'));
      return;
    }
    Modal.confirm({
      title: t('确定删除所选历史活动？'),
      content: t(
        '仅已结束或已终止的活动可以删除，关联券和流水会保留。此操作不可撤销。',
      ),
      onOk: async () => {
        try {
          const response = await API.delete(
            '/api/benefit/admin/activities/batch',
            {
              data: { ids },
            },
          );
          if (!response.data?.success) {
            Toast.error(response.data?.message || t('批量删除失败'));
            return;
          }
          setSelectedActivities([]);
          const result = response.data?.data || {};
          Toast.success(
            t('已删除 {{deleted}} 个活动，跳过 {{skipped}} 个进行中活动', {
              deleted: result.deleted || 0,
              skipped: result.skipped || 0,
            }),
          );
          await load();
        } catch (error) {
          Toast.error(
            error?.response?.data?.message ||
              error.message ||
              t('批量删除失败'),
          );
        }
      },
    });
  };

  const loadDetail = async (activity, kind) => {
    try {
      const reportResponse =
        kind === 'report'
          ? API.get(`/api/benefit/admin/activities/${activity.id}/report`)
          : null;
      const voucherResponse = API.get(
        `/api/benefit/admin/activities/${activity.id}/vouchers`,
      );
      const [reportResult, voucherResult] = await Promise.all([
        reportResponse,
        voucherResponse,
      ]);
      setDetail({ activityId: activity.id, kind });
      if (kind === 'report') {
        setDetailData(reportResult?.data?.data || null);
        setReportVouchers(voucherResult.data?.data || []);
      } else {
        setDetailData(voucherResult.data?.data || null);
        setReportVouchers([]);
      }
      setLedger(null);
    } catch (error) {
      Toast.error(error?.response?.data?.message || t('福利活动操作失败'));
    }
  };

  const loadLedger = async (voucherId) => {
    try {
      const response = await API.get(
        `/api/benefit/admin/vouchers/${voucherId}/ledger`,
      );
      setLedger(response.data?.data || []);
    } catch (error) {
      Toast.error(error?.response?.data?.message || t('福利活动操作失败'));
    }
  };

  const voidVoucher = (voucher) => {
    Modal.confirm({
      title: t('确认作废福利券？'),
      content: t('作废后剩余额度将清零，且无法恢复。'),
      onOk: async () => {
        const reason = window.prompt(t('作废原因'), '')?.trim();
        if (!reason) return;
        const response = await API.post(
          `/api/benefit/admin/vouchers/${voucher.id}/void`,
          { confirm: true, reason },
        );
        if (!response.data?.success) {
          Toast.error(response.data?.message || t('福利活动操作失败'));
          return;
        }
        Toast.success(t('操作成功'));
        if (detail) await loadDetail({ id: detail.activityId }, 'vouchers');
      },
    });
  };

  const confirmTerminate = (activity) => {
    if (!terminateReason.trim()) {
      Toast.error(t('请输入终止原因'));
      return;
    }
    Modal.confirm({
      title: t('确认终止福利活动？'),
      content:
        terminateMode === 'all'
          ? t('这会作废所有已领取券的剩余额度。')
          : t('这会作废尚未领取的份额。'),
      onOk: () =>
        runAction(activity, 'terminate', {
          mode: terminateMode,
          reason: terminateReason,
          confirm: true,
        }),
    });
  };

  const columns = useMemo(
    () => [
      { title: t('ID'), dataIndex: 'id', width: 80 },
      {
        title: t('活动名称'),
        dataIndex: 'name',
        width: 180,
        render: (name, record) => (
          <div>
            <div className='font-semibold'>{name}</div>
            <Text type='tertiary' size='small'>
              {record.description || t('暂无说明')}
            </Text>
          </div>
        ),
      },
      {
        title: t('绑定分组'),
        dataIndex: 'group_name_snapshot',
        width: 160,
        render: (name, record) => (
          <div>
            <div>{name || `#${record.group_id}`}</div>
            <Text type='tertiary' size='small'>
              ID: {record.group_id}
            </Text>
          </div>
        ),
      },
      {
        title: t('总预算'),
        dataIndex: 'total_amount',
        width: 120,
        render: (value, record) =>
          record.total_quota
            ? formatQuota(record.total_quota)
            : formatAmount(
                amountFromActivity(value, record.total_amount_cents, 0),
              ),
      },
      { title: t('总份数'), dataIndex: 'total_count', width: 90 },
      {
        title: t('状态'),
        dataIndex: 'status',
        width: 110,
        render: (value) => (
          <Tag
            color={
              value === 'published'
                ? 'green'
                : value === 'terminated' || value === 'ended'
                  ? 'red'
                  : 'grey'
            }
          >
            {value}
          </Tag>
        ),
      },
      {
        title: t('结束时间'),
        dataIndex: 'ends_at',
        width: 170,
        render: (value) => formatBeijingDateTime(value),
      },
      {
        title: t('操作'),
        dataIndex: 'id',
        width: 110,
        render: (_, record) => (
          <Dropdown
            trigger='click'
            position='bottomRight'
            clickToHide
            render={
              <Dropdown.Menu>
                <Dropdown.Item disabled>{t('活动管理')}</Dropdown.Item>
                {record.status === 'draft' && (
                  <Dropdown.Item onClick={() => runAction(record, 'publish')}>
                    {t('发布活动')}
                  </Dropdown.Item>
                )}
                {record.status === 'published' && (
                  <Dropdown.Item onClick={() => runAction(record, 'pause')}>
                    {t('暂停活动')}
                  </Dropdown.Item>
                )}
                {record.status === 'paused' && (
                  <Dropdown.Item onClick={() => runAction(record, 'resume')}>
                    {t('恢复活动')}
                  </Dropdown.Item>
                )}
                {(record.status === 'published' ||
                  record.status === 'paused') && (
                  <Dropdown.Item onClick={() => runAction(record, 'end')}>
                    {t('提前结束')}
                  </Dropdown.Item>
                )}
                <Dropdown.Item
                  icon={<FilePenLine size={14} />}
                  onClick={() => openEdit(record)}
                >
                  {t('编辑活动')}
                </Dropdown.Item>
                {(record.status === 'published' ||
                  record.status === 'paused') && (
                  <Dropdown.Item
                    type='danger'
                    icon={<SquareX size={14} />}
                    onClick={() => confirmTerminate(record)}
                  >
                    {t('终止活动')}
                  </Dropdown.Item>
                )}
                <Dropdown.Divider />
                <Dropdown.Item disabled>{t('数据查看')}</Dropdown.Item>
                <Dropdown.Item
                  icon={<BarChart3 size={14} />}
                  onClick={() => loadDetail(record, 'report')}
                >
                  {t('查看报表')}
                </Dropdown.Item>
                <Dropdown.Item
                  icon={<Eye size={14} />}
                  onClick={() => loadDetail(record, 'vouchers')}
                >
                  {t('券列表')}
                </Dropdown.Item>
              </Dropdown.Menu>
            }
          >
            <Button
              theme='solid'
              type='primary'
              size='small'
              icon={<ChevronDown size={14} />}
              iconPosition='right'
              aria-label={t('操作')}
            >
              {t('操作')}
            </Button>
          </Dropdown>
        ),
      },
    ],
    [t, terminateMode, terminateReason],
  );

  const activitySelection = useMemo(
    () => ({
      selectedRowKeys: selectedActivities.map((activity) => activity.id),
      getCheckboxProps: (record) => ({
        disabled: !['ended', 'terminated'].includes(record.status),
      }),
      onChange: (keys, rows) => setSelectedActivities(rows),
    }),
    [selectedActivities],
  );

  if (loading) return <Spin spinning style={{ width: '100%', padding: 40 }} />;

  const detailActivity = detail
    ? activities.find((activity) => activity.id === detail.activityId)
    : null;

  return (
    <div className='grid gap-3'>
      <Card
        className='!rounded-xl border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] shadow-sm'
        bodyStyle={{ padding: 16 }}
        title={
          <div className='flex items-center justify-between gap-3'>
            <Space>
              <span className='font-semibold'>{t('时效额度券活动')}</span>
            </Space>
            <Space>
              <Button
                theme='borderless'
                icon={<RefreshCw size={14} />}
                onClick={load}
              >
                {t('刷新')}
              </Button>
              <Button
                theme='solid'
                type='primary'
                icon={<Plus size={14} />}
                onClick={openCreate}
              >
                {t('创建活动')}
              </Button>
              <Button
                theme='light'
                type='danger'
                icon={<Trash2 size={14} />}
                disabled={selectedActivities.length === 0}
                onClick={deleteSelectedActivities}
              >
                {t('删除历史活动')} ({selectedActivities.length})
              </Button>
            </Space>
          </div>
        }
      >
        <div className='mb-3 flex flex-wrap items-center gap-2 border-b border-[var(--semi-color-border)] pb-3'>
          <Input
            placeholder={t('终止原因（终止操作必填）')}
            value={terminateReason}
            onChange={setTerminateReason}
            style={{ maxWidth: 300 }}
          />
          <Select
            value={terminateMode}
            onChange={setTerminateMode}
            style={{ width: 160 }}
            optionList={[
              { label: t('作废未用券'), value: 'unused' },
              { label: t('作废所有券'), value: 'all' },
            ]}
          />
          <Text type='tertiary' size='small'>
            {t(
              '金额按元填写，最多两位小数；系统会自动换算为内部额度。时间为 Asia/Shanghai。',
            )}
          </Text>
        </div>
        <Table
          rowKey='id'
          columns={columns}
          dataSource={activities}
          rowSelection={activitySelection}
          pagination={false}
          scroll={{ x: 1380 }}
          empty={<Empty description={t('暂无时效额度券活动')} />}
        />
      </Card>
      {detail && (
        <Card
          className='!rounded-xl border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] shadow-sm'
          bodyStyle={{ padding: 16 }}
          title={
            <div className='flex items-center justify-between'>
              <Title heading={5} className='!mb-0'>
                {detail.kind === 'report' ? t('报表') : t('券列表')}
              </Title>
              <Space>
                {detail.kind === 'report' && (
                  <Button
                    theme='borderless'
                    icon={<RefreshCw size={14} />}
                    onClick={() =>
                      loadDetail(
                        detailActivity || { id: detail.activityId },
                        'report',
                      )
                    }
                  >
                    {t('刷新数据')}
                  </Button>
                )}
                <Button
                  theme='borderless'
                  onClick={() => {
                    setDetail(null);
                    setReportVouchers([]);
                    setLedger(null);
                  }}
                >
                  {t('关闭')}
                </Button>
              </Space>
            </div>
          }
        >
          {detail.kind === 'report' && detailData && (
            <BenefitActivityReportView
              activity={detailActivity}
              report={detailData}
              vouchers={reportVouchers}
              t={t}
            />
          )}
          {detail.kind === 'vouchers' && (
            <div className='grid gap-2 border-t pt-3 text-sm'>
              {(detailData || []).map((voucher) => (
                <div
                  key={voucher.id}
                  className='flex flex-wrap items-center justify-between border-b pb-2'
                >
                  <span>
                    #{voucher.id} · {t('剩余')} {voucher.remaining_quota}
                  </span>
                  <Space>
                    <Button
                      size='small'
                      theme='borderless'
                      onClick={() => loadLedger(voucher.id)}
                    >
                      {t('流水')}
                    </Button>
                    <Button
                      size='small'
                      type='danger'
                      theme='light'
                      disabled={voucher.status === 'voided'}
                      onClick={() => voidVoucher(voucher)}
                    >
                      {t('作废')}
                    </Button>
                  </Space>
                </div>
              ))}
              {ledger && (
                <div className='bg-fill-0 grid gap-1 rounded p-2'>
                  {ledger.map((entry) => (
                    <div
                      key={entry.id}
                      className='flex justify-between text-xs'
                    >
                      <span>{entry.type}</span>
                      <span>
                        {entry.quota_delta} · {entry.balance_after}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </Card>
      )}
      <SideSheet
        title={editing ? t('编辑时效额度券活动') : t('创建时效额度券活动')}
        visible={editorVisible}
        onCancel={closeEditor}
        width={620}
        footer={
          <div className='flex justify-end gap-2'>
            <Button onClick={closeEditor}>{t('取消')}</Button>
            <Button
              theme='solid'
              type='primary'
              onClick={() => formApiRef.current?.submitForm()}
            >
              {t('保存')}
            </Button>
          </div>
        }
      >
        <Form
          key={editorSessionKey}
          getFormApi={(api) => {
            formApiRef.current = api;
          }}
          initValues={
            editing
              ? toFormValues(editing)
              : {
                  ...defaultFormValues,
                  starts_at_text: formatBeijingDateTime(
                    Math.floor(Date.now() / 1000),
                  ),
                  ends_at_text: formatBeijingDateTime(
                    Math.floor(Date.now() / 1000) + 86400,
                  ),
                }
          }
          onSubmit={saveForm}
        >
          {({ values }) => (
            <>
              <Form.Input
                field='name'
                label={t('活动名称')}
                placeholder={t('例如：周末福利')}
                rules={[{ required: true, message: t('请输入活动名称') }]}
                extraText={t('展示给管理员和用户的活动名称。')}
              />
              <Form.Input
                field='description'
                label={t('活动说明')}
                placeholder={t('可选，说明活动规则')}
                extraText={t('可选说明，会显示在活动列表中。')}
              />
              <Form.Select
                field='group_id'
                label={t('绑定分组')}
                placeholder={t('请选择分组')}
                optionList={editorGroupOptions}
                loading={groupLoading}
                search
                style={{ width: '100%' }}
                rules={[{ required: true, message: t('请选择分组') }]}
                extraText={t('只有用户明确选择该分组时才会使用福利券。')}
              />
              <Form.Select
                field='amount_mode'
                label={t('面额模式')}
                style={{ width: '100%' }}
                optionList={[
                  { label: t('固定面额'), value: 'fixed' },
                  { label: t('随机面额'), value: 'random' },
                ]}
                extraText={t(
                  '固定模式每张金额相同；随机模式在最小和最大面额之间分配。',
                )}
              />
              {values.amount_mode === 'fixed' ? (
                <>
                  <Form.InputNumber
                    field='fixed_amount'
                    label={t('每份金额（元）')}
                    min={0.01}
                    step={0.01}
                    style={{ width: '100%' }}
                    extraText={t('每张券的金额，单位为元。')}
                  />
                  <Form.InputNumber
                    field='total_count'
                    label={t('总份数')}
                    min={1}
                    style={{ width: '100%' }}
                    extraText={t('要发放的券数量。')}
                  />
                  <Form.Slot label={t('总预算（元）')}>
                    <div className='bg-gray-50 rounded-md border px-3 py-2'>
                      <div className='font-semibold'>
                        {formatAmount(
                          fixedTotalAmount(
                            Number(values.fixed_amount || 0),
                            Number(values.total_count || 0),
                          ),
                        )}
                      </div>
                      <div className='text-xs text-gray-500'>
                        {t('每份金额 × 总份数，自动计算。')}
                      </div>
                    </div>
                  </Form.Slot>
                </>
              ) : (
                <>
                  <Form.InputNumber
                    field='total_amount'
                    label={t('总预算（元）')}
                    min={0.01}
                    step={0.01}
                    style={{ width: '100%' }}
                    extraText={t('活动全部券的基础金额，单位为元。')}
                  />
                  <Form.InputNumber
                    field='total_count'
                    label={t('总份数')}
                    min={1}
                    style={{ width: '100%' }}
                    extraText={t('要发放的券数量。')}
                  />
                  <Form.InputNumber
                    field='min_amount'
                    label={t('最低面额（元）')}
                    min={0.01}
                    step={0.01}
                    style={{ width: '100%' }}
                  />
                  <Form.InputNumber
                    field='max_amount'
                    label={t('最高面额（元）')}
                    min={0.01}
                    step={0.01}
                    style={{ width: '100%' }}
                  />
                  <Form.Slot label={t('可行总预算范围')}>
                    <div className='bg-gray-50 rounded-md border px-3 py-2'>
                      <div className='font-semibold'>
                        {(() => {
                          const range = amountRange(
                            Number(values.min_amount || 0),
                            Number(values.max_amount || 0),
                            Number(values.total_count || 0),
                          );
                          return range
                            ? `${formatAmount(range.minimum)} ~ ${formatAmount(range.maximum)}`
                            : '-';
                        })()}
                      </div>
                      <div className='text-xs text-gray-500'>
                        {t('总预算需落在此范围内。')}
                      </div>
                    </div>
                  </Form.Slot>
                </>
              )}
              <Form.InputNumber
                field='claim_paid_threshold'
                label={t('实付门槛（元）')}
                min={0}
                step={0.01}
                style={{ width: '100%' }}
                extraText={t(
                  '用户累计实付金额达到该值才能领取，单位为元；0 表示无门槛。',
                )}
              />
              <Form.InputNumber
                field='personal_valid_hours'
                label={t('个人券有效期（小时）')}
                min={1}
                step={1}
                style={{ width: '100%' }}
                extraText={t('用户领取后可使用的时长，单位为小时。')}
              />
              <Form.Input
                field='starts_at_text'
                label={t('活动开始时间（Asia/Shanghai）')}
                type='datetime-local'
                extraText={t('开始领取时间，北京时间。')}
              />
              <Form.Input
                field='ends_at_text'
                label={t('活动结束时间（Asia/Shanghai）')}
                type='datetime-local'
                extraText={t(
                  '停止领取时间；已领取的券按个人券有效期继续计算。',
                )}
              />
            </>
          )}
        </Form>
      </SideSheet>
    </div>
  );
}
