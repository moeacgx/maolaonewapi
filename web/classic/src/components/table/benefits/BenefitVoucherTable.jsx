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

import React, { useEffect, useMemo, useState } from 'react';
import {
  Button,
  Dropdown,
  Empty,
  Input,
  Modal,
  Pagination,
  Select,
  Table,
  Tag,
  TextArea,
  Toast,
} from '@douyinfe/semi-ui';
import { Ban, ChevronDown, Search } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, renderQuota, timestamp2string } from '../../../helpers';
import {
  benefitVoucherStatusColor,
  benefitVoucherStatusLabel,
  benefitVoucherVoidSkipReasonLabel,
} from '../../benefits/benefitLabels';

const VOUCHER_PAGE_SIZE = 20;

const STATUS_OPTIONS = (t) => [
  { label: t('All statuses'), value: '' },
  { label: benefitVoucherStatusLabel(t, 'active'), value: 'active' },
  { label: benefitVoucherStatusLabel(t, 'exhausted'), value: 'exhausted' },
  { label: benefitVoucherStatusLabel(t, 'expired'), value: 'expired' },
  { label: benefitVoucherStatusLabel(t, 'voided'), value: 'voided' },
];

export default function BenefitVoucherTable({ activityId }) {
  const { t } = useTranslation();
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState('');
  const [keyword, setKeyword] = useState('');
  const [keywordInput, setKeywordInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [selectedIds, setSelectedIds] = useState([]);
  const [voidTargetIds, setVoidTargetIds] = useState(null);
  const [voidReason, setVoidReason] = useState('');
  const [voiding, setVoiding] = useState(false);

  const load = async (nextPage = page) => {
    setLoading(true);
    try {
      const params = new URLSearchParams({
        p: String(nextPage),
        page_size: String(VOUCHER_PAGE_SIZE),
      });
      if (status) params.set('status', status);
      if (keyword) params.set('keyword', keyword);
      const response = await API.get(
        `/api/benefit/admin/activities/${activityId}/vouchers?${params.toString()}`,
      );
      if (!response.data?.success) {
        Toast.error(response.data?.message || t('Failed to load vouchers'));
        return;
      }
      const data = response.data?.data || {};
      setItems(data.items || []);
      setTotal(data.total || 0);
      setPage(data.page || nextPage);
    } catch (error) {
      Toast.error(
        error?.response?.data?.message ||
          error?.message ||
          t('Failed to load vouchers'),
      );
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    setSelectedIds([]);
    load(1);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activityId, status, keyword]);

  const runSearch = () => setKeyword(keywordInput.trim());

  const openVoidModal = (ids) => {
    setVoidTargetIds(ids);
    setVoidReason('');
  };

  const closeVoidModal = () => {
    setVoidTargetIds(null);
    setVoidReason('');
  };

  const confirmVoid = async () => {
    const reason = voidReason.trim();
    if (!reason) {
      Toast.error(t('A reason is required'));
      return;
    }
    setVoiding(true);
    try {
      const isBatch = voidTargetIds.length > 1;
      const response = isBatch
        ? await API.post('/api/benefit/admin/vouchers/batch-void', {
            ids: voidTargetIds,
            reason,
            confirm: true,
          })
        : await API.post(
            `/api/benefit/admin/vouchers/${voidTargetIds[0]}/void`,
            { reason, confirm: true },
          );
      if (!response.data?.success) {
        Toast.error(response.data?.message || t('Void failed'));
        return;
      }
      if (isBatch) {
        const result = response.data?.data || {};
        const updated = result.updated_ids || [];
        const skipped = result.skipped || [];
        Toast.success(
          t('Voided {{count}} voucher(s)', { count: updated.length }),
        );
        if (skipped.length > 0) {
          Modal.info({
            title: t('Some vouchers were skipped'),
            content: (
              <ul className='grid gap-1 text-sm'>
                {skipped.map((entry) => (
                  <li key={entry.id}>
                    #{entry.id}:{' '}
                    {benefitVoucherVoidSkipReasonLabel(t, entry.reason)}
                  </li>
                ))}
              </ul>
            ),
          });
        }
      } else {
        Toast.success(t('Voucher voided'));
      }
      setSelectedIds([]);
      closeVoidModal();
      await load(page);
    } catch (error) {
      Toast.error(
        error?.response?.data?.message || error?.message || t('Void failed'),
      );
    } finally {
      setVoiding(false);
    }
  };

  const columns = useMemo(
    () => [
      { title: t('ID'), dataIndex: 'id', width: 70 },
      {
        title: t('Claimed by'),
        dataIndex: 'username',
        width: 140,
        render: (username, record) => username || `#${record.user_id}`,
      },
      {
        title: t('Original amount'),
        dataIndex: 'original_quota',
        width: 130,
        render: (value) => renderQuota(value),
      },
      {
        title: t('Used'),
        dataIndex: 'used_quota',
        width: 130,
        render: (value) => renderQuota(value),
      },
      {
        title: t('Remaining'),
        dataIndex: 'remaining_quota',
        width: 130,
        render: (value) => renderQuota(value),
      },
      {
        title: t('Status'),
        dataIndex: 'status',
        width: 110,
        render: (value) => (
          <Tag color={benefitVoucherStatusColor(value)}>
            {benefitVoucherStatusLabel(t, value)}
          </Tag>
        ),
      },
      {
        title: t('Claimed at'),
        dataIndex: 'claimed_at',
        width: 160,
        render: (value) => timestamp2string(value),
      },
      {
        title: t('Expires at'),
        dataIndex: 'expires_at',
        width: 160,
        render: (value) => timestamp2string(value),
      },
      {
        title: t('操作'),
        dataIndex: 'id',
        fixed: 'right',
        width: 90,
        render: (_, record) => (
          <Dropdown
            trigger='click'
            position='bottomRight'
            clickToHide
            render={
              <Dropdown.Menu>
                <Dropdown.Item
                  type='danger'
                  icon={<Ban size={14} />}
                  disabled={record.status !== 'active'}
                  onClick={() => openVoidModal([record.id])}
                >
                  {t('Void')}
                </Dropdown.Item>
              </Dropdown.Menu>
            }
          >
            <Button
              theme='borderless'
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
    [t],
  );

  const rowSelection = useMemo(
    () => ({
      selectedRowKeys: selectedIds,
      getCheckboxProps: (record) => ({
        disabled: record.status !== 'active',
      }),
      onChange: (keys) => setSelectedIds(keys || []),
    }),
    [selectedIds],
  );

  return (
    <div className='grid gap-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <Input
          value={keywordInput}
          onChange={setKeywordInput}
          placeholder={t('Search by username or user ID')}
          prefix={<Search size={14} />}
          showClear
          style={{ width: 220 }}
          onEnterPress={runSearch}
        />
        <Button onClick={runSearch}>{t('Search')}</Button>
        <Select
          value={status}
          onChange={setStatus}
          optionList={STATUS_OPTIONS(t)}
          style={{ width: 160 }}
        />
        <div className='flex-1' />
        {selectedIds.length > 0 && (
          <Button
            type='danger'
            theme='light'
            icon={<Ban size={14} />}
            onClick={() => openVoidModal(selectedIds)}
          >
            {t('Void selected')} ({selectedIds.length})
          </Button>
        )}
      </div>

      <Table
        rowKey='id'
        columns={columns}
        dataSource={items}
        loading={loading}
        rowSelection={rowSelection}
        pagination={false}
        scroll={{ x: '100%' }}
        empty={<Empty description={t('No vouchers yet')} />}
      />

      <div className='flex justify-end'>
        <Pagination
          currentPage={page}
          pageSize={VOUCHER_PAGE_SIZE}
          total={total}
          onPageChange={(nextPage) => load(nextPage)}
        />
      </div>

      <Modal
        title={t('Void voucher(s)')}
        visible={voidTargetIds != null}
        onCancel={closeVoidModal}
        onOk={confirmVoid}
        confirmLoading={voiding}
        okButtonProps={{ type: 'danger' }}
      >
        <p className='mb-2 text-sm text-[var(--semi-color-text-2)]'>
          {t(
            'The remaining balance of the selected voucher(s) will be cleared and cannot be restored.',
          )}
        </p>
        <TextArea
          value={voidReason}
          onChange={setVoidReason}
          placeholder={t('Reason (required)')}
          rows={2}
        />
      </Modal>
    </div>
  );
}
