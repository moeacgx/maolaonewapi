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
} from 'lucide-react';
import {
  API,
  createGroupOptions,
  extractGroupDetailsResponse,
  getCurrencyConfig,
  renderQuota,
} from '../../../helpers';
import { quotaToDisplayAmount } from '../../../helpers/quota';
import { useTranslation } from 'react-i18next';
import {
  benefitActivityStatusColor,
  benefitActivityStatusLabel,
  formatDisplayAmount,
  isBenefitActivityDeletable,
} from '../../benefits/benefitLabels';
import BenefitActivityReport from './BenefitActivityReport';
import BenefitVoucherTable from './BenefitVoucherTable';
import BenefitActivityBatchActions from './BenefitActivityBatchActions';

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

// All amount fields in this form (total/fixed/min/max/threshold) are typed
// by the admin in the site's *current* quota_display_type unit, never a
// fixed currency: currency types allow at most 2 decimals, Tokens mode is
// integer-only. The server is told which display type the values were
// entered in (`amount_display_type`) and converts to internal quota itself.
const isValidDisplayAmount = (value, tokensMode) => {
  const number = Number(value);
  if (!Number.isFinite(number) || number < 0) return false;
  if (tokensMode) return Number.isInteger(number);
  const scaled = Math.round(number * 100);
  return Math.abs(number * 100 - scaled) < 1e-7;
};

const roundDisplayAmount = (value, tokensMode) => {
  const number = Number(value || 0);
  return tokensMode ? Math.round(number) : Math.round(number * 100) / 100;
};

const fixedTotalAmount = (amount, count, tokensMode) => {
  if (
    !isValidDisplayAmount(amount, tokensMode) ||
    !Number.isInteger(count) ||
    count <= 0
  ) {
    return 0;
  }
  return roundDisplayAmount(amount * count, tokensMode);
};

const amountRange = (minimum, maximum, count, tokensMode) => {
  if (
    !isValidDisplayAmount(minimum, tokensMode) ||
    !isValidDisplayAmount(maximum, tokensMode) ||
    !Number.isInteger(count) ||
    count <= 0
  ) {
    return null;
  }
  return {
    minimum: roundDisplayAmount(minimum * count, tokensMode),
    maximum: roundDisplayAmount(maximum * count, tokensMode),
  };
};

const amountFieldLabel = (t, baseLabelKey, currency) =>
  `${t(baseLabelKey)} (${currency.type === 'TOKENS' ? t('Tokens') : currency.symbol})`;

// fixed_quota/min_quota/max_quota are real, backend-authoritative fields on
// every activity (set at create/publish time and backfilled by migration),
// so editing just reads them straight through quotaToDisplayAmount() — no
// derivation from total_quota/total_count needed.
const toFormValues = (activity, currency) => {
  const isTokens = currency.type === 'TOKENS';
  return {
    ...defaultFormValues,
    ...activity,
    fixed_amount:
      activity.amount_mode === 'fixed'
        ? roundDisplayAmount(
            quotaToDisplayAmount(Number(activity.fixed_quota || 0)),
            isTokens,
          ) || defaultFormValues.fixed_amount
        : defaultFormValues.fixed_amount,
    min_amount:
      activity.amount_mode === 'random'
        ? roundDisplayAmount(
            quotaToDisplayAmount(Number(activity.min_quota || 0)),
            isTokens,
          ) || defaultFormValues.min_amount
        : defaultFormValues.min_amount,
    max_amount:
      activity.amount_mode === 'random'
        ? roundDisplayAmount(
            quotaToDisplayAmount(Number(activity.max_quota || 0)),
            isTokens,
          ) || defaultFormValues.max_amount
        : defaultFormValues.max_amount,
    claim_paid_threshold: Number(activity.claim_paid_threshold || 0),
    personal_valid_hours: Number(
      activity.personal_valid_hours ?? defaultFormValues.personal_valid_hours,
    ),
    total_amount: roundDisplayAmount(
      quotaToDisplayAmount(Number(activity.total_quota || 0)),
      isTokens,
    ),
    starts_at_text: formatBeijingDateTime(activity.starts_at),
    ends_at_text: formatBeijingDateTime(activity.ends_at),
  };
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
  const [terminateMode, setTerminateMode] = useState('unused');
  const [terminateReason, setTerminateReason] = useState('');
  const [groupOptions, setGroupOptions] = useState([]);
  const [groupLoading, setGroupLoading] = useState(false);
  const formApiRef = useRef(null);
  const currency = getCurrencyConfig();
  const isTokens = currency.type === 'TOKENS';
  const amountStep = isTokens ? 1 : 0.01;
  const amountPrecision = isTokens ? 0 : 2;

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const response = await API.get(
        '/api/benefit/admin/activities?p=1&page_size=100',
      );
      setActivities(response.data?.data?.items || []);
    } catch (error) {
      Toast.error(
        error?.response?.data?.message ||
          t('Failed to load benefit activities'),
      );
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
        throw new Error(response.data.message || t('Failed to load groups'));
      }
      const groups = extractGroupDetailsResponse(response?.data);
      if (groups === null) {
        throw new Error(response?.data?.message || t('Failed to load groups'));
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
      Toast.error(error?.message || t('Failed to load groups'));
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
        ? fixedTotalAmount(fixedAmount, totalCount, isTokens)
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
      amount_display_type: currency.type,
    };
    if (
      !payload.name?.trim() ||
      payload.group_id <= 0 ||
      payload.total_count <= 0
    ) {
      Toast.error(
        t(
          'Please fill in the activity name, group, total budget, and share count',
        ),
      );
      return;
    }
    const amounts = [
      ...(payload.amount_mode === 'fixed'
        ? [payload.fixed_amount]
        : [payload.total_amount, payload.min_amount, payload.max_amount]),
      payload.claim_paid_threshold,
    ];
    if (
      amounts.some((amount) => !isValidDisplayAmount(amount, isTokens)) ||
      payload.total_amount <= 0 ||
      (payload.amount_mode === 'fixed' && payload.fixed_amount <= 0) ||
      (payload.amount_mode === 'random' &&
        (payload.min_amount <= 0 || payload.max_amount <= 0))
    ) {
      Toast.error(
        isTokens
          ? t('Tokens amounts must be positive whole numbers')
          : t('Amounts must be valid and have at most 2 decimal places'),
      );
      return;
    }
    if (
      payload.personal_valid_hours <= 0 ||
      payload.starts_at <= 0 ||
      payload.ends_at <= payload.starts_at
    ) {
      Toast.error(
        t(
          'Please fill in a valid activity window and personal validity period',
        ),
      );
      return;
    }
    const randomRange = amountRange(
      payload.min_amount,
      payload.max_amount,
      payload.total_count,
      isTokens,
    );
    if (
      payload.amount_mode === 'random' &&
      (!randomRange ||
        payload.total_amount < randomRange.minimum ||
        payload.total_amount > randomRange.maximum)
    ) {
      Toast.error(t('The min/max amount range cannot cover the total budget'));
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
        Toast.error(
          response.data?.message || t('Benefit activity operation failed'),
        );
        return;
      }
      Toast.success(t('Saved'));
      closeEditor();
      await load();
    } catch (error) {
      Toast.error(
        error?.response?.data?.message ||
          t('Benefit activity operation failed'),
      );
    }
  };

  const runAction = async (activity, action, payload = {}) => {
    try {
      const response = await API.post(
        `/api/benefit/admin/activities/${activity.id}/${action}`,
        payload,
      );
      if (!response.data?.success) {
        Toast.error(
          response.data?.message || t('Benefit activity operation failed'),
        );
        return;
      }
      Toast.success(t('Saved'));
      await load();
    } catch (error) {
      Toast.error(
        error?.response?.data?.message ||
          t('Benefit activity operation failed'),
      );
    }
  };

  const loadDetail = async (activity, kind) => {
    if (kind === 'vouchers') {
      setDetail({ activityId: activity.id, kind });
      setDetailData(null);
      return;
    }
    try {
      const reportResponse = await API.get(
        `/api/benefit/admin/activities/${activity.id}/report`,
      );
      setDetail({ activityId: activity.id, kind });
      setDetailData(reportResponse?.data?.data || null);
    } catch (error) {
      Toast.error(
        error?.response?.data?.message ||
          t('Benefit activity operation failed'),
      );
    }
  };

  const confirmTerminate = (activity) => {
    if (!terminateReason.trim()) {
      Toast.error(t('Please enter a termination reason'));
      return;
    }
    Modal.confirm({
      title: t('Terminate this benefit activity?'),
      content:
        terminateMode === 'all'
          ? t('This voids the remaining balance of every claimed voucher.')
          : t('This voids shares that have not been claimed yet.'),
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
        title: t('Activity name'),
        dataIndex: 'name',
        width: 180,
        render: (name, record) => (
          <div>
            <div className='font-semibold'>{name}</div>
            <Text type='tertiary' size='small'>
              {record.description || t('No description')}
            </Text>
          </div>
        ),
      },
      {
        title: t('Bound group'),
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
        title: t('Total budget'),
        dataIndex: 'total_quota',
        width: 120,
        render: (value) => renderQuota(value),
      },
      { title: t('Shares'), dataIndex: 'total_count', width: 90 },
      {
        title: t('Status'),
        dataIndex: 'status',
        width: 110,
        render: (value) => (
          <Tag color={benefitActivityStatusColor(value)}>
            {benefitActivityStatusLabel(t, value)}
          </Tag>
        ),
      },
      {
        title: t('Ends at'),
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
                <Dropdown.Item disabled>
                  {t('Activity management')}
                </Dropdown.Item>
                {record.status === 'draft' && (
                  <Dropdown.Item onClick={() => runAction(record, 'publish')}>
                    {t('Publish')}
                  </Dropdown.Item>
                )}
                {record.status === 'published' && (
                  <Dropdown.Item onClick={() => runAction(record, 'pause')}>
                    {t('Pause')}
                  </Dropdown.Item>
                )}
                {record.status === 'paused' && (
                  <Dropdown.Item onClick={() => runAction(record, 'resume')}>
                    {t('Resume')}
                  </Dropdown.Item>
                )}
                {(record.status === 'published' ||
                  record.status === 'paused') && (
                  <Dropdown.Item onClick={() => runAction(record, 'end')}>
                    {t('End early')}
                  </Dropdown.Item>
                )}
                <Dropdown.Item
                  icon={<FilePenLine size={14} />}
                  onClick={() => openEdit(record)}
                >
                  {t('Edit')}
                </Dropdown.Item>
                {(record.status === 'published' ||
                  record.status === 'paused') && (
                  <Dropdown.Item
                    type='danger'
                    icon={<SquareX size={14} />}
                    onClick={() => confirmTerminate(record)}
                  >
                    {t('Terminate')}
                  </Dropdown.Item>
                )}
                <Dropdown.Divider />
                <Dropdown.Item disabled>{t('Data')}</Dropdown.Item>
                <Dropdown.Item
                  icon={<BarChart3 size={14} />}
                  onClick={() => loadDetail(record, 'report')}
                >
                  {t('View report')}
                </Dropdown.Item>
                <Dropdown.Item
                  icon={<Eye size={14} />}
                  onClick={() => loadDetail(record, 'vouchers')}
                >
                  {t('Vouchers')}
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [t, terminateMode, terminateReason],
  );

  const activitySelection = useMemo(
    () => ({
      selectedRowKeys: selectedActivities.map((activity) => activity.id),
      getCheckboxProps: (record) => ({
        disabled: !isBenefitActivityDeletable(record.status),
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
        className='!rounded-lg border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] shadow-sm'
        bodyStyle={{ padding: 16 }}
        title={
          <div className='flex items-center justify-between gap-3'>
            <Space>
              <span className='font-semibold'>
                {t('Time-limited voucher activities')}
              </span>
            </Space>
            <Space>
              <Button
                theme='borderless'
                icon={<RefreshCw size={14} />}
                onClick={load}
              >
                {t('Refresh')}
              </Button>
              <Button
                theme='solid'
                type='primary'
                icon={<Plus size={14} />}
                onClick={openCreate}
              >
                {t('Create activity')}
              </Button>
              <BenefitActivityBatchActions
                selectedIds={selectedActivities.map((activity) => activity.id)}
                onDeleted={() => {
                  setSelectedActivities([]);
                  load();
                }}
              />
            </Space>
          </div>
        }
      >
        <div className='mb-3 flex flex-wrap items-center gap-2 border-b border-[var(--semi-color-border)] pb-3'>
          <Input
            placeholder={t('Termination reason (required to terminate)')}
            value={terminateReason}
            onChange={setTerminateReason}
            style={{ maxWidth: 300 }}
          />
          <Select
            value={terminateMode}
            onChange={setTerminateMode}
            style={{ width: 160 }}
            optionList={[
              { label: t('Void unclaimed shares'), value: 'unused' },
              { label: t('Void all vouchers'), value: 'all' },
            ]}
          />
          <Text type='tertiary' size='small'>
            {t(
              'Amounts are entered in the site’s current display unit and converted to internal quota automatically. Times are Asia/Shanghai.',
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
          empty={<Empty description={t('No voucher activities yet')} />}
        />
      </Card>
      {detail && (
        <Card
          className='!rounded-lg border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] shadow-sm'
          bodyStyle={{ padding: 16 }}
          title={
            <div className='flex items-center justify-between'>
              <Title heading={5} className='!mb-0'>
                {detail.kind === 'report' ? t('Report') : t('Vouchers')}
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
                    {t('Refresh data')}
                  </Button>
                )}
                <Button theme='borderless' onClick={() => setDetail(null)}>
                  {t('Close')}
                </Button>
              </Space>
            </div>
          }
        >
          {detail.kind === 'report' && detailData && (
            <BenefitActivityReport
              activity={detailActivity}
              report={detailData}
            />
          )}
          {detail.kind === 'vouchers' && (
            <BenefitVoucherTable activityId={detail.activityId} />
          )}
        </Card>
      )}
      <SideSheet
        title={
          editing ? t('Edit voucher activity') : t('Create voucher activity')
        }
        visible={editorVisible}
        onCancel={closeEditor}
        width={620}
        footer={
          <div className='flex justify-end gap-2'>
            <Button onClick={closeEditor}>{t('Cancel')}</Button>
            <Button
              theme='solid'
              type='primary'
              onClick={() => formApiRef.current?.submitForm()}
            >
              {t('Save')}
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
              ? toFormValues(editing, currency)
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
                label={t('Activity name')}
                placeholder={t('e.g. Weekend benefit')}
                rules={[
                  {
                    required: true,
                    message: t('Please enter an activity name'),
                  },
                ]}
                extraText={t('Shown to admins and users.')}
              />
              <Form.Input
                field='description'
                label={t('Description')}
                placeholder={t('Optional; explains the activity rules')}
                extraText={t('Optional; shown in the activity list.')}
              />
              <Form.Select
                field='group_id'
                label={t('Bound group')}
                placeholder={t('Select a group')}
                optionList={editorGroupOptions}
                loading={groupLoading}
                search
                style={{ width: '100%' }}
                rules={[
                  { required: true, message: t('Please select a group') },
                ]}
                extraText={t(
                  'The benefit voucher is only used when the user explicitly selects this group.',
                )}
              />
              <Form.Select
                field='amount_mode'
                label={t('Amount mode')}
                style={{ width: '100%' }}
                optionList={[
                  { label: t('Fixed amount'), value: 'fixed' },
                  { label: t('Random amount'), value: 'random' },
                ]}
                extraText={t(
                  'Fixed mode: every voucher is worth the same amount. Random mode: each voucher is assigned an amount between the min and max.',
                )}
              />
              {values.amount_mode === 'fixed' ? (
                <>
                  <Form.InputNumber
                    field='fixed_amount'
                    label={amountFieldLabel(t, 'Amount per voucher', currency)}
                    min={isTokens ? 1 : 0.01}
                    step={amountStep}
                    precision={amountPrecision}
                    style={{ width: '100%' }}
                    extraText={t('The amount each voucher is worth.')}
                  />
                  <Form.InputNumber
                    field='total_count'
                    label={t('Share count')}
                    min={1}
                    step={1}
                    precision={0}
                    style={{ width: '100%' }}
                    extraText={t('How many vouchers to issue.')}
                  />
                  <Form.Slot
                    label={amountFieldLabel(t, 'Total budget', currency)}
                  >
                    <div className='rounded-md border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] px-3 py-2'>
                      <div className='font-semibold'>
                        {formatDisplayAmount(
                          t,
                          fixedTotalAmount(
                            Number(values.fixed_amount || 0),
                            Number(values.total_count || 0),
                            isTokens,
                          ),
                          currency,
                        )}
                      </div>
                      <div className='text-xs text-[var(--semi-color-text-2)]'>
                        {t(
                          'Amount per voucher × share count, calculated automatically.',
                        )}
                      </div>
                    </div>
                  </Form.Slot>
                </>
              ) : (
                <>
                  <Form.InputNumber
                    field='total_amount'
                    label={amountFieldLabel(t, 'Total budget', currency)}
                    min={isTokens ? 1 : 0.01}
                    step={amountStep}
                    precision={amountPrecision}
                    style={{ width: '100%' }}
                    extraText={t(
                      'The total amount every voucher in this activity shares.',
                    )}
                  />
                  <Form.InputNumber
                    field='total_count'
                    label={t('Share count')}
                    min={1}
                    step={1}
                    precision={0}
                    style={{ width: '100%' }}
                    extraText={t('How many vouchers to issue.')}
                  />
                  <Form.InputNumber
                    field='min_amount'
                    label={amountFieldLabel(t, 'Minimum amount', currency)}
                    min={isTokens ? 1 : 0.01}
                    step={amountStep}
                    precision={amountPrecision}
                    style={{ width: '100%' }}
                  />
                  <Form.InputNumber
                    field='max_amount'
                    label={amountFieldLabel(t, 'Maximum amount', currency)}
                    min={isTokens ? 1 : 0.01}
                    step={amountStep}
                    precision={amountPrecision}
                    style={{ width: '100%' }}
                  />
                  <Form.Slot label={t('Feasible total budget range')}>
                    <div className='rounded-md border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] px-3 py-2'>
                      <div className='font-semibold'>
                        {(() => {
                          const range = amountRange(
                            Number(values.min_amount || 0),
                            Number(values.max_amount || 0),
                            Number(values.total_count || 0),
                            isTokens,
                          );
                          return range
                            ? `${formatDisplayAmount(t, range.minimum, currency)} ~ ${formatDisplayAmount(t, range.maximum, currency)}`
                            : '-';
                        })()}
                      </div>
                      <div className='text-xs text-[var(--semi-color-text-2)]'>
                        {t('The total budget must fall within this range.')}
                      </div>
                    </div>
                  </Form.Slot>
                </>
              )}
              <Form.InputNumber
                field='claim_paid_threshold'
                label={amountFieldLabel(t, 'Claim threshold', currency)}
                min={0}
                step={amountStep}
                precision={amountPrecision}
                style={{ width: '100%' }}
                extraText={t(
                  'A user must have this much historical paid recharge before they can claim; 0 means no threshold.',
                )}
              />
              <Form.InputNumber
                field='personal_valid_hours'
                label={t('Personal validity (hours)')}
                min={1}
                step={1}
                style={{ width: '100%' }}
                extraText={t(
                  'How long a claimed voucher stays usable, in hours.',
                )}
              />
              <Form.Input
                field='starts_at_text'
                label={t('Activity start (Asia/Shanghai)')}
                type='datetime-local'
                extraText={t('When claiming opens, Beijing time.')}
              />
              <Form.Input
                field='ends_at_text'
                label={t('Activity end (Asia/Shanghai)')}
                type='datetime-local'
                extraText={t(
                  'When claiming closes; already-claimed vouchers keep their own personal validity.',
                )}
              />
            </>
          )}
        </Form>
      </SideSheet>
    </div>
  );
}
