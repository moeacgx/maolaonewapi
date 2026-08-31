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

import React, { useCallback, useEffect, useState } from 'react';
import {
  Button,
  Card,
  Empty,
  Input,
  InputNumber,
  Modal,
  Space,
  Spin,
  Tag,
  Toast,
} from '@douyinfe/semi-ui';
import { API } from '../../../helpers';
import { useTranslation } from 'react-i18next';

const formatBeijingDateTime = (timestamp) =>
  new Date(timestamp * 1000)
    .toLocaleString('sv-SE', { timeZone: 'Asia/Shanghai' })
    .slice(0, 16);

const parseBeijingDateTime = (value) => {
  const timestamp = Date.parse(`${value}:00+08:00`);
  return Number.isFinite(timestamp) ? Math.floor(timestamp / 1000) : 0;
};

export default function BenefitActivitiesPanel() {
  const { t } = useTranslation();
  const [activities, setActivities] = useState([]);
  const [loading, setLoading] = useState(true);
  const [reason, setReason] = useState('');
  const [terminateMode, setTerminateMode] = useState('unused');
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState(null);
  const [detail, setDetail] = useState(null);
  const [detailData, setDetailData] = useState(null);
  const [ledger, setLedger] = useState(null);
  const [form, setForm] = useState({
    name: '',
    description: '',
    group_id: 0,
    amount_mode: 'fixed',
    total_amount_cents: 1000,
    total_quota: 100000,
    total_count: 10,
    fixed_amount_cents: 100,
    min_amount_cents: 50,
    max_amount_cents: 200,
    claim_paid_threshold_cents: 0,
    personal_valid_seconds: 86400,
    starts_at: Math.floor(Date.now() / 1000),
    ends_at: Math.floor(Date.now() / 1000) + 86400,
  });

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

  const runAction = async (activity, action, payload = {}) => {
    try {
      const response = await API.post(
        `/api/benefit/admin/activities/${activity.id}/${action}`,
        payload,
      );
      if (!response.data?.success) {
        Toast.error(response.data?.message || t('操作失败'));
        return;
      }
      Toast.success(t('操作成功'));
      await load();
    } catch (error) {
      Toast.error(error?.response?.data?.message || t('操作失败'));
    }
  };

  const openCreate = () => {
    const now = Math.floor(Date.now() / 1000);
    setEditing(null);
    setForm({
      name: '',
      description: '',
      group_id: 0,
      amount_mode: 'fixed',
      total_amount_cents: 1000,
      total_quota: 100000,
      total_count: 10,
      fixed_amount_cents: 100,
      min_amount_cents: 50,
      max_amount_cents: 200,
      claim_paid_threshold_cents: 0,
      personal_valid_seconds: 86400,
      starts_at: now,
      ends_at: now + 86400,
    });
    setFormOpen(true);
  };

  const openEdit = (activity) => {
    setEditing(activity);
    setForm({ ...activity });
    setFormOpen(true);
  };

  const saveForm = async () => {
    if (
      !form.name.trim() ||
      Number(form.group_id) <= 0 ||
      Number(form.total_quota) <= 0
    ) {
      Toast.error(t('请填写活动名称、分组和总额度'));
      return;
    }
    if (
      form.amount_mode === 'fixed' &&
      Number(form.fixed_amount_cents) * Number(form.total_count) !==
        Number(form.total_amount_cents)
    ) {
      Toast.error(t('固定面额乘份数必须等于总预算'));
      return;
    }
    if (
      Number(form.personal_valid_seconds) <= 0 ||
      Number(form.starts_at) <= 0 ||
      Number(form.ends_at) <= Number(form.starts_at)
    ) {
      Toast.error(t('请填写有效的活动时间和个人券有效期'));
      return;
    }
    if (
      form.amount_mode === 'random' &&
      (Number(form.min_amount_cents) <= 0 ||
        Number(form.max_amount_cents) < Number(form.min_amount_cents) ||
        Number(form.total_amount_cents) <
          Number(form.total_count) * Number(form.min_amount_cents) ||
        Number(form.total_amount_cents) >
          Number(form.total_count) * Number(form.max_amount_cents))
    ) {
      Toast.error(t('随机面额范围无法覆盖总预算'));
      return;
    }
    try {
      const response = editing
        ? await API.put(`/api/benefit/admin/activities/${editing.id}`, {
            ...form,
            id: editing.id,
          })
        : await API.post('/api/benefit/admin/activities', form);
      if (!response.data?.success) {
        Toast.error(response.data?.message || t('操作失败'));
        return;
      }
      Toast.success(t('操作成功'));
      setFormOpen(false);
      await load();
    } catch (error) {
      Toast.error(error?.response?.data?.message || t('操作失败'));
    }
  };

  const loadDetail = async (activity, kind) => {
    try {
      const response =
        kind === 'report'
          ? await API.get(`/api/benefit/admin/activities/${activity.id}/report`)
          : await API.get(
              `/api/benefit/admin/activities/${activity.id}/vouchers`,
            );
      setDetail({ activityId: activity.id, kind });
      setDetailData(response.data?.data || null);
      setLedger(null);
    } catch (error) {
      Toast.error(error?.response?.data?.message || t('操作失败'));
    }
  };

  const loadLedger = async (voucherId) => {
    try {
      const response = await API.get(
        `/api/benefit/admin/vouchers/${voucherId}/ledger`,
      );
      setLedger(response.data?.data || []);
    } catch (error) {
      Toast.error(error?.response?.data?.message || t('操作失败'));
    }
  };

  const voidVoucher = (voucher) => {
    Modal.confirm({
      title: t('确认作废福利券？'),
      content: t('作废后剩余额度将清零，且无法恢复。'),
      onOk: async () => {
        const voucherReason = window.prompt(t('作废原因'), '')?.trim();
        if (!voucherReason) return;
        try {
          const response = await API.post(
            `/api/benefit/admin/vouchers/${voucher.id}/void`,
            { confirm: true, reason: voucherReason },
          );
          if (!response.data?.success) {
            Toast.error(response.data?.message || t('操作失败'));
            return;
          }
          Toast.success(t('操作成功'));
          if (detail) await loadDetail({ id: detail.activityId }, 'vouchers');
        } catch (error) {
          Toast.error(error?.response?.data?.message || t('操作失败'));
        }
      },
    });
  };

  if (loading) return <Spin spinning style={{ width: '100%', padding: 40 }} />;

  return (
    <div className='grid gap-3'>
      <div className='flex justify-end'>
        <Button theme='solid' type='primary' onClick={openCreate}>
          {t('创建活动')}
        </Button>
      </div>
      {formOpen && (
        <Card bodyStyle={{ padding: 16 }}>
          <div className='grid gap-3 md:grid-cols-2'>
            <Input
              placeholder={t('活动名称')}
              value={form.name}
              onChange={(value) =>
                setForm((current) => ({ ...current, name: value }))
              }
            />
            <Input
              placeholder={t('活动说明')}
              value={form.description}
              onChange={(value) =>
                setForm((current) => ({ ...current, description: value }))
              }
            />
            <InputNumber
              min={1}
              placeholder={t('绑定分组 ID')}
              value={form.group_id}
              onChange={(value) =>
                setForm((current) => ({ ...current, group_id: value || 0 }))
              }
            />
            <select
              className='semi-select'
              value={form.amount_mode}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  amount_mode: event.target.value,
                }))
              }
            >
              <option value='fixed'>{t('固定面额')}</option>
              <option value='random'>{t('随机面额')}</option>
            </select>
            {form.amount_mode === 'random' && (
              <>
                <InputNumber
                  min={1}
                  placeholder={t('最小面额（分）')}
                  value={form.min_amount_cents}
                  onChange={(value) =>
                    setForm((current) => ({
                      ...current,
                      min_amount_cents: value || 0,
                    }))
                  }
                />
                <InputNumber
                  min={1}
                  placeholder={t('最大面额（分）')}
                  value={form.max_amount_cents}
                  onChange={(value) =>
                    setForm((current) => ({
                      ...current,
                      max_amount_cents: value || 0,
                    }))
                  }
                />
              </>
            )}
            <InputNumber
              min={1}
              placeholder={t('总预算（分）')}
              value={form.total_amount_cents}
              onChange={(value) =>
                setForm((current) => ({
                  ...current,
                  total_amount_cents: value || 0,
                }))
              }
            />
            <InputNumber
              min={1}
              placeholder={t('总额度')}
              value={form.total_quota}
              onChange={(value) =>
                setForm((current) => ({ ...current, total_quota: value || 0 }))
              }
            />
            <InputNumber
              min={1}
              placeholder={t('总份数')}
              value={form.total_count}
              onChange={(value) =>
                setForm((current) => ({ ...current, total_count: value || 0 }))
              }
            />
            <InputNumber
              min={1}
              placeholder={t('固定面额（分）')}
              value={form.fixed_amount_cents}
              onChange={(value) =>
                setForm((current) => ({
                  ...current,
                  fixed_amount_cents: value || 0,
                }))
              }
            />
            <InputNumber
              min={0}
              placeholder={t('实付门槛（分）')}
              value={form.claim_paid_threshold_cents}
              onChange={(value) =>
                setForm((current) => ({
                  ...current,
                  claim_paid_threshold_cents: value || 0,
                }))
              }
            />
            <InputNumber
              min={1}
              placeholder={t('个人券有效期（秒）')}
              value={form.personal_valid_seconds}
              onChange={(value) =>
                setForm((current) => ({
                  ...current,
                  personal_valid_seconds: value || 0,
                }))
              }
            />
            <label className='grid gap-1 text-sm'>
              <span>{t('活动开始时间')}（Asia/Shanghai）</span>
              <input
                className='semi-input'
                type='datetime-local'
                value={formatBeijingDateTime(form.starts_at)}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    starts_at: parseBeijingDateTime(event.target.value),
                  }))
                }
              />
            </label>
            <label className='grid gap-1 text-sm'>
              <span>{t('活动结束时间')}（Asia/Shanghai）</span>
              <input
                className='semi-input'
                type='datetime-local'
                value={formatBeijingDateTime(form.ends_at)}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    ends_at: parseBeijingDateTime(event.target.value),
                  }))
                }
              />
            </label>
          </div>
          <Space className='mt-3'>
            <Button theme='solid' type='primary' onClick={saveForm}>
              {t('保存')}
            </Button>
            <Button onClick={() => setFormOpen(false)}>{t('取消')}</Button>
          </Space>
        </Card>
      )}
      {activities.length === 0 && (
        <Empty description={t('暂无时效额度券活动')} />
      )}
      {activities.map((activity) => (
        <Card key={activity.id} bodyStyle={{ padding: 16 }}>
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <div>
              <div className='font-semibold'>{activity.name}</div>
              <div className='text-secondary text-sm'>
                {activity.group_name_snapshot} ·{' '}
                {(activity.total_amount_cents / 100).toFixed(2)} ·{' '}
                {activity.total_count} {t('份')}
              </div>
            </div>
            <Space>
              <Tag>{activity.status}</Tag>
              {activity.status === 'draft' && (
                <Button onClick={() => runAction(activity, 'publish')}>
                  {t('发布')}
                </Button>
              )}
              {activity.status === 'published' && (
                <Button onClick={() => runAction(activity, 'pause')}>
                  {t('暂停')}
                </Button>
              )}
              {activity.status === 'paused' && (
                <Button onClick={() => runAction(activity, 'resume')}>
                  {t('恢复')}
                </Button>
              )}
              {(activity.status === 'published' ||
                activity.status === 'paused') && (
                <Button onClick={() => runAction(activity, 'end')}>
                  {t('提前结束')}
                </Button>
              )}
              <Button onClick={() => openEdit(activity)}>{t('编辑')}</Button>
              <Button onClick={() => loadDetail(activity, 'report')}>
                {t('报表')}
              </Button>
              <Button onClick={() => loadDetail(activity, 'vouchers')}>
                {t('券列表')}
              </Button>
              {(activity.status === 'published' ||
                activity.status === 'paused') && (
                <Button
                  type='danger'
                  disabled={!reason.trim()}
                  onClick={() =>
                    Modal.confirm({
                      title: t('确认终止福利活动？'),
                      content:
                        terminateMode === 'all'
                          ? t('这会作废所有已领取券的剩余额度。')
                          : t('这会作废尚未领取的份额。'),
                      onOk: () =>
                        runAction(activity, 'terminate', {
                          mode: terminateMode,
                          reason,
                          confirm: true,
                        }),
                    })
                  }
                >
                  {t('终止')}
                </Button>
              )}
            </Space>
          </div>
          {(activity.status === 'published' ||
            activity.status === 'paused') && (
            <div className='mt-3 flex flex-wrap gap-2'>
              <Input
                placeholder={t('终止原因')}
                value={reason}
                onChange={setReason}
              />
              <select
                className='semi-select'
                value={terminateMode}
                onChange={(event) => setTerminateMode(event.target.value)}
              >
                <option value='unused'>{t('作废未用券')}</option>
                <option value='all'>{t('作废所有券')}</option>
              </select>
            </div>
          )}
          {detail?.activityId === activity.id &&
            detail.kind === 'report' &&
            detailData && (
              <div className='mt-3 grid gap-1 border-t pt-3 text-sm'>
                {Object.entries(detailData).map(([key, value]) => (
                  <div key={key} className='flex justify-between'>
                    <span>{key}</span>
                    <span>{String(value)}</span>
                  </div>
                ))}
              </div>
            )}
          {detail?.activityId === activity.id && detail.kind === 'vouchers' && (
            <div className='mt-3 grid gap-2 border-t pt-3 text-sm'>
              {(detailData || []).map((voucher) => (
                <div
                  key={voucher.id}
                  className='flex flex-wrap items-center justify-between gap-2'
                >
                  <span>
                    #{voucher.id} · {t('剩余')} {voucher.remaining_quota}
                  </span>
                  <Space>
                    <Button size='small' onClick={() => loadLedger(voucher.id)}>
                      {t('流水')}
                    </Button>
                    <Button
                      size='small'
                      type='danger'
                      disabled={voucher.status === 'voided'}
                      onClick={() => voidVoucher(voucher)}
                    >
                      {t('作废')}
                    </Button>
                  </Space>
                </div>
              ))}
              {ledger && (
                <div className='bg-fill-0 grid gap-1 p-2'>
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
      ))}
    </div>
  );
}
