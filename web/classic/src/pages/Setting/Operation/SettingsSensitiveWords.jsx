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

import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Button,
  Col,
  Empty,
  Form,
  Input,
  Radio,
  RadioGroup,
  Row,
  Select,
  Space,
  Spin,
  Switch,
  Tag,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IconChevronDown,
  IconChevronRight,
  IconDelete,
  IconPlus,
  IconRefresh,
} from '@douyinfe/semi-icons';
import { API, showError, showWarning } from '../../../helpers';
import { useTranslation } from 'react-i18next';

const ACTION_MASK = 'mask';
const ACTION_BLOCK = 'block';
const SCOPE_REQUEST = 'request';
const SCOPE_RESPONSE = 'response';
const SCOPE_BOTH = 'both';
const TARGET_CHANNEL_TAGS = 'channel_tags';
const TARGET_ROUTES = 'routes';
const TARGET_ALL = 'all';
const DEFAULT_REPLACEMENT = '[REDACTED]';

function createLocalId() {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return crypto.randomUUID();
  }
  return `sensitive-rule-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

function splitKeywords(value) {
  const seen = new Set();
  const keywords = [];

  String(value || '')
    .replace(/\r\n/g, '\n')
    .split('\n')
    .map((item) => item.trim())
    .filter(Boolean)
    .forEach((item) => {
      const key = item.toLowerCase();
      if (seen.has(key)) return;
      seen.add(key);
      keywords.push(item);
    });

  return keywords;
}

function normalizeChannelIds(channelIds) {
  const seen = new Set();
  const normalized = [];

  (channelIds || []).forEach((item) => {
    const id =
      typeof item === 'number' ? item : Number.parseInt(String(item), 10);
    if (!Number.isInteger(id) || id <= 0 || seen.has(id)) return;
    seen.add(id);
    normalized.push(id);
  });

  return normalized.sort((a, b) => a - b);
}

function normalizeChannelTags(channelTags) {
  const seen = new Set();
  const normalized = [];

  (channelTags || [])
    .map((item) => String(item || '').trim())
    .filter(Boolean)
    .forEach((tag) => {
      if (seen.has(tag)) return;
      seen.add(tag);
      normalized.push(tag);
    });

  return normalized.sort();
}

function normalizeGroupCodes(groupCodes) {
  const seen = new Set();
  const normalized = [];
  (groupCodes || [])
    .map((item) => String(item || '').trim())
    .filter((item) => item && item.toLowerCase() !== 'auto')
    .forEach((item) => {
      if (seen.has(item)) return;
      seen.add(item);
      normalized.push(item);
    });
  return normalized.sort();
}

function parseChannelIds(raw) {
  const trimmed = String(raw || '').trim();
  if (!trimmed) return [];

  try {
    const parsed = JSON.parse(trimmed);
    return Array.isArray(parsed) ? normalizeChannelIds(parsed) : [];
  } catch {
    return [];
  }
}

function serializeChannelIds(channelIds) {
  return JSON.stringify(normalizeChannelIds(channelIds));
}

function createRule() {
  return {
    id: createLocalId(),
    name: '',
    enabled: true,
    action: ACTION_MASK,
    scope: SCOPE_REQUEST,
    replacement: DEFAULT_REPLACEMENT,
    keywordsText: '',
    groupRefs: [],
    targetType: TARGET_ROUTES,
    channelIds: [],
    channelTags: [],
    groupCodes: [],
  };
}

function normalizeGroupRefs(groupRefs) {
  const seen = new Set();
  const normalized = [];

  (groupRefs || [])
    .map((item) => String(item).trim())
    .filter(Boolean)
    .forEach((item) => {
      const key = item.toLowerCase();
      if (seen.has(key)) return;
      seen.add(key);
      normalized.push(item);
    });

  return normalized;
}

function normalizeRule(rule) {
  const keywords = splitKeywords(rule.keywordsText);
  const groupRefs = normalizeGroupRefs(rule.groupRefs);
  if (keywords.length === 0 && groupRefs.length === 0) return null;

  const action = rule.action === ACTION_BLOCK ? ACTION_BLOCK : ACTION_MASK;
  const scope =
    rule.scope === SCOPE_RESPONSE || rule.scope === SCOPE_BOTH
      ? rule.scope
      : SCOPE_REQUEST;
  const targetType = [TARGET_CHANNEL_TAGS, TARGET_ALL].includes(rule.targetType)
    ? rule.targetType
    : TARGET_ROUTES;
  const fallbackName = keywords[0] || groupRefs[0] || '';

  return {
    id: rule.id || fallbackName.toLowerCase() || createLocalId(),
    name: String(rule.name || '').trim() || fallbackName,
    enabled: rule.enabled !== false,
    action,
    scope,
    replacement:
      action === ACTION_MASK
        ? String(rule.replacement || '').trim() || DEFAULT_REPLACEMENT
        : undefined,
    keywords,
    group_refs: groupRefs.length > 0 ? groupRefs : undefined,
    target_type: targetType,
    channel_ids:
      targetType === TARGET_ROUTES
        ? normalizeChannelIds(rule.channelIds)
        : undefined,
    channel_tags:
      targetType === TARGET_CHANNEL_TAGS
        ? normalizeChannelTags(rule.channelTags)
        : undefined,
    group_codes:
      targetType === TARGET_ROUTES
        ? normalizeGroupCodes(rule.groupCodes)
        : undefined,
  };
}

function rulesToDrafts(rules, legacyChannelIds) {
  return (rules || []).map((rule) => ({
    id: rule.id || createLocalId(),
    name: rule.name || '',
    enabled: rule.enabled !== false,
    action: rule.action === ACTION_BLOCK ? ACTION_BLOCK : ACTION_MASK,
    scope:
      rule.scope === SCOPE_RESPONSE || rule.scope === SCOPE_BOTH
        ? rule.scope
        : SCOPE_REQUEST,
    replacement: rule.replacement || DEFAULT_REPLACEMENT,
    keywordsText: (rule.keywords || []).join('\n'),
    groupRefs: normalizeGroupRefs(rule.group_refs),
    targetType: [TARGET_CHANNEL_TAGS, TARGET_ALL].includes(rule.target_type)
      ? rule.target_type
      : TARGET_ROUTES,
    channelIds: normalizeChannelIds(
      rule.target_type ? rule.channel_ids : legacyChannelIds,
    ),
    channelTags: normalizeChannelTags(rule.channel_tags),
    groupCodes: normalizeGroupCodes(rule.group_codes),
  }));
}

function serializeRules(rules) {
  return JSON.stringify(
    {
      rules: (rules || [])
        .map((rule) => normalizeRule(rule))
        .filter((rule) => rule !== null),
    },
    null,
    2,
  );
}

function parseRulesConfig(raw, legacyWords, legacyChannelIds) {
  const trimmed = String(raw || '').trim();
  if (trimmed) {
    try {
      const parsed = JSON.parse(trimmed);
      if (Array.isArray(parsed.rules)) {
        return rulesToDrafts(parsed.rules, legacyChannelIds);
      }
    } catch {
      return [];
    }
  }

  const legacyKeywords = splitKeywords(legacyWords);
  if (legacyKeywords.length === 0) return [];

  return rulesToDrafts(
    [
      {
        id: 'legacy-sensitive-words',
        name: 'Legacy sensitive words',
        enabled: true,
        action: ACTION_BLOCK,
        keywords: legacyKeywords,
      },
    ],
    legacyChannelIds,
  );
}

function getEmptyRuleTarget(rule) {
  const hasContent =
    splitKeywords(rule.keywordsText).length > 0 ||
    normalizeGroupRefs(rule.groupRefs).length > 0;
  if (!rule.enabled || !hasContent) return null;

  if (
    rule.targetType === TARGET_CHANNEL_TAGS &&
    normalizeChannelTags(rule.channelTags).length === 0
  ) {
    return TARGET_CHANNEL_TAGS;
  }
  if (rule.targetType === TARGET_ALL) return null;
  if (
    rule.targetType === TARGET_ROUTES &&
    normalizeChannelIds(rule.channelIds).length === 0 &&
    normalizeGroupCodes(rule.groupCodes).length === 0
  ) {
    return TARGET_ROUTES;
  }
  return null;
}

function getInitialExpandedRuleIds(rules) {
  return new Set(
    (rules || [])
      .filter((rule) => getEmptyRuleTarget(rule) !== null)
      .map((rule) => rule.id),
  );
}

function toggleExpandedRuleId(current, id) {
  const next = new Set(current);
  if (next.has(id)) {
    next.delete(id);
  } else {
    next.add(id);
  }
  return next;
}

function expandInvalidRule(current, rule) {
  if (getEmptyRuleTarget(rule) === null || current.has(rule.id)) return current;
  const next = new Set(current);
  next.add(rule.id);
  return next;
}

function removeExpandedRuleId(current, id) {
  if (!current.has(id)) return current;
  const next = new Set(current);
  next.delete(id);
  return next;
}

function getChannelLabel(channel) {
  const name = channel?.name?.trim();
  const base = name ? `${name} #${channel.id}` : `#${channel?.id}`;
  const tag = channel?.tag?.trim();
  return tag ? `${base} · ${tag}` : base;
}

export default function SettingsSensitiveWords(props) {
  const { t } = useTranslation();
  const [saving, setSaving] = useState(false);
  const [channelsLoading, setChannelsLoading] = useState(false);
  const [channelsError, setChannelsError] = useState(false);
  const [channels, setChannels] = useState([]);
  const [groupsLoading, setGroupsLoading] = useState(false);
  const [groupsError, setGroupsError] = useState(false);
  const [groups, setGroups] = useState([]);
  const [inputs, setInputs] = useState({
    CheckSensitiveEnabled: false,
    CheckSensitiveOnPromptEnabled: false,
    SensitiveWords: '',
    SensitiveRules: '{"rules":[]}',
    SensitiveRuleChannelIds: '[]',
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);
  const [rules, setRules] = useState([]);
  const [expandedRuleIds, setExpandedRuleIds] = useState(() => new Set());

  const currentRulesValue = useMemo(() => serializeRules(rules), [rules]);
  const channelIdSet = useMemo(
    () => new Set(channels.map((channel) => channel.id)),
    [channels],
  );
  const groupCodeSet = useMemo(
    () => new Set(groups.map((group) => group.code)),
    [groups],
  );
  const hasInvalidTargets = useMemo(
    () => rules.some((rule) => getEmptyRuleTarget(rule) !== null),
    [rules],
  );
  const hasChanges =
    props.externalDirty === true ||
    inputs.CheckSensitiveEnabled !== inputsRow.CheckSensitiveEnabled ||
    inputs.CheckSensitiveOnPromptEnabled !==
      inputsRow.CheckSensitiveOnPromptEnabled ||
    currentRulesValue !== inputsRow.SensitiveRules;
  const isSaving = saving || props.saving === true;

  const fetchChannels = async () => {
    setChannelsLoading(true);
    setChannelsError(false);
    try {
      const res = await API.get('/api/security-audit/builtin-policy/channels');
      const { success, message, data } = res.data;
      if (!success) {
        setChannelsError(true);
        showError(message);
        return;
      }
      const sortedChannels = [...(data || [])]
        .filter((channel) => Number.isInteger(channel?.id) && channel.id > 0)
        .sort((a, b) => {
          const nameCompare = getChannelLabel(a).localeCompare(
            getChannelLabel(b),
          );
          return nameCompare === 0 ? a.id - b.id : nameCompare;
        });
      setChannels(sortedChannels);
    } catch {
      setChannelsError(true);
      showError(t('获取渠道列表失败'));
    } finally {
      setChannelsLoading(false);
    }
  };

  const fetchGroups = async () => {
    setGroupsLoading(true);
    setGroupsError(false);
    try {
      const res = await API.get('/api/security-audit/builtin-policy/groups');
      const { success, message, data } = res.data;
      if (!success) {
        setGroupsError(true);
        showError(message);
        return;
      }
      const sortedGroups = [...(data || [])]
        .filter((group) => group?.code && Number(group?.id) > 0)
        .sort((a, b) =>
          String(a.name || a.code).localeCompare(String(b.name || b.code)),
        );
      setGroups(sortedGroups);
    } catch {
      setGroupsError(true);
      showError(t('获取分组列表失败'));
    } finally {
      setGroupsLoading(false);
    }
  };

  const updateRule = (id, patch) => {
    const currentRule = rules.find((rule) => rule.id === id);
    if (currentRule) {
      const nextRule = { ...currentRule, ...patch };
      setExpandedRuleIds((current) => expandInvalidRule(current, nextRule));
    }
    setRules((prev) =>
      prev.map((rule) => (rule.id === id ? { ...rule, ...patch } : rule)),
    );
  };

  const toggleRuleExpanded = (id) => {
    setExpandedRuleIds((prev) => toggleExpandedRuleId(prev, id));
  };

  const addRule = () => {
    const nextRule = createRule();
    setRules((prev) => [...prev, nextRule]);
    setExpandedRuleIds((prev) => {
      const next = new Set(prev);
      next.add(nextRule.id);
      return next;
    });
  };

  const removeRule = (id) => {
    setRules((prev) => prev.filter((item) => item.id !== id));
    setExpandedRuleIds((prev) => removeExpandedRuleId(prev, id));
  };

  const getRuleSummary = (rule) => {
    const actionLabel = rule.action === ACTION_BLOCK ? t('拦截') : t('脱敏');
    const scopeLabel =
      rule.scope === SCOPE_RESPONSE
        ? t('返回')
        : rule.scope === SCOPE_BOTH
          ? t('全部')
          : t('请求');
    const targetLabel =
      rule.targetType === TARGET_ALL
        ? t('全部渠道')
        : rule.targetType === TARGET_CHANNEL_TAGS
          ? `${t('历史渠道分组')} (${normalizeChannelTags(rule.channelTags).length})`
          : `${t('指定渠道')} (${normalizeChannelIds(rule.channelIds).length}) · ${t('指定分组')} (${normalizeGroupCodes(rule.groupCodes).length})`;
    return `${actionLabel} · ${scopeLabel} · ${targetLabel}`;
  };

  const resetFormState = (values) => {
    const rawInputs = {
      CheckSensitiveEnabled: false,
      CheckSensitiveOnPromptEnabled: false,
      SensitiveWords: '',
      SensitiveRules: '{"rules":[]}',
      SensitiveRuleChannelIds: '[]',
      ...values,
    };
    const nextChannelIds = parseChannelIds(rawInputs.SensitiveRuleChannelIds);
    const nextRules = parseRulesConfig(
      rawInputs.SensitiveRules,
      rawInputs.SensitiveWords,
      nextChannelIds,
    );
    const nextInputs = {
      ...rawInputs,
      SensitiveRules: serializeRules(nextRules),
      SensitiveRuleChannelIds: serializeChannelIds(nextChannelIds),
    };

    setInputs(nextInputs);
    setInputsRow({ ...nextInputs });
    setRules(nextRules);
    setExpandedRuleIds(getInitialExpandedRuleIds(nextRules));
    refForm.current?.setValues(nextInputs);
  };

  const onSubmit = async () => {
    if (hasInvalidTargets) {
      showWarning(t('启用的规则必须至少选择一个渠道或分组，或选择全部渠道'));
      return;
    }
    const submitInputs = {
      ...inputs,
      SensitiveRules: currentRulesValue,
      SensitiveRuleChannelIds: inputs.SensitiveRuleChannelIds,
    };
    if (!hasChanges) return showWarning(t('你似乎并没有修改什么'));
    if (typeof props.onSave !== 'function') return;

    setSaving(true);
    try {
      await props.onSave(submitInputs);
    } finally {
      setSaving(false);
    }
  };

  const onReset = () => {
    resetFormState(inputsRow);
    props.onResetExternal?.();
  };

  useEffect(() => {
    const currentInputs = {};
    for (let key in props.options) {
      if (Object.keys(inputs).includes(key)) {
        currentInputs[key] = props.options[key];
      }
    }
    resetFormState(currentInputs);
  }, [
    props.options.CheckSensitiveEnabled,
    props.options.CheckSensitiveOnPromptEnabled,
    props.options.SensitiveRuleChannelIds,
    props.options.SensitiveRules,
    props.options.SensitiveWords,
  ]);

  useEffect(() => {
    fetchChannels();
    fetchGroups();
  }, []);

  return (
    <>
      <Spin spinning={isSaving}>
        <Form
          values={inputs}
          getFormApi={(formAPI) => (refForm.current = formAPI)}
          style={{ marginBottom: 15 }}
        >
          <Form.Section text={t('屏蔽词过滤设置')}>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'CheckSensitiveEnabled'}
                  label={t('启用屏蔽词过滤功能')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) => {
                    setInputs({
                      ...inputs,
                      CheckSensitiveEnabled: value,
                    });
                  }}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'CheckSensitiveOnPromptEnabled'}
                  label={t('启用 Prompt 检查')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      CheckSensitiveOnPromptEnabled: value,
                    })
                  }
                />
              </Col>
            </Row>
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: 12,
              }}
            >
              <div>
                <Typography.Title heading={6} style={{ margin: 0 }}>
                  {t('过滤规则')}
                </Typography.Title>
                <Typography.Text type='tertiary'>
                  {t(
                    '每条规则可独立选择请求、返回或全部范围，并执行脱敏或拦截',
                  )}
                </Typography.Text>
              </div>
              <Button icon={<IconPlus />} onClick={addRule}>
                {t('新增规则')}
              </Button>
            </div>

            {rules.length === 0 ? (
              <Empty
                title={t('暂无屏蔽词规则')}
                description={t('新增规则后可配置关键词、脱敏或拦截')}
                style={{ padding: 24 }}
              />
            ) : (
              <Space
                vertical
                align='start'
                spacing='medium'
                style={{ width: '100%' }}
              >
                {rules.map((rule, index) => {
                  const invalidTarget = getEmptyRuleTarget(rule);
                  const isExpanded = expandedRuleIds.has(rule.id);
                  const panelId = `sensitive-rule-panel-${index}`;
                  const ruleLabel =
                    String(rule.name || '').trim() ||
                    t('规则 {{number}}', { number: index + 1 });

                  return (
                    <div
                      key={rule.id}
                      style={{
                        width: '100%',
                        border: '1px solid var(--semi-color-border)',
                        borderRadius: 8,
                        padding: 16,
                      }}
                    >
                      <Row gutter={12} type='flex' align='middle'>
                        <Col xs={24} sm={16} md={18} lg={18} xl={18}>
                          <button
                            type='button'
                            aria-expanded={isExpanded}
                            aria-controls={panelId}
                            onClick={() => toggleRuleExpanded(rule.id)}
                            style={{
                              width: '100%',
                              minHeight: 32,
                              display: 'flex',
                              alignItems: 'center',
                              gap: 8,
                              padding: 4,
                              border: 0,
                              borderRadius: 6,
                              background: 'transparent',
                              color: 'inherit',
                              cursor: 'pointer',
                              textAlign: 'left',
                            }}
                          >
                            {isExpanded ? (
                              <IconChevronDown aria-hidden='true' />
                            ) : (
                              <IconChevronRight aria-hidden='true' />
                            )}
                            <span
                              style={{
                                minWidth: 0,
                                flex: 1,
                                display: 'flex',
                                alignItems: 'center',
                                flexWrap: 'wrap',
                                gap: 8,
                                overflowWrap: 'anywhere',
                              }}
                            >
                              <Typography.Text strong>
                                {ruleLabel}
                              </Typography.Text>
                              <Tag color={rule.enabled ? 'green' : 'grey'}>
                                {rule.enabled ? t('已启用') : t('已禁用')}
                              </Tag>
                              {invalidTarget !== null ? (
                                <Tag color='red'>{t('错误')}</Tag>
                              ) : null}
                              <Typography.Text type='tertiary' size='small'>
                                {getRuleSummary(rule)}
                              </Typography.Text>
                            </span>
                          </button>
                        </Col>
                        <Col xs={24} sm={8} md={6} lg={6} xl={6}>
                          <Space style={{ float: 'right' }}>
                            <Switch
                              checked={rule.enabled}
                              checkedText='｜'
                              uncheckedText='〇'
                              aria-label={t('启用规则')}
                              onChange={(enabled) =>
                                updateRule(rule.id, { enabled })
                              }
                            />
                            <Button
                              type='danger'
                              theme='borderless'
                              icon={<IconDelete />}
                              onClick={() => removeRule(rule.id)}
                            >
                              {t('删除')}
                            </Button>
                          </Space>
                        </Col>
                      </Row>

                      {isExpanded ? (
                        <div
                          id={panelId}
                          role='region'
                          aria-label={ruleLabel}
                          style={{
                            marginTop: 12,
                            paddingTop: 12,
                            borderTop: '1px solid var(--semi-color-border)',
                          }}
                        >
                          <Row gutter={16}>
                            <Col xs={24} sm={12} md={8} lg={6} xl={6}>
                              <Typography.Text strong>
                                {t('规则名称')}
                              </Typography.Text>
                              <Input
                                value={rule.name}
                                placeholder={t('规则名称')}
                                style={{ marginTop: 8 }}
                                onChange={(value) =>
                                  updateRule(rule.id, { name: value })
                                }
                              />
                            </Col>
                            <Col xs={24} sm={12} md={8} lg={6} xl={6}>
                              <Typography.Text strong>
                                {t('处理动作')}
                              </Typography.Text>
                              <Select
                                value={rule.action}
                                style={{ width: '100%', marginTop: 8 }}
                                onChange={(value) =>
                                  updateRule(rule.id, {
                                    action:
                                      value === ACTION_BLOCK
                                        ? ACTION_BLOCK
                                        : ACTION_MASK,
                                    replacement:
                                      value === ACTION_MASK
                                        ? rule.replacement ||
                                          DEFAULT_REPLACEMENT
                                        : rule.replacement,
                                  })
                                }
                              >
                                <Select.Option value={ACTION_MASK}>
                                  {t('脱敏')}
                                </Select.Option>
                                <Select.Option value={ACTION_BLOCK}>
                                  {t('拦截')}
                                </Select.Option>
                              </Select>
                            </Col>
                            <Col xs={24} sm={12} md={8} lg={6} xl={6}>
                              <Typography.Text strong>
                                {t('应用范围')}
                              </Typography.Text>
                              <Select
                                value={rule.scope}
                                style={{ width: '100%', marginTop: 8 }}
                                onChange={(value) =>
                                  updateRule(rule.id, {
                                    scope:
                                      value === SCOPE_RESPONSE ||
                                      value === SCOPE_BOTH
                                        ? value
                                        : SCOPE_REQUEST,
                                  })
                                }
                              >
                                <Select.Option value={SCOPE_REQUEST}>
                                  {t('请求')}
                                </Select.Option>
                                <Select.Option value={SCOPE_RESPONSE}>
                                  {t('返回')}
                                </Select.Option>
                                <Select.Option value={SCOPE_BOTH}>
                                  {t('全部')}
                                </Select.Option>
                              </Select>
                            </Col>
                            {rule.action === ACTION_MASK ? (
                              <Col xs={24} sm={12} md={8} lg={6} xl={6}>
                                <Typography.Text strong>
                                  {t('替换文本')}
                                </Typography.Text>
                                <Input
                                  value={rule.replacement}
                                  placeholder={DEFAULT_REPLACEMENT}
                                  style={{ marginTop: 8 }}
                                  onChange={(value) =>
                                    updateRule(rule.id, { replacement: value })
                                  }
                                />
                              </Col>
                            ) : null}
                          </Row>

                          <Row gutter={16} style={{ marginTop: 12 }}>
                            <Col xs={24} sm={12} md={8} lg={6} xl={6}>
                              <Typography.Text strong>
                                {t('规则目标')}
                              </Typography.Text>
                              <div style={{ marginTop: 8 }}>
                                <RadioGroup
                                  type='button'
                                  size='small'
                                  value={rule.targetType}
                                  onChange={(event) =>
                                    updateRule(rule.id, {
                                      targetType: [
                                        TARGET_ROUTES,
                                        TARGET_ALL,
                                      ].includes(event.target.value)
                                        ? event.target.value
                                        : TARGET_ROUTES,
                                    })
                                  }
                                >
                                  <Radio value={TARGET_ROUTES}>
                                    {t('指定范围')}
                                  </Radio>
                                  <Radio value={TARGET_ALL}>
                                    {t('全部渠道')}
                                  </Radio>
                                </RadioGroup>
                              </div>
                            </Col>
                            <Col xs={24} sm={24} md={16} lg={14} xl={12}>
                              <Typography.Text strong>
                                {rule.targetType === TARGET_CHANNEL_TAGS
                                  ? t('历史渠道分组')
                                  : rule.targetType === TARGET_ALL
                                    ? t('全部渠道')
                                    : t('指定范围')}
                              </Typography.Text>
                              {rule.targetType === TARGET_ALL ? (
                                <Typography.Text type='tertiary'>
                                  {t('该规则对所有渠道生效')}
                                </Typography.Text>
                              ) : rule.targetType === TARGET_CHANNEL_TAGS ? (
                                <Typography.Text
                                  type='tertiary'
                                  style={{ display: 'block', marginTop: 8 }}
                                >
                                  {t(
                                    '该历史规则会保留原渠道分组范围，切换为指定范围后可重新选择渠道和业务分组',
                                  )}
                                </Typography.Text>
                              ) : (
                                <Row gutter={12} style={{ marginTop: 8 }}>
                                  <Col xs={24} lg={12}>
                                    <Typography.Text type='tertiary'>
                                      {t('指定渠道')}
                                    </Typography.Text>
                                    <Select
                                      multiple
                                      filter
                                      maxTagCount={1}
                                      ellipsisTrigger
                                      showRestTagsPopover
                                      loading={channelsLoading}
                                      disabled={channelsError}
                                      value={rule.channelIds || []}
                                      placeholder={t('指定渠道')}
                                      emptyContent={t('暂无渠道')}
                                      style={{ width: '100%', marginTop: 6 }}
                                      onChange={(value) =>
                                        updateRule(rule.id, {
                                          channelIds: normalizeChannelIds(
                                            Array.isArray(value) ? value : [],
                                          ),
                                        })
                                      }
                                    >
                                      {channels.map((channel) => (
                                        <Select.Option
                                          key={channel.id}
                                          value={channel.id}
                                        >
                                          {getChannelLabel(channel)}
                                        </Select.Option>
                                      ))}
                                      {normalizeChannelIds(rule.channelIds)
                                        .filter((id) => !channelIdSet.has(id))
                                        .map((id) => (
                                          <Select.Option
                                            key={`missing-${id}`}
                                            value={id}
                                          >
                                            {t('失效渠道')} #{id}
                                          </Select.Option>
                                        ))}
                                    </Select>
                                  </Col>
                                  <Col xs={24} lg={12}>
                                    <Typography.Text type='tertiary'>
                                      {t('指定分组')}
                                    </Typography.Text>
                                    <Select
                                      multiple
                                      filter
                                      maxTagCount={1}
                                      ellipsisTrigger
                                      showRestTagsPopover
                                      loading={groupsLoading}
                                      disabled={groupsError}
                                      value={rule.groupCodes || []}
                                      placeholder={t('指定分组')}
                                      emptyContent={t('暂无分组')}
                                      style={{ width: '100%', marginTop: 6 }}
                                      onChange={(value) =>
                                        updateRule(rule.id, {
                                          groupCodes: normalizeGroupCodes(
                                            Array.isArray(value) ? value : [],
                                          ),
                                        })
                                      }
                                    >
                                      {groups.map((group) => (
                                        <Select.Option
                                          key={group.code}
                                          value={group.code}
                                        >
                                          {group.name || group.code} #{group.id}
                                        </Select.Option>
                                      ))}
                                      {normalizeGroupCodes(rule.groupCodes)
                                        .filter(
                                          (code) => !groupCodeSet.has(code),
                                        )
                                        .map((code) => (
                                          <Select.Option
                                            key={`missing-${code}`}
                                            value={code}
                                          >
                                            {t('失效分组')}: {code}
                                          </Select.Option>
                                        ))}
                                    </Select>
                                  </Col>
                                </Row>
                              )}
                              {rule.targetType === TARGET_ROUTES &&
                              groupsError ? (
                                <Space wrap style={{ marginTop: 6 }}>
                                  <Typography.Text type='danger' size='small'>
                                    {t('获取分组列表失败')}
                                  </Typography.Text>
                                  <Button
                                    type='tertiary'
                                    theme='borderless'
                                    size='small'
                                    icon={<IconRefresh />}
                                    onClick={fetchGroups}
                                  >
                                    {t('重试')}
                                  </Button>
                                </Space>
                              ) : null}
                              {rule.targetType === TARGET_ROUTES &&
                              channelsError ? (
                                <Space wrap style={{ marginTop: 6 }}>
                                  <Typography.Text type='danger' size='small'>
                                    {t('获取渠道列表失败')}
                                  </Typography.Text>
                                  <Button
                                    type='tertiary'
                                    theme='borderless'
                                    size='small'
                                    icon={<IconRefresh />}
                                    onClick={fetchChannels}
                                  >
                                    {t('重试')}
                                  </Button>
                                </Space>
                              ) : null}
                              {invalidTarget !== null ? (
                                <div
                                  style={{
                                    marginTop: 6,
                                    color: 'var(--semi-color-danger)',
                                  }}
                                >
                                  {t(
                                    '启用的规则必须至少选择一个渠道或分组，或选择全部渠道',
                                  )}
                                </div>
                              ) : null}
                            </Col>
                          </Row>

                          <Row style={{ marginTop: 12 }}>
                            <Col xs={24} sm={24} md={16} lg={14} xl={12}>
                              <Typography.Text strong>
                                {t('关键词')}
                              </Typography.Text>
                              <TextArea
                                value={rule.keywordsText}
                                placeholder={t('一行一个关键词')}
                                autosize={{ minRows: 4, maxRows: 8 }}
                                style={{
                                  marginTop: 8,
                                  fontFamily: 'JetBrains Mono, Consolas',
                                }}
                                onChange={(value) =>
                                  updateRule(rule.id, { keywordsText: value })
                                }
                              />
                              <div
                                style={{
                                  marginTop: 6,
                                  color: 'var(--semi-color-text-2)',
                                }}
                              >
                                {t('空行和重复关键词会被自动忽略')}
                                {'；'}
                                {t(
                                  '英文关键词按完整单词边界匹配，中文关键词仍按连续子串匹配',
                                )}
                              </div>
                            </Col>
                          </Row>
                        </div>
                      ) : null}
                    </div>
                  );
                })}
              </Space>
            )}
            <Row type='flex' justify='end'>
              <Space style={{ marginTop: 16 }}>
                <Button
                  size='default'
                  disabled={!hasChanges || isSaving}
                  onClick={onReset}
                >
                  {t('重置')}
                </Button>
                <Button
                  type='primary'
                  size='default'
                  loading={isSaving}
                  disabled={!hasChanges || isSaving || hasInvalidTargets}
                  onClick={() => void onSubmit()}
                >
                  {t('保存屏蔽词过滤设置')}
                </Button>
              </Space>
            </Row>
          </Form.Section>
        </Form>
      </Spin>
    </>
  );
}
