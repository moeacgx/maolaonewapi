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
  Banner,
  Button,
  Empty,
  Form,
  Popconfirm,
  Progress,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Activity,
  AlertTriangle,
  Gauge,
  RefreshCw,
  ServerCog,
  Trash2,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  API,
  showError,
  showSuccess,
  timestamp2string,
} from '../../../helpers';
import {
  SYSTEM_INSTANCE_POLL_INTERVAL_MS,
  formatBytes,
  formatMetricValue,
  formatPercent,
  getInstanceActiveRequests,
  getInstanceDisplayName,
  getInstanceHostname,
  getInstanceRpm,
  getInstanceRoleDescription,
  getInstanceRoleLabel,
  getInstanceRuntimeLabel,
  getInstanceStatusLabel,
  getInstanceStatusTagColor,
  getResourceColor,
  getSystemInstancesFromResponse,
  isStaleInstance,
  normalizePercent,
  shouldConfigureNodeName,
  summarizeInstanceTraffic,
} from './systemInstances';

const { Text } = Typography;

function formatTimestamp(timestamp) {
  if (typeof timestamp !== 'number' || timestamp <= 0) return '-';
  return timestamp2string(timestamp);
}

function ResourceUsage({ value }) {
  const percent = normalizePercent(value);

  return (
    <div style={{ minWidth: 104 }}>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          marginBottom: 4,
        }}
      >
        <Text size='small' strong>
          {formatPercent(value)}
        </Text>
      </div>
      <Progress
        percent={percent ?? 0}
        showInfo={false}
        size='small'
        stroke={getResourceColor(percent)}
      />
    </div>
  );
}

function StorageCell({ storage, t }) {
  const content = <ResourceUsage value={storage?.used_percent} />;
  if (!storage) return content;

  return (
    <Tooltip
      content={
        <div style={{ minWidth: 180 }}>
          <div>
            {t('Used')}: {formatBytes(storage.used_bytes)}
          </div>
          <div>
            {t('Free')}: {formatBytes(storage.free_bytes)}
          </div>
          <div>
            {t('Total')}: {formatBytes(storage.total_bytes)}
          </div>
        </div>
      }
    >
      {content}
    </Tooltip>
  );
}

function NodeNameCell({ instance, t }) {
  const shouldConfigure = shouldConfigureNodeName(instance);

  return (
    <Space spacing={8} align='start' style={{ maxWidth: 260 }}>
      <span
        style={{
          width: 8,
          height: 8,
          borderRadius: 999,
          flex: '0 0 auto',
          marginTop: 7,
          background:
            instance.status === 'online'
              ? 'var(--semi-color-success)'
              : 'var(--semi-color-warning)',
        }}
      />
      <div style={{ minWidth: 0 }}>
        <Space spacing={6} align='center'>
          <Text
            strong
            ellipsis={{ showTooltip: true }}
            style={{ maxWidth: 170 }}
          >
            {getInstanceDisplayName(instance)}
          </Text>
          {shouldConfigure && (
            <Tooltip
              content={t(
                'This instance is using an automatic hostname. Set NODE_NAME to a stable unique value for multi-instance management.',
              )}
            >
              <Tag color='orange' prefixIcon={<AlertTriangle size={12} />}>
                {t('Configure NODE_NAME')}
              </Tag>
            </Tooltip>
          )}
        </Space>
        <Text
          type='tertiary'
          size='small'
          ellipsis={{ showTooltip: true }}
          style={{ display: 'block', maxWidth: 220, fontFamily: 'monospace' }}
        >
          {getInstanceHostname(instance)}
        </Text>
      </div>
    </Space>
  );
}

export default function SystemInstancesPanel() {
  const { t } = useTranslation();
  const mountedRef = useRef(false);
  const [instances, setInstances] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [deletingNodeName, setDeletingNodeName] = useState('');
  const [deletingAll, setDeletingAll] = useState(false);

  const loadInstances = useCallback(
    async (showLoading = false) => {
      if (showLoading) setLoading(true);
      try {
        const response = await API.get('/api/system-info/instances');
        const success = response?.data?.success;
        const nextInstances = getSystemInstancesFromResponse(response);
        if (!success || !Array.isArray(response?.data?.data)) {
          throw new Error(
            response?.data?.message || t('We could not load instances.'),
          );
        }
        if (!mountedRef.current) return;
        setInstances(nextInstances);
        setError('');
      } catch (err) {
        if (!mountedRef.current) return;
        const message =
          err?.response?.data?.message ||
          err?.message ||
          t('We could not load instances.');
        setError(message);
        if (showLoading) showError(message);
      } finally {
        if (mountedRef.current && showLoading) setLoading(false);
      }
    },
    [t],
  );

  useEffect(() => {
    mountedRef.current = true;
    loadInstances(true);
    const timer = window.setInterval(() => {
      loadInstances(false);
    }, SYSTEM_INSTANCE_POLL_INTERVAL_MS);

    return () => {
      mountedRef.current = false;
      window.clearInterval(timer);
    };
  }, [loadInstances]);

  const staleInstances = useMemo(
    () => instances.filter((instance) => isStaleInstance(instance)),
    [instances],
  );

  const trafficSummary = useMemo(
    () => summarizeInstanceTraffic(instances),
    [instances],
  );
  const hasCompleteTrafficSummary =
    trafficSummary.onlineInstances === trafficSummary.instancesWithMetrics;

  async function deleteStaleInstance(instance) {
    if (!isStaleInstance(instance)) return;
    setDeletingNodeName(instance.node_name);
    try {
      const response = await API.delete(
        `/api/system-info/instances/${encodeURIComponent(instance.node_name)}`,
      );
      if (!response?.data?.success) {
        showError(response?.data?.message || t('Delete failed'));
        return;
      }
      showSuccess(t('Deleted stale instance'));
      await loadInstances(false);
    } catch (err) {
      showError(
        err?.response?.data?.message || err?.message || t('Delete failed'),
      );
    } finally {
      setDeletingNodeName('');
    }
  }

  async function deleteAllStaleInstances() {
    if (staleInstances.length === 0) return;
    setDeletingAll(true);
    try {
      const response = await API.delete('/api/system-info/stale-instances');
      if (!response?.data?.success) {
        showError(response?.data?.message || t('Delete failed'));
        return;
      }
      showSuccess(
        t('Deleted {{count}} stale instances', {
          count: response?.data?.data?.deleted_count ?? 0,
        }),
      );
      await loadInstances(false);
    } catch (err) {
      showError(
        err?.response?.data?.message || err?.message || t('Delete failed'),
      );
    } finally {
      setDeletingAll(false);
    }
  }

  const columns = useMemo(
    () => [
      {
        title: t('Instances'),
        dataIndex: 'node_name',
        width: 280,
        render: (_, record) => <NodeNameCell instance={record} t={t} />,
      },
      {
        title: t('Status'),
        dataIndex: 'status',
        width: 90,
        render: (status) => (
          <Tag color={getInstanceStatusTagColor(status)}>
            {t(getInstanceStatusLabel(status))}
          </Tag>
        ),
      },
      {
        title: t('Role'),
        dataIndex: 'role',
        width: 100,
        render: (_, record) => (
          <Tooltip content={t(getInstanceRoleDescription(record))}>
            <Tag color={record?.info?.role?.is_master ? 'blue' : 'grey'}>
              {getInstanceRoleLabel(record)}
            </Tag>
          </Tooltip>
        ),
      },
      {
        title: 'CPU',
        dataIndex: 'cpu',
        width: 130,
        render: (_, record) => (
          <ResourceUsage value={record?.info?.resources?.cpu?.usage_percent} />
        ),
      },
      {
        title: t('Memory'),
        dataIndex: 'memory',
        width: 130,
        render: (_, record) => (
          <ResourceUsage
            value={record?.info?.resources?.memory?.usage_percent}
          />
        ),
      },
      {
        title: t('Storage'),
        dataIndex: 'storage',
        width: 130,
        render: (_, record) => (
          <StorageCell storage={record?.info?.resources?.storage} t={t} />
        ),
      },
      {
        title: 'RPM',
        dataIndex: 'rpm',
        width: 90,
        render: (_, record) => formatMetricValue(getInstanceRpm(record)),
      },
      {
        title: t('Active concurrency'),
        dataIndex: 'active_requests',
        width: 130,
        render: (_, record) => (
          <Space spacing={4}>
            <Activity size={14} />
            {formatMetricValue(getInstanceActiveRequests(record))}
          </Space>
        ),
      },
      {
        title: t('Version'),
        dataIndex: 'version',
        width: 120,
        render: (_, record) => record?.info?.runtime?.version || '-',
      },
      {
        title: t('Runtime'),
        dataIndex: 'runtime',
        width: 130,
        render: (_, record) => getInstanceRuntimeLabel(record),
      },
      {
        title: t('Started'),
        dataIndex: 'started_at',
        width: 170,
        render: (value) => formatTimestamp(value),
      },
      {
        title: t('Last Seen'),
        dataIndex: 'last_seen_at',
        width: 170,
        render: (value) => formatTimestamp(value),
      },
      {
        title: t('Actions'),
        dataIndex: 'operate',
        width: 120,
        fixed: 'right',
        render: (_, record) => {
          if (!isStaleInstance(record)) return <Text type='tertiary'>-</Text>;

          return (
            <Popconfirm
              title={t('Delete stale instance')}
              content={t(
                'Delete stale instance "{{name}}"? If it has reported again, it will not be deleted.',
                { name: getInstanceDisplayName(record) },
              )}
              onConfirm={() => deleteStaleInstance(record)}
            >
              <Button
                type='danger'
                theme='borderless'
                size='small'
                loading={deletingNodeName === record.node_name}
                icon={<Trash2 size={14} />}
              >
                {t('Delete')}
              </Button>
            </Popconfirm>
          );
        },
      },
    ],
    [deletingNodeName, t],
  );

  let content;
  if (error) {
    content = (
      <Banner
        type='danger'
        description={error}
        closeIcon={null}
        style={{ marginTop: 12 }}
      />
    );
  } else if (!loading && instances.length === 0) {
    content = (
      <Empty
        image={<ServerCog size={42} />}
        description={t('No instances have reported yet.')}
        style={{ padding: 32 }}
      />
    );
  } else {
    content = (
      <Table
        columns={columns}
        dataSource={instances}
        rowKey='node_name'
        loading={loading}
        pagination={false}
        scroll={{ x: 1700 }}
      />
    );
  }

  return (
    <Form.Section text={t('Multi-node instances')}>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'flex-start',
          gap: 12,
          flexWrap: 'wrap',
          marginBottom: 12,
        }}
      >
        <div>
          <Text>
            {t(
              'Heartbeats and resource usage for application nodes behind load balancing.',
            )}
          </Text>
          <Text
            type='tertiary'
            size='small'
            style={{ display: 'block', marginTop: 4 }}
          >
            {t('Auto-refreshing every {{seconds}}s', {
              seconds: SYSTEM_INSTANCE_POLL_INTERVAL_MS / 1000,
            })}
          </Text>
        </div>
        <Space spacing={8} wrap>
          {staleInstances.length > 0 && (
            <Popconfirm
              title={t('Delete stale instances')}
              content={t(
                'Delete {{count}} stale instance records? Online instances will not be deleted.',
                { count: staleInstances.length },
              )}
              onConfirm={deleteAllStaleInstances}
            >
              <Button
                type='danger'
                size='small'
                loading={deletingAll}
                icon={<Trash2 size={14} />}
              >
                {t('Delete all stale')}
              </Button>
            </Popconfirm>
          )}
          <Button
            size='small'
            loading={loading}
            icon={<RefreshCw size={14} />}
            onClick={() => loadInstances(true)}
          >
            {loading ? t('Refreshing...') : t('Refresh')}
          </Button>
        </Space>
      </div>
      <Space spacing={18} wrap style={{ marginBottom: 12 }}>
        <Text type='tertiary'>
          <Gauge
            size={14}
            style={{ verticalAlign: 'middle', marginRight: 4 }}
          />
          {t('Online RPM')}:{' '}
          <Text strong>
            {hasCompleteTrafficSummary
              ? formatMetricValue(trafficSummary.rpm)
              : '-'}
          </Text>
        </Text>
        <Text type='tertiary'>
          <Activity
            size={14}
            style={{ verticalAlign: 'middle', marginRight: 4 }}
          />
          {t('Online concurrency')}:{' '}
          <Text strong>
            {hasCompleteTrafficSummary
              ? formatMetricValue(trafficSummary.activeRequests)
              : '-'}
          </Text>
        </Text>
      </Space>
      {content}
    </Form.Section>
  );
}
