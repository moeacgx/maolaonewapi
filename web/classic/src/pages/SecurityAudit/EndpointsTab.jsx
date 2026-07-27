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

import React, { useState } from 'react';
import {
  Banner,
  Button,
  Card,
  Empty,
  Input,
  Modal,
  Select,
  Space,
  Switch,
  Tag,
  Toast,
  Typography,
} from '@douyinfe/semi-ui';
import {
  ChevronDown,
  ChevronUp,
  KeyRound,
  Plus,
  Server,
  Stethoscope,
  Trash2,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { showError, timestamp2string } from '../../helpers';
import { probeSecurityAuditEndpoint } from './api';

const { Text } = Typography;

const createEndpoint = () => {
  const suffix = Date.now().toString(36);
  return {
    id: `guard-${suffix}`,
    name: '',
    protocol: 'openai_compatible',
    base_url: '',
    model: 'sileader/qwen3guard:0.6b',
    timeout_ms: 3000,
    input_limit: 4000,
    enabled: true,
    has_token: false,
    token_status: 'missing',
    token_action: 'replace',
    token: '',
  };
};

const EndpointsTab = ({ endpoints, onChange, runSensitive }) => {
  const { t } = useTranslation();
  const [probingId, setProbingId] = useState('');
  const [probeResults, setProbeResults] = useState({});

  const updateEndpoint = (index, patch) => {
    const next = [...endpoints];
    next[index] = { ...next[index], ...patch };
    onChange(next);
  };

  const moveEndpoint = (index, offset) => {
    const target = index + offset;
    if (target < 0 || target >= endpoints.length) return;
    const next = [...endpoints];
    [next[index], next[target]] = [next[target], next[index]];
    onChange(next);
  };

  const removeEndpoint = (index) => {
    const endpoint = endpoints[index];
    Modal.confirm({
      title: t('移除 Guard 节点'),
      content: t('保存配置后该节点将被移除，确定继续吗？'),
      okType: 'danger',
      onOk: () =>
        onChange(endpoints.filter((_, itemIndex) => itemIndex !== index)),
    });
  };

  const probe = (endpoint) => {
    if (!endpoint.id || !endpoint.base_url || !endpoint.model) {
      Toast.warning({ content: t('请先填写节点 ID、地址和模型') });
      return;
    }
    setProbingId(endpoint.id);
    void runSensitive(
      () => probeSecurityAuditEndpoint(endpoint),
      (result) => {
        setProbeResults((current) => ({ ...current, [endpoint.id]: result }));
        if (result?.ok) {
          Toast.success({ content: t('Guard 节点探测成功') });
        } else {
          Toast.error({ content: result?.message || t('Guard 节点探测失败') });
        }
        setProbingId('');
      },
      {
        title: t('验证节点探测'),
        description: t('节点探测会使用已保存或待替换的 Guard 令牌。'),
      },
    )
      .catch((error) => {
        setProbingId('');
        showError(error?.message || t('Guard 节点探测失败'));
      })
      .finally(() => {
        window.setTimeout(() => setProbingId(''), 0);
      });
  };

  return (
    <div className='space-y-4'>
      <Banner
        type='info'
        description={t(
          '节点按页面顺序依次故障切换。令牌不会回显；请选择保留、替换或清除。',
        )}
      />

      <div className='flex justify-end'>
        <Button
          type='primary'
          icon={<Plus size={15} />}
          onClick={() => onChange([...endpoints, createEndpoint()])}
        >
          {t('添加 Guard 节点')}
        </Button>
      </div>

      {endpoints.length === 0 ? (
        <Card>
          <Empty
            image={<Server size={42} color='var(--semi-color-text-2)' />}
            description={t('尚未配置 Guard 节点')}
          />
        </Card>
      ) : (
        endpoints.map((endpoint, index) => {
          const probeResult = probeResults[endpoint.id];
          return (
            <Card
              key={`${endpoint.id}-${index}`}
              title={
                <Space>
                  <span>{endpoint.name || endpoint.id || t('未命名节点')}</span>
                  <Tag color={endpoint.enabled ? 'green' : 'grey'}>
                    {endpoint.enabled ? t('已启用') : t('已停用')}
                  </Tag>
                  <Tag>{`${t('优先级')} ${index + 1}`}</Tag>
                </Space>
              }
              headerExtraContent={
                <Space spacing={4}>
                  <Button
                    theme='borderless'
                    icon={<ChevronUp size={16} />}
                    disabled={index === 0}
                    aria-label={t('上移')}
                    onClick={() => moveEndpoint(index, -1)}
                  />
                  <Button
                    theme='borderless'
                    icon={<ChevronDown size={16} />}
                    disabled={index === endpoints.length - 1}
                    aria-label={t('下移')}
                    onClick={() => moveEndpoint(index, 1)}
                  />
                  <Button
                    theme='borderless'
                    type='danger'
                    icon={<Trash2 size={16} />}
                    aria-label={t('删除')}
                    onClick={() => removeEndpoint(index)}
                  />
                </Space>
              }
              bodyStyle={{ padding: 16 }}
            >
              <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
                <label className='space-y-1'>
                  <Text type='tertiary' size='small'>
                    {t('节点 ID')}
                  </Text>
                  <Input
                    value={endpoint.id}
                    disabled={endpoint.has_token}
                    placeholder='qwen3guard-primary'
                    onChange={(value) => updateEndpoint(index, { id: value })}
                  />
                </label>
                <label className='space-y-1'>
                  <Text type='tertiary' size='small'>
                    {t('节点名称')}
                  </Text>
                  <Input
                    value={endpoint.name}
                    placeholder={t('例如：主 Guard 节点')}
                    onChange={(value) => updateEndpoint(index, { name: value })}
                  />
                </label>
                <label className='space-y-1 lg:col-span-2'>
                  <Text type='tertiary' size='small'>
                    {t('API地址')}
                  </Text>
                  <Input
                    value={endpoint.base_url}
                    placeholder='https://guard.example.com/v1'
                    onChange={(value) =>
                      updateEndpoint(index, { base_url: value })
                    }
                  />
                </label>
                <label className='space-y-1 lg:col-span-2'>
                  <Text type='tertiary' size='small'>
                    {t('模型')}
                  </Text>
                  <Input
                    value={endpoint.model}
                    placeholder='sileader/qwen3guard:0.6b'
                    onChange={(value) =>
                      updateEndpoint(index, { model: value })
                    }
                  />
                </label>
                <label className='space-y-1'>
                  <Text type='tertiary' size='small'>
                    {t('令牌处理')}
                  </Text>
                  <Select
                    value={endpoint.token_action || 'keep'}
                    style={{ width: '100%' }}
                    onChange={(value) =>
                      updateEndpoint(index, {
                        token_action: value,
                        token: value === 'replace' ? endpoint.token || '' : '',
                      })
                    }
                  >
                    <Select.Option value='keep' disabled={!endpoint.has_token}>
                      {t('保留现有令牌')}
                    </Select.Option>
                    <Select.Option value='replace'>
                      {t('替换令牌')}
                    </Select.Option>
                    <Select.Option value='clear'>{t('清除令牌')}</Select.Option>
                  </Select>
                </label>
                <div className='flex items-end pb-1'>
                  <Space>
                    <Switch
                      checked={endpoint.enabled}
                      onChange={(checked) =>
                        updateEndpoint(index, { enabled: checked })
                      }
                    />
                    <Text>{t('启用节点')}</Text>
                  </Space>
                </div>
                {endpoint.token_action === 'replace' ? (
                  <label className='space-y-1 lg:col-span-2'>
                    <Text type='tertiary' size='small'>
                      {t('令牌')}
                    </Text>
                    <Input
                      mode='password'
                      value={endpoint.token}
                      prefix={<KeyRound size={15} />}
                      placeholder={t('输入新的 Guard 令牌')}
                      onChange={(value) =>
                        updateEndpoint(index, { token: value })
                      }
                    />
                  </label>
                ) : null}
              </div>

              <div className='mt-4 flex flex-col gap-3 border-t border-[var(--semi-color-border)] pt-4 sm:flex-row sm:items-center sm:justify-between'>
                <div>
                  <Space>
                    <Tag color={endpoint.has_token ? 'green' : 'orange'}>
                      {endpoint.has_token ? t('令牌已配置') : t('令牌缺失')}
                    </Tag>
                    {probeResult ? (
                      <Tag color={probeResult.ok ? 'green' : 'red'}>
                        {probeResult.ok ? t('健康') : t('异常')}
                      </Tag>
                    ) : null}
                  </Space>
                  {probeResult ? (
                    <Text type='tertiary' size='small' className='mt-2 block'>
                      {t('延迟：{{latency}} 毫秒', {
                        latency: probeResult.latency_ms || 0,
                      })}{' '}
                      · {timestamp2string(probeResult.checked_at)} ·{' '}
                      {probeResult.message || probeResult.error_code || '-'}
                    </Text>
                  ) : null}
                </div>
                <Button
                  icon={<Stethoscope size={15} />}
                  loading={probingId === endpoint.id}
                  onClick={() => probe(endpoint)}
                >
                  {t('连通性探测')}
                </Button>
              </div>
            </Card>
          );
        })
      )}
    </div>
  );
};

export default EndpointsTab;
