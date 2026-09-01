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
  personal_valid_seconds: 86400,
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

const toFormValues = (activity) => ({
  ...defaultFormValues,
  ...activity,
  total_amount: amountFromActivity(
    activity.total_amount,
    activity.total_amount_cents,
    defaultFormValues.total_amount,
  ),
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
  starts_at_text: formatBeijingDateTime(activity.starts_at),
  ends_at_text: formatBeijingDateTime(activity.ends_at),
});

const amountInMinorUnits = (value) => {
  const number = Number(value);
  if (!Number.isFinite(number) || number < 0) return null;
  const minor = Math.round(number * 100);
  return Math.abs(number * 100 - minor) < 1e-7 ? minor : null;
};

export default function BenefitActivitiesPanel() {
  const { t } = useTranslation();
  const [activities, setActivities] = useState([]);
  const [loading, setLoading] = useState(true);
  const [editorVisible, setEditorVisible] = useState(false);
  const [editorSessionKey, setEditorSessionKey] = useState(0);
  const [editing, setEditing] = useState(null);
  const [detail, setDetail] = useState(null);
  const [detailData, setDetailData] = useState(null);
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
      setGroupOptions(
        createGroupOptions(groups)
          .filter(
            (group) =>
              group.status === 1 &&
              Number.isInteger(Number(group.id)) &&
              group.id > 0,
          )
          .map((group) => ({
            ...group,
            value: group.id,
            label: [
              group.name || group.code,
              group.code && group.code !== group.name ? group.code : '',
              group.description,
            ]
              .filter(Boolean)
              .join(' | '),
          })),
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
    const payload = {
      name: values.name,
      description: values.description,
      group_id: Number(values.group_id || 0),
      amount_mode: values.amount_mode,
      total_amount: Number(values.total_amount || 0),
      total_count: Number(values.total_count || 0),
      fixed_amount: Number(values.fixed_amount || 0),
      min_amount: Number(values.min_amount || 0),
      max_amount: Number(values.max_amount || 0),
      claim_paid_threshold: Number(values.claim_paid_threshold || 0),
      personal_valid_seconds: Number(values.personal_valid_seconds || 0),
      starts_at: parseBeijingDateTime(values.starts_at_text),
      ends_at: parseBeijingDateTime(values.ends_at_text),
    };
    const amounts = [
      payload.total_amount,
      payload.fixed_amount,
      payload.min_amount,
      payload.max_amount,
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
      payload.amount_mode === 'fixed' &&
      amountInMinorUnits(payload.fixed_amount) * payload.total_count !==
        amountInMinorUnits(payload.total_amount)
    ) {
      Toast.error(t('固定面额乘份数必须等于总预算'));
      return;
    }
    if (
      payload.personal_valid_seconds <= 0 ||
      payload.starts_at <= 0 ||
      payload.ends_at <= payload.starts_at
    ) {
      Toast.error(t('请填写有效的活动时间和个人券有效期'));
      return;
    }
    if (
      payload.amount_mode === 'random' &&
      (payload.max_amount < payload.min_amount ||
        amountInMinorUnits(payload.total_amount) <
          payload.total_count * amountInMinorUnits(payload.min_amount) ||
        amountInMinorUnits(payload.total_amount) >
          payload.total_count * amountInMinorUnits(payload.max_amount))
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
          formatAmount(amountFromActivity(value, record.total_amount_cents, 0)),
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
        width: 330,
        render: (_, record) => (
          <Space wrap>
            {record.status === 'draft' && (
              <Button
                theme='solid'
                type='primary'
                size='small'
                onClick={() => runAction(record, 'publish')}
              >
                {t('发布')}
              </Button>
            )}
            {record.status === 'published' && (
              <Button
                theme='borderless'
                size='small'
                onClick={() => runAction(record, 'pause')}
              >
                {t('暂停')}
              </Button>
            )}
            {record.status === 'paused' && (
              <Button
                theme='borderless'
                size='small'
                onClick={() => runAction(record, 'resume')}
              >
                {t('恢复')}
              </Button>
            )}
            {(record.status === 'published' || record.status === 'paused') && (
              <Button
                theme='borderless'
                size='small'
                onClick={() => runAction(record, 'end')}
              >
                {t('提前结束')}
              </Button>
            )}
            <Button
              theme='borderless'
              size='small'
              icon={<FilePenLine size={14} />}
              onClick={() => openEdit(record)}
            >
              {t('编辑')}
            </Button>
            <Button
              theme='borderless'
              size='small'
              icon={<BarChart3 size={14} />}
              onClick={() => loadDetail(record, 'report')}
            >
              {t('报表')}
            </Button>
            <Button
              theme='borderless'
              size='small'
              icon={<Eye size={14} />}
              onClick={() => loadDetail(record, 'vouchers')}
            >
              {t('券列表')}
            </Button>
            {(record.status === 'published' || record.status === 'paused') && (
              <Button
                theme='light'
                type='danger'
                size='small'
                icon={<SquareX size={14} />}
                onClick={() => confirmTerminate(record)}
              >
                {t('终止')}
              </Button>
            )}
          </Space>
        ),
      },
    ],
    [t, terminateMode, terminateReason],
  );

  if (loading) return <Spin spinning style={{ width: '100%', padding: 40 }} />;

  return (
    <div className='grid gap-3'>
      <Card
        className='!rounded-xl shadow-sm'
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
          pagination={false}
          scroll={{ x: 1380 }}
          empty={<Empty description={t('暂无时效额度券活动')} />}
        />
      </Card>
      {detail && (
        <Card
          className='!rounded-xl shadow-sm'
          bodyStyle={{ padding: 16 }}
          title={
            <div className='flex items-center justify-between'>
              <Title heading={5} className='!mb-0'>
                {detail.kind === 'report' ? t('报表') : t('券列表')}
              </Title>
              <Button
                theme='borderless'
                onClick={() => {
                  setDetail(null);
                  setLedger(null);
                }}
              >
                {t('关闭')}
              </Button>
            </div>
          }
        >
          {detail.kind === 'report' && detailData && (
            <div className='grid gap-1 border-t pt-3 text-sm'>
              {Object.entries(detailData).map(([key, value]) => (
                <div key={key} className='flex justify-between border-b py-2'>
                  <span>{t(key)}</span>
                  <span>{String(value)}</span>
                </div>
              ))}
            </div>
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
              <Form.InputNumber
                field='total_amount'
                label={t('总预算（元）')}
                min={0.01}
                step={0.01}
                style={{ width: '100%' }}
                extraText={t(
                  '活动全部券的基础金额，单位为元，最多保留两位小数。',
                )}
              />
              <Form.InputNumber
                field='total_count'
                label={t('总份数')}
                min={1}
                style={{ width: '100%' }}
                extraText={t(
                  '要发放的券数量。固定模式下：固定面额 × 总份数 = 总预算。',
                )}
              />
              <Form.InputNumber
                field='fixed_amount'
                label={t('固定面额（元）')}
                min={0.01}
                step={0.01}
                style={{ width: '100%' }}
                extraText={t('固定模式下每张券的基础金额，单位为元。')}
              />
              {values.amount_mode === 'random' && (
                <>
                  <Form.InputNumber
                    field='min_amount'
                    label={t('最小面额（元）')}
                    min={0.01}
                    step={0.01}
                    style={{ width: '100%' }}
                  />
                  <Form.InputNumber
                    field='max_amount'
                    label={t('最大面额（元）')}
                    min={0.01}
                    step={0.01}
                    style={{ width: '100%' }}
                  />
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
                field='personal_valid_seconds'
                label={t('个人券有效期（秒）')}
                min={1}
                style={{ width: '100%' }}
                extraText={t('用户领取后可使用的时长，单位为秒。')}
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
