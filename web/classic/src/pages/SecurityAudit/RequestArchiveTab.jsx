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
import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Banner,
  Button,
  Card,
  Empty,
  Input,
  Modal,
  Select,
  Space,
  Spin,
  Switch,
  Tag,
  Toast,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Database,
  HardDrive,
  KeyRound,
  Plus,
  RefreshCw,
  Save,
  Stethoscope,
  Trash2,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { showError, timestamp2string } from '../../helpers/utils';
import {
  getRequestArchiveConfig,
  getRequestArchiveRuntime,
  probeRequestArchiveTarget,
  requestArchiveConfigToDraft,
  updateRequestArchiveConfig,
} from './api';

const { Text, Title } = Typography;

const getErrorMessage = (error, fallback) =>
  error?.response?.data?.message || error?.message || fallback;

const createTarget = () => {
  const suffix =
    typeof crypto !== 'undefined' && 'randomUUID' in crypto
      ? crypto.randomUUID().slice(0, 8)
      : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
  return {
    id: `archive-${suffix}`,
    name: '',
    type: 'local',
    enabled: true,
    local_path: '',
    endpoint: '',
    bucket: '',
    region: 'us-east-1',
    prefix: '',
    path_style: false,
    access_key_configured: false,
    secret_key_configured: false,
    created_at: 0,
    updated_at: 0,
    access_key_action: 'keep',
    access_key: '',
    secret_key_action: 'keep',
    secret_key: '',
  };
};

const getTargetTypeLabel = (type, t) =>
  type === 'local' ? t('本地存储') : t('S3 兼容对象存储');

const formatArchiveBytes = (bytes) => {
  if (typeof bytes !== 'number' || !Number.isFinite(bytes) || bytes < 0) {
    return '-';
  }
  if (bytes < 1024) return `${bytes} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let value = bytes / 1024;
  for (const unit of units) {
    if (value < 1024 || unit === units[units.length - 1]) {
      return `${value.toFixed(value >= 10 ? 1 : 2)} ${unit}`;
    }
    value /= 1024;
  }
  return `${bytes} B`;
};

const getSecretStatus = (configured, action, value, t) => {
  if (action === 'clear') return t('将被清除');
  if (action === 'replace' && value) return t('待替换');
  return configured ? t('已配置') : t('未配置');
};

const validateDraft = (draft, t) => {
  if (
    !Number.isInteger(draft.retention_days) ||
    draft.retention_days < 1 ||
    draft.retention_days > 3650
  ) {
    return t('请求归档保留天数必须在 1 到 3650 之间');
  }
  if (
    !Number.isInteger(draft.worker_count) ||
    draft.worker_count < 1 ||
    draft.worker_count > 32
  ) {
    return t('请求归档 Worker 数量必须在 1 到 32 之间');
  }
  if (
    !Number.isInteger(draft.queue_capacity) ||
    draft.queue_capacity < 1 ||
    draft.queue_capacity > 1048576
  ) {
    return t('请求归档队列容量必须在 1 到 1048576 之间');
  }
  if (
    !Number.isInteger(draft.max_body_bytes) ||
    draft.max_body_bytes < 1024 ||
    draft.max_body_bytes > 128 * 1024 * 1024
  ) {
    return t('单个完整请求最大归档大小必须在 1 KiB 到 128 MiB 之间');
  }
  if (
    !Number.isInteger(draft.queue_max_bytes) ||
    draft.queue_max_bytes < draft.max_body_bytes ||
    draft.queue_max_bytes > 64 * 1024 * 1024 * 1024
  ) {
    return t('归档队列正文上限必须不小于单请求上限且不超过 64 GiB');
  }
  if ((draft.targets || []).length > 64) {
    return t('请求归档最多支持 64 个存储目标');
  }

  const ids = new Set();
  for (const target of draft.targets || []) {
    const id = String(target.id || '').trim();
    if (!id || !String(target.name || '').trim()) {
      return t('每个请求归档存储目标都必须填写 ID 和名称');
    }
    if (ids.has(id)) return t('请求归档存储目标 ID 不能重复');
    ids.add(id);

    if (target.type === 'local') {
      if (!String(target.local_path || '').trim()) {
        return t('本地请求归档存储必须填写绝对目录路径');
      }
      continue;
    }

    if (!String(target.bucket || '').trim()) {
      return t('S3 兼容对象存储必须填写 Bucket');
    }
    if (
      target.access_key_action === 'replace' &&
      !String(target.access_key || '').trim()
    ) {
      return t('请填写新的访问密钥');
    }
    if (
      target.secret_key_action === 'replace' &&
      !String(target.secret_key || '').trim()
    ) {
      return t('请填写新的密钥');
    }
    if (
      target.enabled &&
      !target.access_key_configured &&
      target.access_key_action !== 'replace'
    ) {
      return t('新的 S3 兼容对象存储必须配置访问密钥');
    }
    if (
      target.enabled &&
      !target.secret_key_configured &&
      target.secret_key_action !== 'replace'
    ) {
      return t('新的 S3 兼容对象存储必须配置密钥');
    }
    const clearsAccessKey = target.access_key_action === 'clear';
    const clearsSecretKey = target.secret_key_action === 'clear';
    if (clearsAccessKey !== clearsSecretKey) {
      return t('S3 兼容对象存储的两项凭据必须同时清除');
    }
    if (clearsAccessKey && target.enabled) {
      return t('清除 S3 兼容对象存储凭据前必须先停用该目标');
    }
  }

  if (draft.enabled) {
    const activeTarget = (draft.targets || []).find(
      (target) => target.id === draft.active_target_id,
    );
    if (!activeTarget?.enabled) {
      return t('启用请求归档前必须选择一个已启用的活动存储目标');
    }
  }
  return '';
};

const RequestArchiveTab = () => {
  const { t } = useTranslation();
  const [config, setConfig] = useState(null);
  const [draft, setDraft] = useState(null);
  const [runtime, setRuntime] = useState(null);
  const [loading, setLoading] = useState(true);
  const [runtimeLoading, setRuntimeLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState('');
  const [runtimeError, setRuntimeError] = useState('');
  const [probingId, setProbingId] = useState('');

  const loadConfig = useCallback(async () => {
    setLoading(true);
    setLoadError('');
    try {
      const nextConfig = await getRequestArchiveConfig();
      setConfig(nextConfig);
      setDraft(requestArchiveConfigToDraft(nextConfig));
    } catch (error) {
      setLoadError(getErrorMessage(error, t('请求归档加载失败')));
    } finally {
      setLoading(false);
    }
  }, [t]);

  const loadRuntime = useCallback(
    async (showLoading = true) => {
      if (showLoading) setRuntimeLoading(true);
      setRuntimeError('');
      try {
        setRuntime(await getRequestArchiveRuntime());
      } catch (error) {
        setRuntimeError(getErrorMessage(error, t('请求归档运行状态加载失败')));
      } finally {
        if (showLoading) setRuntimeLoading(false);
      }
    },
    [t],
  );

  useEffect(() => {
    void loadConfig();
    void loadRuntime();
  }, [loadConfig, loadRuntime]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      void loadRuntime(false);
    }, 10000);
    return () => window.clearInterval(timer);
  }, [loadRuntime]);

  const baseline = useMemo(
    () => (config ? requestArchiveConfigToDraft(config) : null),
    [config],
  );
  const dirty = Boolean(
    draft && baseline && JSON.stringify(draft) !== JSON.stringify(baseline),
  );

  const updateDraft = (patch) => {
    setDraft((current) => (current ? { ...current, ...patch } : current));
  };
  const updateTarget = (id, patch) => {
    setDraft((current) =>
      current
        ? {
            ...current,
            targets: current.targets.map((target) =>
              target.id === id ? { ...target, ...patch } : target,
            ),
          }
        : current,
    );
  };

  const removeTarget = (id) => {
    Modal.confirm({
      title: t('移除请求归档存储目标'),
      content: t('该目标只会从待保存的配置中移除，已归档的对象不会被删除。'),
      okType: 'danger',
      onOk: () => {
        setDraft((current) => {
          if (!current) return current;
          return {
            ...current,
            active_target_id:
              current.active_target_id === id ? '' : current.active_target_id,
            targets: current.targets.filter((target) => target.id !== id),
          };
        });
      },
    });
  };

  const save = () => {
    if (!draft) return;
    const validationError = validateDraft(draft, t);
    if (validationError) {
      Toast.warning({ content: validationError });
      return;
    }
    setSaving(true);
    void updateRequestArchiveConfig(draft)
      .then((saved) => {
        setConfig(saved);
        setDraft(requestArchiveConfigToDraft(saved));
        Toast.success({ content: t('请求归档配置已保存') });
        void loadRuntime();
      })
      .catch(async (error) => {
        if (
          error?.response?.status === 409 &&
          error?.response?.data?.code === 'request_archive_config_conflict'
        ) {
          Toast.error({
            content: t('请求归档配置已被其他管理员更新，已重新加载最新版本'),
          });
          await loadConfig();
          return;
        }
        showError(getErrorMessage(error, t('保存失败')));
      })
      .finally(() => {
        setSaving(false);
      });
  };

  const probe = (target) => {
    if (!draft) return;
    const validationError = validateDraft(
      { ...draft, enabled: false, active_target_id: '', targets: [target] },
      t,
    );
    if (validationError) {
      Toast.warning({ content: validationError });
      return;
    }
    setProbingId(target.id);
    void probeRequestArchiveTarget(target)
      .then((result) => {
        if (result?.healthy) {
          Toast.success({
            content: t('请求归档存储在 {{latency}} 毫秒内响应', {
              latency: result.latency_ms || 0,
            }),
          });
        } else {
          Toast.error({ content: result?.message || t('请求归档存储不可用') });
        }
      })
      .catch((error) => {
        showError(getErrorMessage(error, t('请求归档存储探测失败')));
      })
      .finally(() => {
        window.setTimeout(() => setProbingId(''), 0);
      });
  };

  const setTargetType = (target, type) => {
    if (type === 'local') {
      updateTarget(target.id, {
        type,
        endpoint: '',
        bucket: '',
        region: '',
        prefix: '',
        path_style: false,
        access_key_action: 'keep',
        access_key: '',
        secret_key_action: 'keep',
        secret_key: '',
      });
      return;
    }
    updateTarget(target.id, {
      type,
      region: target.region || 'us-east-1',
      access_key_action: target.access_key_configured ? 'keep' : 'replace',
      secret_key_action: target.secret_key_configured ? 'keep' : 'replace',
    });
  };

  if (loading && !draft) {
    return (
      <div className='flex min-h-72 items-center justify-center'>
        <Spin spinning />
      </div>
    );
  }

  if (loadError || !draft) {
    return (
      <Banner
        type='danger'
        closeIcon={null}
        description={
          <Space wrap>
            <span>{loadError || t('请求归档加载失败')}</span>
            <Button
              size='small'
              loading={loading}
              onClick={() => void loadConfig()}
            >
              {t('重试')}
            </Button>
          </Space>
        }
      />
    );
  }

  const queue = runtime?.queue;

  return (
    <div className='space-y-4'>
      <div className='flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between'>
        <div>
          <Title heading={5} className='m-0'>
            {t('请求归档')}
          </Title>
          <Text type='tertiary' className='mt-1 block'>
            {t(
              '认证后归档 HTTP 原始请求正文及全部 Realtime 客户端帧的加密副本；外部存储写入在后台执行，归档失败不会拒绝客户端请求。',
            )}
          </Text>
        </div>
        <Space wrap>
          <Button
            icon={<RefreshCw size={15} />}
            loading={loading || runtimeLoading}
            onClick={() => {
              void loadConfig();
              void loadRuntime();
            }}
          >
            {t('刷新')}
          </Button>
          <Button
            type='primary'
            icon={<Save size={15} />}
            loading={saving}
            disabled={!dirty}
            onClick={save}
          >
            {t('保存更改')}
          </Button>
        </Space>
      </div>

      <Banner
        type='warning'
        closeIcon={null}
        description={
          <div>
            <Text strong>{t('隐私与容量提醒')}</Text>
            <Text type='tertiary' size='small' className='mt-1 block'>
              {t(
                'Realtime 归档包含文本 JSON、二进制 JSON 和原始二进制音频。这些数据可能含有敏感信息，音频也会快速占用队列与存储容量，请按合规要求设置保留期和访问权限。',
              )}
            </Text>
          </div>
        }
      />

      {runtimeError ? (
        <Banner type='warning' closeIcon={null} description={runtimeError} />
      ) : null}

      <Spin spinning={runtimeLoading}>
        <div className='grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4'>
          <Card title={t('归档 Worker')} bodyStyle={{ padding: 16 }}>
            <Space>
              <Text strong className='text-xl'>
                {runtime?.worker_active || 0} / {runtime?.worker_count || 0}
              </Text>
              <Tag color={runtime?.worker_running ? 'green' : 'grey'}>
                {runtime?.worker_running ? t('运行中') : t('已停止')}
              </Tag>
            </Space>
            <Text type='tertiary' size='small' className='mt-2 block'>
              {t('处理请求归档任务的后台 Worker')}
            </Text>
          </Card>
          <Card title={t('归档队列')} bodyStyle={{ padding: 16 }}>
            <Text strong className='text-xl'>
              {queue?.active || 0} / {queue?.capacity || 0}
            </Text>
            <Text type='tertiary' size='small' className='mt-2 block'>
              {t('归档队列正文总字节上限')}：
              {formatArchiveBytes(queue?.active_bytes)} /{' '}
              {formatArchiveBytes(queue?.capacity_bytes)}
            </Text>
            <Text type='tertiary' size='small' className='mt-2 block'>
              {t('已入队：{{enqueued}}，已丢弃：{{dropped}}', {
                enqueued: runtime?.enqueued || 0,
                dropped: runtime?.dropped || 0,
              })}
            </Text>
            <div className='mt-3 grid grid-cols-[minmax(0,1fr)_auto] gap-x-3 gap-y-1'>
              <Text type='tertiary' size='small'>
                {t('排队中')}
              </Text>
              <Text strong size='small'>
                {queue?.queued || 0}
              </Text>
              <Text type='tertiary' size='small'>
                {t('处理中')}
              </Text>
              <Text strong size='small'>
                {queue?.processing || 0}
              </Text>
              <Text type='tertiary' size='small'>
                {t('重试')}
              </Text>
              <Text strong size='small'>
                {queue?.retry || 0}
              </Text>
              <Text type='tertiary' size='small'>
                {t('失败')}
              </Text>
              <Text strong size='small'>
                {queue?.failed || 0}
              </Text>
              <Text type='tertiary' size='small'>
                {t('队列延迟')}
              </Text>
              <Text strong size='small'>
                {t('{{delay}} 毫秒', {
                  delay: runtime?.queue_delay_ms || 0,
                })}
              </Text>
            </div>
            <Text type='tertiary' size='small' className='mt-3 block'>
              {t(
                '失败任务会保留加密正文并继续占用队列容量，直到保留期清理完成。',
              )}
            </Text>
          </Card>
          <Card title={t('最近归档请求')} bodyStyle={{ padding: 16 }}>
            <Text strong>
              {runtime?.last_processed_at
                ? timestamp2string(runtime.last_processed_at)
                : '-'}
            </Text>
            <Text type='tertiary' size='small' className='mt-2 block'>
              {t('最近一次成功的后台归档')}
            </Text>
          </Card>
          <Card title={t('最近归档错误')} bodyStyle={{ padding: 16 }}>
            <Text strong className='break-all'>
              {runtime?.last_error_code || t('无')}
            </Text>
            <Text type='tertiary' size='small' className='mt-2 block'>
              {t('最近一次归档 Worker 错误代码')} · {t('入队状态')}：
              {runtime?.last_enqueue_code || t('无')}
            </Text>
          </Card>
        </div>
      </Spin>

      <Card title={t('归档策略')} bodyStyle={{ padding: 16 }}>
        <div className='space-y-4'>
          <div className='flex flex-col justify-between gap-3 sm:flex-row sm:items-center'>
            <div>
              <Text strong>{t('启用请求归档')}</Text>
              <Text type='tertiary' size='small' className='mt-1 block'>
                {t(
                  '将加密的 HTTP 请求正文和 Realtime 客户端帧异步写入选定的活动存储目标。',
                )}
              </Text>
            </div>
            <Switch
              checked={draft.enabled}
              onChange={(enabled) => updateDraft({ enabled })}
              aria-label={t('启用请求归档')}
            />
          </div>
          <div className='grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4'>
            <label className='space-y-1'>
              <Text type='tertiary' size='small'>
                {t('归档范围')}
              </Text>
              <Select
                value={draft.archive_scope || 'all_requests'}
                style={{ width: '100%' }}
                onChange={(archive_scope) => updateDraft({ archive_scope })}
              >
                <Select.Option value='all_requests'>
                  {t('所有符合条件的请求')}
                </Select.Option>
                <Select.Option value='audit_events'>
                  {t('仅归档产生审计事件的请求')}
                </Select.Option>
              </Select>
              <Text type='tertiary' size='small'>
                {t('仅审计事件模式会在事件成功写入后保存原始请求。')}
              </Text>
            </label>
            <label className='space-y-1'>
              <Text type='tertiary' size='small'>
                {t('活动存储目标')}
              </Text>
              <Select
                value={draft.active_target_id || undefined}
                placeholder={t('选择存储目标')}
                style={{ width: '100%' }}
                onChange={(active_target_id) =>
                  updateDraft({ active_target_id })
                }
              >
                {(draft.targets || []).map((target) => (
                  <Select.Option
                    key={target.id}
                    value={target.id}
                    disabled={!target.enabled}
                  >
                    {target.name || target.id}
                  </Select.Option>
                ))}
              </Select>
              <Text type='tertiary' size='small'>
                {t('只有已启用的目标会接收新入队的归档任务。')}
              </Text>
            </label>
            <label className='space-y-1'>
              <Text type='tertiary' size='small'>
                {t('保留天数')}
              </Text>
              <Input
                type='number'
                min={1}
                max={3650}
                step={1}
                value={String(draft.retention_days)}
                onChange={(retention_days) =>
                  updateDraft({ retention_days: Number(retention_days) })
                }
              />
            </label>
            <label className='space-y-1'>
              <Text type='tertiary' size='small'>
                {t('Worker 数量')}
              </Text>
              <Input
                type='number'
                min={1}
                max={32}
                step={1}
                value={String(draft.worker_count)}
                onChange={(worker_count) =>
                  updateDraft({ worker_count: Number(worker_count) })
                }
              />
            </label>
            <label className='space-y-1'>
              <Text type='tertiary' size='small'>
                {t('队列容量')}
              </Text>
              <Input
                type='number'
                min={1}
                max={1048576}
                step={1}
                value={String(draft.queue_capacity)}
                onChange={(queue_capacity) =>
                  updateDraft({ queue_capacity: Number(queue_capacity) })
                }
              />
            </label>
            <label className='space-y-1'>
              <Text type='tertiary' size='small'>
                {t('单个完整请求最大归档字节数')}
              </Text>
              <Input
                type='number'
                min={1024}
                max={128 * 1024 * 1024}
                step={1}
                value={String(draft.max_body_bytes)}
                onChange={(max_body_bytes) =>
                  updateDraft({ max_body_bytes: Number(max_body_bytes) })
                }
              />
              <Text type='tertiary' size='small'>
                {t(
                  '每个归档任务可保留的 HTTP 请求正文或 Realtime 客户端帧最大字节数。',
                )}
              </Text>
            </label>
            <label className='space-y-1'>
              <Text type='tertiary' size='small'>
                {t('归档队列正文总字节上限')}
              </Text>
              <Input
                type='number'
                min={draft.max_body_bytes}
                max={64 * 1024 * 1024 * 1024}
                step={1}
                value={String(draft.queue_max_bytes)}
                onChange={(queue_max_bytes) =>
                  updateDraft({ queue_max_bytes: Number(queue_max_bytes) })
                }
              />
              <Text type='tertiary' size='small'>
                {t(
                  '等待落盘的 HTTP 请求正文和 Realtime 客户端帧总字节数上限。',
                )}
              </Text>
            </label>
          </div>
        </div>
      </Card>

      <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
        <div>
          <Title heading={5} className='m-0'>
            {t('归档存储目标')}
          </Title>
          <Text type='tertiary' size='small' className='mt-1 block'>
            {t(
              '可配置本地磁盘、S3 兼容对象存储或 Cloudflare R2；凭据只可写入，绝不回显。',
            )}
          </Text>
        </div>
        <Button
          type='primary'
          icon={<Plus size={15} />}
          onClick={() =>
            updateDraft({ targets: [...draft.targets, createTarget()] })
          }
        >
          {t('添加存储目标')}
        </Button>
      </div>

      {(draft.targets || []).length === 0 ? (
        <Card bodyStyle={{ padding: 24 }}>
          <Empty
            image={<Database size={42} color='var(--semi-color-text-2)' />}
            description={t('尚未配置请求归档存储目标')}
          >
            <Button
              type='primary'
              icon={<Plus size={15} />}
              onClick={() =>
                updateDraft({ targets: [...draft.targets, createTarget()] })
              }
            >
              {t('添加存储目标')}
            </Button>
          </Empty>
        </Card>
      ) : (
        <div className='grid grid-cols-1 gap-4 xl:grid-cols-2'>
          {draft.targets.map((target) => (
            <Card
              key={target.id}
              title={
                <Space wrap className='min-w-0'>
                  <HardDrive size={16} />
                  <span className='min-w-0 break-words'>
                    {target.name || t('未命名存储目标')}
                  </span>
                  <Tag color={target.enabled ? 'green' : 'grey'}>
                    {target.enabled ? t('已启用') : t('已停用')}
                  </Tag>
                  <Tag>{getTargetTypeLabel(target.type, t)}</Tag>
                  {target.id === draft.active_target_id ? (
                    <Tag color='blue'>{t('活动')}</Tag>
                  ) : null}
                </Space>
              }
              headerExtraContent={
                <Button
                  theme='borderless'
                  type='danger'
                  icon={<Trash2 size={16} />}
                  aria-label={t('删除存储目标')}
                  onClick={() => removeTarget(target.id)}
                />
              }
              bodyStyle={{ padding: 16 }}
            >
              <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
                <label className='space-y-1'>
                  <Text type='tertiary' size='small'>
                    {t('名称')}
                  </Text>
                  <Input
                    value={target.name}
                    placeholder={t('归档主存储')}
                    onChange={(name) => updateTarget(target.id, { name })}
                  />
                </label>
                <label className='space-y-1'>
                  <Text type='tertiary' size='small'>
                    {t('存储类型')}
                  </Text>
                  <Select
                    value={target.type}
                    style={{ width: '100%' }}
                    onChange={(type) => setTargetType(target, type)}
                  >
                    <Select.Option value='local'>{t('本地存储')}</Select.Option>
                    <Select.Option value='s3'>
                      {t('S3 兼容对象存储 / Cloudflare R2')}
                    </Select.Option>
                  </Select>
                </label>
                <div className='flex items-end pb-1'>
                  <Space>
                    <Switch
                      checked={target.enabled}
                      aria-label={t('启用目标')}
                      onChange={(enabled) =>
                        updateTarget(target.id, { enabled })
                      }
                    />
                    <Text>{t('启用目标')}</Text>
                  </Space>
                </div>

                {target.type === 'local' ? (
                  <label className='space-y-1 lg:col-span-2'>
                    <Text type='tertiary' size='small'>
                      {t('本地归档目录')}
                    </Text>
                    <Input
                      value={target.local_path || ''}
                      placeholder={t('D:\\newapi-archive')}
                      onChange={(local_path) =>
                        updateTarget(target.id, { local_path })
                      }
                    />
                    <Text type='tertiary' size='small'>
                      {t('使用运行 New API 服务端的绝对目录路径。')}
                    </Text>
                  </label>
                ) : (
                  <>
                    <label className='space-y-1 lg:col-span-2'>
                      <Text type='tertiary' size='small'>
                        {t('S3 Endpoint')}
                      </Text>
                      <Input
                        value={target.endpoint || ''}
                        placeholder='https://<account-id>.r2.cloudflarestorage.com'
                        onChange={(endpoint) =>
                          updateTarget(target.id, { endpoint })
                        }
                      />
                      <Text type='tertiary' size='small'>
                        {t(
                          'AWS 可留空；Cloudflare R2 或其他 S3 兼容服务请填写对应 Endpoint。',
                        )}
                      </Text>
                    </label>
                    <label className='space-y-1'>
                      <Text type='tertiary' size='small'>
                        {t('Bucket')}
                      </Text>
                      <Input
                        value={target.bucket || ''}
                        onChange={(bucket) =>
                          updateTarget(target.id, { bucket })
                        }
                      />
                    </label>
                    <label className='space-y-1'>
                      <Text type='tertiary' size='small'>
                        {t('区域')}
                      </Text>
                      <Input
                        value={target.region || ''}
                        placeholder={t('us-east-1')}
                        onChange={(region) =>
                          updateTarget(target.id, { region })
                        }
                      />
                    </label>
                    <label className='space-y-1'>
                      <Text type='tertiary' size='small'>
                        {t('对象前缀')}
                      </Text>
                      <Input
                        value={target.prefix || ''}
                        placeholder={t('request-archive')}
                        onChange={(prefix) =>
                          updateTarget(target.id, { prefix })
                        }
                      />
                    </label>
                    <div className='flex items-end pb-1'>
                      <Space>
                        <Switch
                          checked={target.path_style}
                          aria-label={t('使用路径式 S3 地址')}
                          onChange={(path_style) =>
                            updateTarget(target.id, { path_style })
                          }
                        />
                        <Text>{t('使用路径式 S3 地址')}</Text>
                      </Space>
                    </div>
                    <label className='space-y-1'>
                      <Text type='tertiary' size='small'>
                        {t('访问密钥操作')}
                      </Text>
                      <Select
                        value={target.access_key_action}
                        style={{ width: '100%' }}
                        onChange={(access_key_action) =>
                          updateTarget(target.id, {
                            access_key_action,
                            access_key: '',
                          })
                        }
                      >
                        <Select.Option
                          value='keep'
                          disabled={!target.access_key_configured}
                        >
                          {t('保留')}
                        </Select.Option>
                        <Select.Option value='replace'>
                          {t('替换')}
                        </Select.Option>
                        <Select.Option value='clear'>{t('清除')}</Select.Option>
                      </Select>
                      <Text type='tertiary' size='small'>
                        {getSecretStatus(
                          target.access_key_configured,
                          target.access_key_action,
                          target.access_key,
                          t,
                        )}
                      </Text>
                    </label>
                    <label className='space-y-1'>
                      <Text type='tertiary' size='small'>
                        {t('密钥操作')}
                      </Text>
                      <Select
                        value={target.secret_key_action}
                        style={{ width: '100%' }}
                        onChange={(secret_key_action) =>
                          updateTarget(target.id, {
                            secret_key_action,
                            secret_key: '',
                          })
                        }
                      >
                        <Select.Option
                          value='keep'
                          disabled={!target.secret_key_configured}
                        >
                          {t('保留')}
                        </Select.Option>
                        <Select.Option value='replace'>
                          {t('替换')}
                        </Select.Option>
                        <Select.Option value='clear'>{t('清除')}</Select.Option>
                      </Select>
                      <Text type='tertiary' size='small'>
                        {getSecretStatus(
                          target.secret_key_configured,
                          target.secret_key_action,
                          target.secret_key,
                          t,
                        )}
                      </Text>
                    </label>
                    {target.access_key_action === 'replace' ? (
                      <label className='space-y-1 lg:col-span-2'>
                        <Text type='tertiary' size='small'>
                          {t('新的访问密钥')}
                        </Text>
                        <Input
                          mode='password'
                          value={target.access_key}
                          prefix={<KeyRound size={15} />}
                          onChange={(access_key) =>
                            updateTarget(target.id, { access_key })
                          }
                        />
                      </label>
                    ) : null}
                    {target.secret_key_action === 'replace' ? (
                      <label className='space-y-1 lg:col-span-2'>
                        <Text type='tertiary' size='small'>
                          {t('新的密钥')}
                        </Text>
                        <Input
                          mode='password'
                          value={target.secret_key}
                          prefix={<KeyRound size={15} />}
                          onChange={(secret_key) =>
                            updateTarget(target.id, { secret_key })
                          }
                        />
                      </label>
                    ) : null}
                  </>
                )}
              </div>
              <div className='mt-4 flex justify-end border-t border-[var(--semi-color-border)] pt-4'>
                <Button
                  icon={<Stethoscope size={15} />}
                  loading={probingId === target.id}
                  onClick={() => probe(target)}
                >
                  {t('连通性探测')}
                </Button>
              </div>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
};

export default RequestArchiveTab;
