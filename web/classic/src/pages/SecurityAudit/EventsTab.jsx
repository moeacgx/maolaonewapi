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
import ReactMarkdown from 'react-markdown';
import remarkBreaks from 'remark-breaks';
import remarkGfm from 'remark-gfm';
import {
  Banner,
  Button,
  Card,
  Checkbox,
  Descriptions,
  Empty,
  Input,
  InputNumber,
  Modal,
  Pagination,
  Select,
  Space,
  Spin,
  Table,
  Tabs,
  Tag,
  Toast,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Eye,
  Filter,
  RefreshCw,
  Search,
  Settings2,
  Trash2,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { showError, timestamp2string } from '../../helpers/utils';
import {
  batchDeleteSecurityAuditEvents,
  cleanSecurityAuditFilter,
  deleteSecurityAuditEvent,
  deleteSecurityAuditEventsByFilter,
  getSecurityAuditEvent,
  getSecurityAuditEvents,
  getSecurityAuditBuiltinPolicyChannels,
  previewSecurityAuditDelete,
} from './api';
import { getDecisionColor, getRiskColor } from './constants';
import {
  createKeywordHighlightPlugin,
  normalizeMatchedKeywords,
} from './matched-keyword-highlight';
import {
  AUDIT_EVENT_ORIGIN_ASSIGNED,
  AUDIT_EVENT_ORIGIN_HISTORICAL,
  getAuditEventChannelOrigin,
  getAuditEventChannelGroupsOrigin,
  getAuditEventRouteGroupOrigin,
  getAuditEventTokenGroupsOrigin,
} from './event-origin';

const { Text } = Typography;

const EMPTY_FILTER = {
  keyword: '',
  source: '',
  stage: '',
  decision: '',
  action: '',
  risk_level: '',
  endpoint: '',
  username: '',
  token_id: undefined,
  group_id: undefined,
  channel_id: undefined,
};

const getDecisionLabel = (decision, t) => {
  switch (
    String(decision || '')
      .trim()
      .toLowerCase()
  ) {
    case 'pass':
    case 'allow':
    case 'allowed':
    case 'safe':
      return t('允许');
    case 'flag':
    case 'flagged':
      return t('标记');
    case 'critical':
    case 'block':
    case 'blocked':
      return t('阻断');
    case 'error':
      return t('错误');
    default:
      return decision || '-';
  }
};

const getRiskLevelLabel = (riskLevel, t) => {
  switch (
    String(riskLevel || '')
      .trim()
      .toLowerCase()
  ) {
    case 'safe':
      return t('安全');
    case 'low':
      return t('低风险');
    case 'medium':
      return t('中风险');
    case 'high':
      return t('高风险');
    case 'critical':
      return t('严重风险');
    case 'unknown':
      return t('未知');
    default:
      return riskLevel || '-';
  }
};

const CATEGORY_LABELS = {
  sensitive_word: '屏蔽词',
  cyber_policy: '官方风控（cyber_policy）',
  violent: '暴力内容',
  violence: '暴力内容',
  'violent content': '暴力内容',
  non_violent_illegal_acts: '非暴力违法行为',
  'non violent illegal acts': '非暴力违法行为',
  sexual_content_or_sexual_acts: '色情内容或性行为',
  'sexual content or sexual acts': '色情内容或性行为',
  pii: '个人敏感信息',
  'personal sensitive information': '个人敏感信息',
  suicide_and_self_harm: '自杀与自残',
  'suicide and self harm': '自杀与自残',
  unethical_acts: '不道德行为',
  'unethical acts': '不道德行为',
  politically_sensitive_topics: '政治敏感话题',
  'politically sensitive topics': '政治敏感话题',
  copyright_violation: '版权侵权',
  'copyright violation': '版权侵权',
  jailbreak: '越狱攻击',
  'jailbreak attack': '越狱攻击',
};

const getCategoryLabel = (category, t) => {
  const normalized = String(category || '')
    .trim()
    .toLowerCase()
    .replaceAll('-', ' ');
  return CATEGORY_LABELS[normalized]
    ? t(CATEGORY_LABELS[normalized])
    : category || '-';
};

const COLUMN_VISIBILITY_STORAGE_KEY = 'classic-security-audit-event-columns';

const DEFAULT_COLUMN_VISIBILITY = {
  created_at: true,
  decision: true,
  action: true,
  risk_level: true,
  risk_categories: true,
  source: true,
  redacted_preview: false,
  matched_keywords: true,
  username: true,
  user_cyber_policy_count: true,
  model: true,
  channel_id: true,
  token_groups: false,
  group_id: false,
  guard_endpoint_id: true,
  latency_ms: true,
  operate: true,
};

const loadColumnVisibility = () => {
  try {
    const raw = localStorage.getItem(COLUMN_VISIBILITY_STORAGE_KEY);
    if (!raw) return { ...DEFAULT_COLUMN_VISIBILITY };
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return { ...DEFAULT_COLUMN_VISIBILITY };
    }
    return {
      ...DEFAULT_COLUMN_VISIBILITY,
      ...Object.fromEntries(
        Object.entries(parsed).filter(
          ([, value]) => typeof value === 'boolean',
        ),
      ),
    };
  } catch {
    return { ...DEFAULT_COLUMN_VISIBILITY };
  }
};

const getSourceLabel = (source, t) => {
  switch (source) {
    case 'sensitive_word':
      return t('屏蔽词');
    case 'upstream_policy':
      return t('官方风控（cyber_policy）');
    case 'prompt_guard':
      return t('Prompt Guard');
    default:
      return source || t('Prompt Guard');
  }
};

const getSourceColor = (source) => {
  switch (source) {
    case 'sensitive_word':
      return 'amber';
    case 'upstream_policy':
      return 'violet';
    default:
      return 'blue';
  }
};

const getActionLabel = (action, t) => {
  switch (
    String(action || '')
      .trim()
      .toLowerCase()
  ) {
    case 'block':
      return t('已拦截');
    case 'mask':
      return t('已过滤（脱敏）');
    case 'warn':
      return t('仅标记');
    case 'allow':
      return t('已放行');
    case 'pending':
      return t('待处理');
    case 'error':
      return t('处理失败');
    default:
      return action || '-';
  }
};

const getActionColor = (action) => {
  switch (
    String(action || '')
      .trim()
      .toLowerCase()
  ) {
    case 'block':
      return 'red';
    case 'mask':
      return 'amber';
    case 'allow':
      return 'green';
    case 'error':
      return 'red';
    default:
      return 'blue';
  }
};

const getStageLabel = (stage, t) => {
  switch (stage) {
    case 'request':
      return t('请求');
    case 'response':
      return t('返回');
    case 'response_stream':
      return t('流式返回');
    case 'realtime_request':
      return t('Realtime 请求');
    case 'realtime_response':
      return t('Realtime 返回');
    case 'task_response':
      return t('任务返回');
    case 'http':
      return t('HTTP 请求');
    case 'realtime':
      return t('Realtime 请求');
    case 'async_worker':
      return t('异步 Worker');
    default:
      return stage || '-';
  }
};

const getContextSide = (stage) => {
  const normalized = String(stage || '')
    .trim()
    .toLowerCase();
  return normalized.includes('response') || normalized === 'task_response'
    ? 'llm'
    : 'client';
};

const getOriginStateLabel = (state, t) =>
  state === AUDIT_EVENT_ORIGIN_HISTORICAL ? t('历史事件未记录') : t('尚未分配');

const renderChannelOrigin = (event, t) => {
  const channel = getAuditEventChannelOrigin(event);
  if (channel.state !== AUDIT_EVENT_ORIGIN_ASSIGNED) {
    return (
      <Text type='tertiary' size='small'>
        {getOriginStateLabel(channel.state, t)}
      </Text>
    );
  }

  return (
    <div className='min-w-0'>
      <Text ellipsis={{ showTooltip: true }} className='block'>
        {channel.name || t('未知渠道')}
      </Text>
      <Text type='tertiary' size='small' className='block tabular-nums'>
        #{channel.id}
      </Text>
    </div>
  );
};

const renderGroupOrigin = (event, t, kind = 'route') => {
  const groups =
    kind === 'channel'
      ? getAuditEventChannelGroupsOrigin(event)
      : getAuditEventRouteGroupOrigin(event);
  if (kind === 'channel' && groups.items.length === 0) {
    return (
      <Text type='tertiary' size='small'>
        {groups.state === AUDIT_EVENT_ORIGIN_ASSIGNED
          ? t('暂无渠道分组')
          : getOriginStateLabel(groups.state, t)}
      </Text>
    );
  }
  if (groups.state !== AUDIT_EVENT_ORIGIN_ASSIGNED) {
    return (
      <Text type='tertiary' size='small'>
        {getOriginStateLabel(groups.state, t)}
      </Text>
    );
  }

  return (
    <div className='flex flex-wrap gap-1'>
      {groups.items.map((group, index) => {
        const title = group.name || group.code || t('分组');
        const meta = group.id ? `#${group.id}` : group.code;
        return (
          <Tag
            key={`${group.id || group.code || group.name}-${index}`}
            color='cyan'
          >
            {meta ? `${title} (${meta})` : title}
          </Tag>
        );
      })}
    </div>
  );
};

const renderTokenGroupOrigin = (event, t) => {
  const groups = getAuditEventTokenGroupsOrigin(event);
  if (groups.state === AUDIT_EVENT_ORIGIN_HISTORICAL) {
    return (
      <Text type='tertiary' size='small'>
        {t('历史事件未记录')}
      </Text>
    );
  }
  if (groups.mode === 'auto') {
    return <Tag color='orange'>auto</Tag>;
  }
  if (groups.items.length === 0) {
    return (
      <Text type='tertiary' size='small'>
        {t('未绑定')}
      </Text>
    );
  }
  return (
    <div className='flex flex-wrap gap-1'>
      {groups.mode === 'inherit' ? (
        <Tag color='grey'>{t('继承默认')}</Tag>
      ) : null}
      {groups.items.map((group, index) => {
        const title = group.name || group.code || t('分组');
        const meta = group.id ? `#${group.id}` : group.code;
        return (
          <Tag
            key={`${group.id || group.code || group.name}-${index}`}
            color='cyan'
          >
            {meta ? `${title} (${meta})` : title}
          </Tag>
        );
      })}
    </div>
  );
};

const EventsTab = ({ endpoints }) => {
  const { t } = useTranslation();
  const [filter, setFilter] = useState(EMPTY_FILTER);
  const [appliedFilter, setAppliedFilter] = useState(EMPTY_FILTER);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [events, setEvents] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [refreshKey, setRefreshKey] = useState(0);
  const [selectedRowKeys, setSelectedRowKeys] = useState([]);
  const [detail, setDetail] = useState(null);
  const [detailVisible, setDetailVisible] = useState(false);
  const [contextFilter, setContextFilter] = useState('all');
  const [channels, setChannels] = useState([]);
  const [columnSettingsVisible, setColumnSettingsVisible] = useState(false);
  const [columnVisibility, setColumnVisibility] = useState(() =>
    loadColumnVisibility(),
  );

  useEffect(() => {
    localStorage.setItem(
      COLUMN_VISIBILITY_STORAGE_KEY,
      JSON.stringify(columnVisibility),
    );
  }, [columnVisibility]);

  useEffect(() => {
    let active = true;
    void getSecurityAuditBuiltinPolicyChannels()
      .then((items) => {
        if (!active) return;
        setChannels(
          [...(items || [])]
            .filter(
              (channel) => Number.isInteger(channel?.id) && channel.id > 0,
            )
            .sort((left, right) =>
              String(left.name || left.id).localeCompare(
                String(right.name || right.id),
              ),
            ),
        );
      })
      .catch(() => {
        if (active) setChannels([]);
      });
    return () => {
      active = false;
    };
  }, []);

  const loadEvents = useCallback(async () => {
    setLoading(true);
    try {
      const result = await getSecurityAuditEvents(
        appliedFilter,
        page,
        pageSize,
      );
      setEvents(result?.items || []);
      setTotal(Number(result?.total || 0));
      setSelectedRowKeys([]);
    } catch (error) {
      showError(error?.message || t('审计事件加载失败'));
    } finally {
      setLoading(false);
    }
  }, [appliedFilter, page, pageSize, refreshKey, t]);

  useEffect(() => {
    void loadEvents();
  }, [loadEvents]);

  const refresh = () => setRefreshKey((value) => value + 1);

  const applyFilter = () => {
    setPage(1);
    setAppliedFilter({ ...filter });
  };

  const resetFilter = () => {
    setFilter({ ...EMPTY_FILTER });
    setAppliedFilter({ ...EMPTY_FILTER });
    setPage(1);
  };

  const openDetail = (event) => {
    void getSecurityAuditEvent(event.id)
      .then((result) => {
        setDetail(result);
        setContextFilter('all');
        setDetailVisible(true);
      })
      .catch((error) => showError(error?.message || t('详情加载失败')));
  };

  const removeOne = (event) => {
    Modal.confirm({
      title: t('删除审计事件'),
      content: t('该操作会同时清理关联的已完成任务，且无法撤销。'),
      okType: 'danger',
      onOk: () =>
        deleteSecurityAuditEvent(event.id)
          .then((result) => {
            Toast.success({
              content: t('已删除 {{count}} 条审计事件', {
                count: result?.deleted_events || 0,
              }),
            });
            refresh();
          })
          .catch((error) => showError(error?.message || t('删除失败'))),
    });
  };

  const batchRemove = () => {
    if (selectedRowKeys.length === 0) return;
    const ids = selectedRowKeys.map(Number).filter((id) => id > 0);
    Modal.confirm({
      title: t('批量删除审计事件'),
      content: t('确定删除选中的 {{count}} 条审计事件吗？', {
        count: ids.length,
      }),
      okType: 'danger',
      onOk: () =>
        batchDeleteSecurityAuditEvents(ids)
          .then((result) => {
            Toast.success({
              content: t('已删除 {{count}} 条审计事件', {
                count: result?.deleted_events || 0,
              }),
            });
            refresh();
          })
          .catch((error) => showError(error?.message || t('删除失败'))),
    });
  };

  const confirmFilteredDelete = (preview, filterSnapshot) => {
    Modal.confirm({
      title: t('按筛选删除审计事件'),
      content: (
        <div className='space-y-2'>
          <div>
            {t('预览匹配 {{count}} 条事件，仅删除预览时已存在的数据。', {
              count: preview?.matched_count || 0,
            })}
          </div>
          <Text type='tertiary' size='small'>
            {t('确认令牌五分钟内有效；事件变化后必须重新预览。')}
          </Text>
        </div>
      ),
      okType: 'danger',
      okText: t('确认删除'),
      onOk: () =>
        deleteSecurityAuditEventsByFilter(filterSnapshot, preview)
          .then((result) => {
            Toast.success({
              content: t('已删除 {{count}} 条审计事件', {
                count: result?.deleted_events || 0,
              }),
            });
            refresh();
          })
          .catch((error) => showError(error?.message || t('删除失败'))),
    });
  };

  const previewFilteredDelete = () => {
    const filterSnapshot = cleanSecurityAuditFilter(appliedFilter);
    if (Object.keys(filterSnapshot).length === 0) {
      Toast.warning({ content: t('请至少设置一个筛选条件') });
      return;
    }
    void previewSecurityAuditDelete(filterSnapshot)
      .then((preview) => confirmFilteredDelete(preview, filterSnapshot))
      .catch((error) => showError(error?.message || t('删除预览失败')));
  };

  const columns = useMemo(
    () => [
      {
        title: t('时间'),
        dataIndex: 'created_at',
        width: 170,
        render: (value) => (value ? timestamp2string(value) : '-'),
      },
      {
        title: t('判定'),
        dataIndex: 'decision',
        width: 100,
        render: (value) => (
          <Tag color={getDecisionColor(value)}>
            {getDecisionLabel(value, t)}
          </Tag>
        ),
      },
      {
        title: t('处理结果'),
        dataIndex: 'action',
        width: 130,
        render: (value) => (
          <Tag color={getActionColor(value)}>{getActionLabel(value, t)}</Tag>
        ),
      },
      {
        title: t('风险等级'),
        dataIndex: 'risk_level',
        width: 110,
        render: (value) => (
          <Tag color={getRiskColor(value)}>{getRiskLevelLabel(value, t)}</Tag>
        ),
      },
      {
        title: t('风险分类'),
        dataIndex: 'risk_categories',
        width: 220,
        render: (_, record) =>
          (record.categories || []).length > 0 ? (
            <div className='flex flex-wrap gap-1'>
              {record.categories.slice(0, 3).map((category) => (
                <Tag key={category} color='orange'>
                  {getCategoryLabel(category, t)}
                </Tag>
              ))}
            </div>
          ) : (
            '-'
          ),
      },
      {
        title: t('审计来源'),
        dataIndex: 'source',
        width: 160,
        render: (value, record) => (
          <div className='space-y-1'>
            <Tag color={getSourceColor(value)}>{getSourceLabel(value, t)}</Tag>
            <Text type='tertiary' size='small' className='block'>
              {getStageLabel(record.stage, t)}
            </Text>
          </div>
        ),
      },
      {
        title: t('正文预览'),
        dataIndex: 'redacted_preview',
        width: 360,
        render: (value, record) => (
          <Text ellipsis={{ showTooltip: true, rows: 2 }}>
            {record.prompt_available === false
              ? t('未保存提示词正文')
              : value || '-'}
          </Text>
        ),
      },
      {
        title: t('拦截关键词'),
        dataIndex: 'matched_keywords',
        width: 220,
        render: (value) => {
          const keywords = normalizeMatchedKeywords(value);
          return keywords.length > 0 ? (
            <div className='flex flex-wrap gap-1'>
              {keywords.map((keyword) => (
                <Tag key={keyword.toLowerCase()} color='red'>
                  {keyword}
                </Tag>
              ))}
            </div>
          ) : (
            '-'
          );
        },
      },
      {
        title: t('用户'),
        dataIndex: 'username',
        width: 140,
        render: (value, record) => value || `#${record.user_id || '-'}`,
      },
      {
        title: t('窗口内累计'),
        dataIndex: 'user_cyber_policy_count',
        width: 130,
        render: (value, record) => {
          const count = Math.max(0, Number(value) || 0);
          const hours = Math.max(
            0,
            Number(record.cyber_policy_window_hours) || 0,
          );
          return (
            <div className='flex flex-col gap-0.5 tabular-nums'>
              <Text strong>{t('{{count}} 次', { count })}</Text>
              <Text type='tertiary' size='small'>
                {hours > 0 ? t('{{hours}} 小时内', { hours }) : '-'}
              </Text>
            </div>
          );
        },
      },
      {
        title: t('模型'),
        dataIndex: 'model',
        width: 180,
        render: (value) => (
          <Text ellipsis={{ showTooltip: true }}>{value || '-'}</Text>
        ),
      },
      {
        title: t('渠道'),
        dataIndex: 'channel_id',
        width: 180,
        render: (_, record) => renderChannelOrigin(record, t),
      },
      {
        title: t('令牌绑定分组'),
        dataIndex: 'token_groups',
        render: (_, record) => renderTokenGroupOrigin(record, t),
      },
      {
        title: t('分组'),
        dataIndex: 'group_id',
        width: 220,
        render: (_, record) => renderGroupOrigin(record, t),
      },
      {
        title: t('Guard 节点'),
        dataIndex: 'guard_endpoint_id',
        width: 140,
        render: (value) => value || '-',
      },
      {
        title: t('延迟'),
        dataIndex: 'latency_ms',
        width: 90,
        render: (value) => `${value || 0} ms`,
      },
      {
        title: t('操作'),
        dataIndex: 'operate',
        fixed: 'right',
        width: 130,
        render: (_, record) => (
          <Space spacing={4}>
            <Button
              size='small'
              theme='borderless'
              icon={<Eye size={15} />}
              aria-label={t('查看详情')}
              onClick={(event) => {
                event.stopPropagation();
                openDetail(record);
              }}
            />
            <Button
              size='small'
              theme='borderless'
              type='danger'
              icon={<Trash2 size={15} />}
              aria-label={t('删除')}
              onClick={(event) => {
                event.stopPropagation();
                removeOne(record);
              }}
            />
          </Space>
        ),
      },
    ],
    [t],
  );

  const visibleColumns = useMemo(
    () =>
      columns.filter(
        (column) =>
          column.dataIndex === 'operate' || columnVisibility[column.dataIndex],
      ),
    [columnVisibility, columns],
  );

  const optionalColumns = useMemo(
    () => columns.filter((column) => column.dataIndex !== 'operate'),
    [columns],
  );

  const expandedRowRender = (record) => (
    <div className='rounded-lg border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] p-3'>
      <Descriptions
        data={[
          {
            key: t('正文预览'),
            value:
              record.prompt_available === false
                ? t('未保存提示词正文')
                : record.redacted_preview || '-',
          },
          {
            key: t('提示词哈希'),
            value: record.prompt_hash || '-',
          },
          {
            key: t('分组'),
            value: renderGroupOrigin(record, t),
          },
          {
            key: t('令牌绑定分组'),
            value: renderTokenGroupOrigin(record, t),
          },
        ]}
      />
    </div>
  );

  const renderPromptContext = () => {
    if (!detail) return null;
    const segments = detail.context_segments || [];
    const matchedKeywords = normalizeMatchedKeywords(detail.matched_keywords);
    const visible = segments.filter(
      (segment) => contextFilter === 'all' || segment.kind === contextFilter,
    );
    return (
      <div className='audit-prompt-rendered max-h-[52vh] min-h-[10rem] overflow-y-auto overscroll-contain break-words rounded-xl border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] p-4 text-sm leading-6'>
        {segments.length > 0 ? (
          <div className='space-y-4'>
            {visible.map((segment, index) => (
              <section
                key={`${segment.role}-${index}`}
                className='rounded-md border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] p-3'
              >
                <div className='mb-2 flex items-center gap-2'>
                  <Tag color={segment.kind === 'llm' ? 'violet' : 'blue'}>
                    {segment.kind === 'llm' ? t('LLM 输出') : t('客户端输出')}
                  </Tag>
                  <Text type='tertiary' size='small'>
                    {segment.role ||
                      (segment.kind === 'llm' ? 'assistant' : 'user')}
                  </Text>
                </div>
                <ReactMarkdown
                  remarkPlugins={[remarkGfm, remarkBreaks]}
                  rehypePlugins={
                    matchedKeywords.length > 0
                      ? [createKeywordHighlightPlugin(matchedKeywords)]
                      : []
                  }
                  components={{
                    a: ({ node: _node, ...props }) => (
                      <a {...props} target='_blank' rel='noopener noreferrer' />
                    ),
                  }}
                >
                  {segment.text}
                </ReactMarkdown>
              </section>
            ))}
            {visible.length === 0 ? (
              <Text type='tertiary' className='block py-8 text-center'>
                {t('没有匹配的上下文输出')}
              </Text>
            ) : null}
          </div>
        ) : contextFilter !== 'all' &&
          contextFilter !== getContextSide(detail.stage) ? (
          <Text type='tertiary' className='block py-8 text-center'>
            {t('没有匹配的上下文输出')}
          </Text>
        ) : (
          <ReactMarkdown
            remarkPlugins={[remarkGfm, remarkBreaks]}
            rehypePlugins={
              matchedKeywords.length > 0
                ? [createKeywordHighlightPlugin(matchedKeywords)]
                : []
            }
          >
            {detail.full_prompt}
          </ReactMarkdown>
        )}
      </div>
    );
  };

  return (
    <div className='space-y-4'>
      <Card bodyStyle={{ padding: 16 }}>
        <div className='grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4'>
          <Input
            prefix={<Search size={15} />}
            value={filter.keyword}
            placeholder={t('搜索正文预览')}
            onChange={(value) =>
              setFilter((current) => ({ ...current, keyword: value }))
            }
            onEnterPress={applyFilter}
          />
          <Select
            value={filter.source || undefined}
            placeholder={t('审计来源')}
            showClear
            onChange={(value) =>
              setFilter((current) => ({ ...current, source: value || '' }))
            }
          >
            <Select.Option value='prompt_guard'>
              {t('Prompt Guard')}
            </Select.Option>
            <Select.Option value='sensitive_word'>{t('屏蔽词')}</Select.Option>
            <Select.Option value='upstream_policy'>
              {t('官方风控（cyber_policy）')}
            </Select.Option>
          </Select>
          <Select
            value={filter.stage || undefined}
            placeholder={t('处理阶段')}
            showClear
            onChange={(value) =>
              setFilter((current) => ({ ...current, stage: value || '' }))
            }
          >
            {[
              'request',
              'response',
              'response_stream',
              'realtime_request',
              'realtime_response',
              'task_response',
              'http',
              'realtime',
              'async_worker',
            ].map((value) => (
              <Select.Option key={value} value={value}>
                {getStageLabel(value, t)}
              </Select.Option>
            ))}
          </Select>
          <Select
            value={filter.decision || undefined}
            placeholder={t('判定结果')}
            showClear
            onChange={(value) =>
              setFilter((current) => ({ ...current, decision: value || '' }))
            }
          >
            <Select.Option value='pass'>{t('允许')}</Select.Option>
            <Select.Option value='flag'>{t('标记')}</Select.Option>
            <Select.Option value='critical'>{t('阻断')}</Select.Option>
            <Select.Option value='error'>{t('错误')}</Select.Option>
          </Select>
          <Select
            value={filter.action || undefined}
            placeholder={t('处理结果')}
            showClear
            onChange={(value) =>
              setFilter((current) => ({ ...current, action: value || '' }))
            }
          >
            <Select.Option value='block'>{t('已拦截')}</Select.Option>
            <Select.Option value='mask'>{t('已过滤（脱敏）')}</Select.Option>
            <Select.Option value='warn'>{t('仅标记')}</Select.Option>
            <Select.Option value='allow'>{t('已放行')}</Select.Option>
            <Select.Option value='pending'>{t('待处理')}</Select.Option>
            <Select.Option value='error'>{t('处理失败')}</Select.Option>
          </Select>
          <Select
            value={filter.risk_level || undefined}
            placeholder={t('风险等级')}
            showClear
            onChange={(value) =>
              setFilter((current) => ({ ...current, risk_level: value || '' }))
            }
          >
            {['safe', 'low', 'medium', 'high', 'critical'].map((value) => (
              <Select.Option key={value} value={value}>
                {getRiskLevelLabel(value, t)}
              </Select.Option>
            ))}
          </Select>
          <Select
            value={filter.channel_id || undefined}
            placeholder={t('渠道')}
            showClear
            filter
            onChange={(value) =>
              setFilter((current) => ({
                ...current,
                channel_id: value ? Number(value) : undefined,
              }))
            }
          >
            {channels.map((channel) => (
              <Select.Option key={channel.id} value={channel.id}>
                {channel.name || t('未知渠道')} #{channel.id}
              </Select.Option>
            ))}
          </Select>
          <Select
            value={filter.endpoint || undefined}
            placeholder={t('Guard 节点')}
            showClear
            filter
            onChange={(value) =>
              setFilter((current) => ({ ...current, endpoint: value || '' }))
            }
          >
            {(endpoints || []).map((endpoint) => (
              <Select.Option key={endpoint.id} value={endpoint.id}>
                {endpoint.name || endpoint.id}
              </Select.Option>
            ))}
          </Select>
          <Input
            value={filter.username}
            placeholder={t('用户名')}
            maxLength={128}
            onChange={(value) =>
              setFilter((current) => ({ ...current, username: value }))
            }
            onEnterPress={applyFilter}
          />
          <InputNumber
            value={filter.token_id}
            min={1}
            placeholder={t('令牌 ID')}
            style={{ width: '100%' }}
            onChange={(value) =>
              setFilter((current) => ({ ...current, token_id: value }))
            }
          />
          <InputNumber
            value={filter.group_id}
            min={1}
            placeholder={t('分组 ID')}
            style={{ width: '100%' }}
            onChange={(value) =>
              setFilter((current) => ({ ...current, group_id: value }))
            }
          />
          <Space wrap>
            <Button
              type='primary'
              icon={<Filter size={15} />}
              onClick={applyFilter}
            >
              {t('筛选')}
            </Button>
            <Button onClick={resetFilter}>{t('重置')}</Button>
          </Space>
        </div>
      </Card>

      <Card
        title={t('审计事件')}
        headerExtraContent={
          <Space wrap>
            <Button
              type='danger'
              theme='outline'
              icon={<Trash2 size={15} />}
              disabled={selectedRowKeys.length === 0}
              onClick={batchRemove}
            >
              {t('删除选中')}
            </Button>
            <Button
              type='danger'
              theme='outline'
              onClick={previewFilteredDelete}
            >
              {t('按筛选删除')}
            </Button>
            <Button
              icon={<RefreshCw size={15} />}
              loading={loading}
              onClick={refresh}
            >
              {t('刷新')}
            </Button>
            <Button
              icon={<Settings2 size={15} />}
              onClick={() => setColumnSettingsVisible(true)}
            >
              {t('列设置')}
            </Button>
          </Space>
        }
        bodyStyle={{ padding: 0 }}
      >
        <Spin spinning={loading}>
          <Table
            rowKey='id'
            columns={visibleColumns}
            dataSource={events}
            pagination={false}
            scroll={{
              x: Math.max(
                1600,
                visibleColumns.reduce(
                  (totalWidth, column) => totalWidth + (column.width || 160),
                  0,
                ),
              ),
            }}
            rowSelection={{
              selectedRowKeys,
              onChange: (keys) => setSelectedRowKeys(keys),
            }}
            empty={<Empty description={t('暂无审计事件')} />}
            expandedRowRender={expandedRowRender}
            expandRowByClick
          />
          <div className='flex flex-col gap-3 border-t border-[var(--semi-color-border)] p-4 sm:flex-row sm:items-center sm:justify-between'>
            <Text type='tertiary' size='small'>
              {t('共 {{count}} 条', { count: total })}
            </Text>
            <Pagination
              currentPage={page}
              pageSize={pageSize}
              total={total}
              showSizeChanger
              pageSizeOpts={[20, 50, 100]}
              onPageChange={setPage}
              onPageSizeChange={(size) => {
                setPageSize(size);
                setPage(1);
              }}
            />
          </div>
        </Spin>
      </Card>

      <Modal
        title={t('列设置')}
        visible={columnSettingsVisible}
        onCancel={() => setColumnSettingsVisible(false)}
        footer={
          <Space>
            <Button
              onClick={() =>
                setColumnVisibility({ ...DEFAULT_COLUMN_VISIBILITY })
              }
            >
              {t('重置')}
            </Button>
            <Button
              type='primary'
              onClick={() => setColumnSettingsVisible(false)}
            >
              {t('确定')}
            </Button>
          </Space>
        }
      >
        <div className='mb-4'>
          <Checkbox
            checked={optionalColumns.every(
              (column) => columnVisibility[column.dataIndex],
            )}
            indeterminate={
              optionalColumns.some(
                (column) => columnVisibility[column.dataIndex],
              ) &&
              !optionalColumns.every(
                (column) => columnVisibility[column.dataIndex],
              )
            }
            onChange={(event) => {
              const checked = event.target.checked;
              setColumnVisibility((current) => ({
                ...current,
                ...Object.fromEntries(
                  optionalColumns.map((column) => [column.dataIndex, checked]),
                ),
              }));
            }}
          >
            {t('全选')}
          </Checkbox>
        </div>
        <div className='grid max-h-96 grid-cols-1 gap-3 overflow-y-auto rounded-lg border border-[var(--semi-color-border)] p-4 sm:grid-cols-2'>
          {optionalColumns.map((column) => (
            <Checkbox
              key={column.dataIndex}
              checked={columnVisibility[column.dataIndex] !== false}
              onChange={(event) =>
                setColumnVisibility((current) => ({
                  ...current,
                  [column.dataIndex]: event.target.checked,
                }))
              }
            >
              {column.title}
            </Checkbox>
          ))}
        </div>
      </Modal>

      <Modal
        title={t('审计事件详情')}
        visible={detailVisible}
        onCancel={() => {
          setDetailVisible(false);
          setDetail(null);
          setContextFilter('all');
        }}
        footer={
          <Button
            type='primary'
            onClick={() => {
              setDetailVisible(false);
              setDetail(null);
              setContextFilter('all');
            }}
          >
            {t('关闭')}
          </Button>
        }
        width={820}
        style={{ maxWidth: '94vw' }}
      >
        {detail ? (
          <div className='space-y-4'>
            <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
              {[
                ['请求 ID', detail.request_id || '-'],
                ['提示词哈希', detail.prompt_hash || '-'],
                ['用户', detail.username || `#${detail.user_id || '-'}`],
                ['模型', detail.model || '-'],
                ['渠道', renderChannelOrigin(detail, t)],
                ['令牌绑定分组', renderTokenGroupOrigin(detail, t)],
                ['分组', renderGroupOrigin(detail, t)],
                ['审计来源', getSourceLabel(detail.source, t)],
                ['处理阶段', getStageLabel(detail.stage, t)],
                ['判定', getDecisionLabel(detail.decision, t)],
                ['处理结果', getActionLabel(detail.action, t)],
                ['风险等级', getRiskLevelLabel(detail.risk_level, t)],
                ['风险分数', detail.risk_score ?? '-'],
                ['字符数', detail.prompt_length || 0],
                ['分片数', detail.chunk_total || 0],
              ].map(([label, value]) => (
                <div
                  key={label}
                  className='rounded-lg bg-[var(--semi-color-fill-0)] p-3'
                >
                  <Text type='tertiary' size='small'>
                    {t(label)}
                  </Text>
                  <div className='mt-1 break-all'>{value}</div>
                </div>
              ))}
            </div>
            <div>
              <Text strong>{t('风险分类')}</Text>
              <div className='mt-2 flex flex-wrap gap-2'>
                {(detail.categories || []).length > 0 ? (
                  detail.categories.map((category) => (
                    <Tag key={category} color='orange'>
                      {getCategoryLabel(category, t)}
                    </Tag>
                  ))
                ) : (
                  <Text type='tertiary'>-</Text>
                )}
              </div>
            </div>
            {normalizeMatchedKeywords(detail.matched_keywords).length > 0 ? (
              <div>
                <Text strong>{t('关键词')}</Text>
                <div className='mt-2 flex flex-wrap gap-2'>
                  {normalizeMatchedKeywords(detail.matched_keywords).map(
                    (keyword) => (
                      <Tag key={keyword.toLowerCase()} color='red'>
                        {keyword}
                      </Tag>
                    ),
                  )}
                </div>
              </div>
            ) : null}
            {detail.prompt_available && detail.full_prompt ? (
              <div>
                <div className='flex flex-wrap items-center gap-2'>
                  <Text strong>{t('完整提示词')}</Text>
                  <Tag
                    color={
                      getContextSide(detail.stage) === 'llm' ? 'violet' : 'blue'
                    }
                  >
                    {getContextSide(detail.stage) === 'llm'
                      ? t('LLM → 客户端')
                      : t('客户端 → LLM')}
                  </Tag>
                </div>
                <Tabs
                  type='button'
                  activeKey={contextFilter}
                  onChange={setContextFilter}
                  className='mt-2'
                >
                  <Tabs.TabPane tab={t('全部输出')} itemKey='all'>
                    {renderPromptContext()}
                  </Tabs.TabPane>
                  <Tabs.TabPane tab={t('客户端输出')} itemKey='client'>
                    {renderPromptContext()}
                  </Tabs.TabPane>
                  <Tabs.TabPane tab={t('LLM 输出')} itemKey='llm'>
                    {renderPromptContext()}
                  </Tabs.TabPane>
                </Tabs>
                {detail.prompt_truncated ? (
                  <Text type='warning' size='small' className='mt-2 block'>
                    {t('该提示词已按持久化上限截断。')}
                  </Text>
                ) : null}
              </div>
            ) : (
              <Banner
                type='info'
                closeIcon={null}
                description={t(
                  '未保存提示词正文。加密存储不可用时，事件仅保留不可逆哈希、长度、来源和技术元数据。',
                )}
              />
            )}
          </div>
        ) : null}
      </Modal>
    </div>
  );
};

export default EventsTab;
