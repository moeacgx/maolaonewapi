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

import React, { useMemo, useState } from 'react';
import {
  Button,
  Input,
  Popconfirm,
  Select,
  Switch,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IconChevronDown,
  IconChevronRight,
  IconDelete,
  IconPlus,
} from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

const FAILURE_FILTER_FIELDS = [
  { value: 'status_code', label: '状态码' },
  { value: 'error_code', label: '错误码' },
  { value: 'message', label: '错误正文' },
  { value: 'full_error', label: '完整错误响应' },
];

const FAILURE_FILTER_MODES = [
  { value: 'contains', label: '包含' },
  { value: 'exact', label: '精确匹配' },
  { value: 'regex', label: '正则表达式' },
];

const FAILURE_FILTER_FIELD_VALUES = new Set(
  FAILURE_FILTER_FIELDS.map((item) => item.value),
);
const FAILURE_FILTER_MODE_VALUES = new Set(
  FAILURE_FILTER_MODES.map((item) => item.value),
);
const FAILURE_FILTER_ID_PATTERN = /^[A-Za-z0-9._-]{1,64}$/;
const MAX_FAILURE_FILTER_VALUES = 64;
const MAX_FAILURE_FILTER_VALUE_LENGTH = 4096;

function parseRules(rawValue) {
  try {
    const rules = JSON.parse(typeof rawValue === 'string' ? rawValue : '[]');
    return Array.isArray(rules)
      ? rules.map((rule) => ({
          ...rule,
          values: Array.isArray(rule?.values)
            ? rule.values
            : typeof rule?.value === 'string'
              ? [rule.value]
              : [],
        }))
      : [];
  } catch {
    return [];
  }
}

function createRule() {
  const randomPart =
    typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
      ? crypto.randomUUID()
      : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;

  return {
    id: `failure-filter-${randomPart}`.slice(0, 64),
    name: '',
    enabled: true,
    field: 'status_code',
    mode: 'exact',
    values: [],
  };
}

export function getFailureFilterRulesValidationError(rawValue) {
  let rules;
  try {
    rules = JSON.parse(rawValue);
  } catch {
    return { key: '模型广场失败过滤规则配置无效' };
  }

  if (!Array.isArray(rules)) {
    return { key: '模型广场失败过滤规则配置无效' };
  }
  if (rules.length > 100) {
    return { key: '最多只能添加 100 条失败排除规则' };
  }

  const ids = new Set();
  for (let index = 0; index < rules.length; index += 1) {
    const rule = rules[index];
    const id = typeof rule?.id === 'string' ? rule.id.trim() : '';
    const name = typeof rule?.name === 'string' ? rule.name.trim() : '';
    const values = Array.isArray(rule?.values)
      ? rule.values
      : typeof rule?.value === 'string'
        ? [rule.value]
        : [];

    if (
      !name ||
      values.length === 0 ||
      values.length > MAX_FAILURE_FILTER_VALUES
    ) {
      return {
        key: '第 {{number}} 条失败排除规则不完整，请填写规则名称和匹配值',
        options: { number: index + 1 },
      };
    }
    if (
      !FAILURE_FILTER_ID_PATTERN.test(id) ||
      ids.has(id) ||
      typeof rule.enabled !== 'boolean' ||
      !FAILURE_FILTER_FIELD_VALUES.has(rule.field) ||
      !FAILURE_FILTER_MODE_VALUES.has(rule.mode) ||
      Array.from(name).length > 128 ||
      values.some(
        (value) =>
          typeof value !== 'string' ||
          !value.trim() ||
          Array.from(value).length > MAX_FAILURE_FILTER_VALUE_LENGTH,
      )
    ) {
      return {
        key: '第 {{number}} 条失败排除规则配置无效',
        options: { number: index + 1 },
      };
    }
    ids.add(id);
  }

  return null;
}

export default function FailureFilterRulesEditor({ value, onChange }) {
  const { t } = useTranslation();
  const rules = useMemo(() => parseRules(value), [value]);
  const [drafts, setDrafts] = useState({});
  const [expandedRules, setExpandedRules] = useState({});

  const toggleRule = (ruleId) => {
    setExpandedRules((current) => ({
      ...current,
      [ruleId]: !current[ruleId],
    }));
  };

  const emitChange = (nextRules) => {
    onChange(
      JSON.stringify(
        nextRules.map((rule) => ({
          ...rule,
          value: rule.values?.[0] || rule.value || '',
        })),
      ),
    );
  };

  const addRule = () => {
    const rule = createRule();
    emitChange([...rules, rule]);
    setExpandedRules((current) => ({ ...current, [rule.id]: true }));
  };

  const updateRule = (index, patch) => {
    emitChange(
      rules.map((rule, currentIndex) =>
        currentIndex === index ? { ...rule, ...patch } : rule,
      ),
    );
  };

  const addDraftValue = (index) => {
    const rule = rules[index];
    const draft = drafts[rule.id] || '';
    if (!draft.trim() || rule.values.length >= MAX_FAILURE_FILTER_VALUES) {
      return;
    }
    updateRule(index, { values: [...rule.values, draft] });
    setDrafts((current) => ({ ...current, [rule.id]: '' }));
  };

  const updateValue = (index, valueIndex, nextValue) => {
    const values = [...rules[index].values];
    values[valueIndex] = nextValue;
    updateRule(index, { values });
  };

  return (
    <div>
      <div
        style={{
          display: 'flex',
          flexWrap: 'wrap',
          alignItems: 'flex-start',
          justifyContent: 'space-between',
          gap: 12,
          marginBottom: 12,
        }}
      >
        <div style={{ minWidth: 0, flex: 1 }}>
          <Text strong>{t('失败排除规则')}</Text>
          <Text
            type='tertiary'
            size='small'
            style={{ display: 'block', marginTop: 4 }}
          >
            {t(
              '命中任意一条启用规则的响应不会计入模型广场连接失败，原始错误与审计记录仍会保留。',
            )}
          </Text>
        </div>
        <Button
          type='primary'
          theme='outline'
          icon={<IconPlus />}
          disabled={rules.length >= 100}
          onClick={addRule}
        >
          {t('添加过滤规则')}
        </Button>
      </div>

      {rules.length === 0 ? (
        <div
          style={{
            border: '1px dashed var(--semi-color-border)',
            borderRadius: 6,
            padding: 16,
            textAlign: 'center',
          }}
        >
          <Text type='tertiary'>{t('暂未配置失败排除规则。')}</Text>
        </div>
      ) : (
        <div style={{ display: 'grid', gap: 12 }}>
          {rules.map((rule, index) => (
            <div
              key={`${rule.id || 'rule'}-${index}`}
              style={{
                border: '1px solid var(--semi-color-border)',
                borderRadius: 6,
                padding: 12,
              }}
            >
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  gap: 12,
                  marginBottom: 12,
                }}
              >
                <Button
                  theme='borderless'
                  type='tertiary'
                  icon={
                    expandedRules[rule.id] ? (
                      <IconChevronDown />
                    ) : (
                      <IconChevronRight />
                    )
                  }
                  aria-expanded={expandedRules[rule.id] === true}
                  aria-controls={`failure-filter-rule-${index}`}
                  onClick={() => toggleRule(rule.id)}
                  style={{
                    minWidth: 0,
                    flex: 1,
                    justifyContent: 'flex-start',
                    padding: 0,
                  }}
                >
                  <span style={{ minWidth: 0, overflow: 'hidden' }}>
                    <Text strong ellipsis={{ showTooltip: true }}>
                      {rule.name || t('规则 {{number}}', { number: index + 1 })}
                    </Text>
                    <Text
                      type='tertiary'
                      size='small'
                      ellipsis={{ showTooltip: true }}
                      style={{ marginLeft: 8 }}
                    >
                      {t(
                        FAILURE_FILTER_FIELDS.find(
                          (item) => item.value === rule.field,
                        )?.label || '匹配字段',
                      )}{' '}
                      ·{' '}
                      {t(
                        FAILURE_FILTER_MODES.find(
                          (item) => item.value === rule.mode,
                        )?.label || '匹配方式',
                      )}{' '}
                      ·{' '}
                      {t('{{count}} / {{max}} 个匹配值', {
                        count: rule.values.length,
                        max: MAX_FAILURE_FILTER_VALUES,
                      })}
                    </Text>
                  </span>
                </Button>
                <div
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 8,
                    flexShrink: 0,
                  }}
                >
                  <Switch
                    checked={rule.enabled}
                    aria-label={t('启用规则')}
                    onChange={(enabled) => updateRule(index, { enabled })}
                  />
                  <Popconfirm
                    title={t('确认删除该规则？')}
                    onConfirm={() =>
                      emitChange(
                        rules.filter(
                          (_, currentIndex) => currentIndex !== index,
                        ),
                      )
                    }
                  >
                    <Button
                      type='danger'
                      theme='borderless'
                      icon={<IconDelete />}
                      aria-label={t('删除规则')}
                    />
                  </Popconfirm>
                </div>
              </div>

              {expandedRules[rule.id] && (
                <div id={`failure-filter-rule-${index}`}>
                  <div
                    style={{
                      display: 'grid',
                      gridTemplateColumns:
                        'repeat(auto-fit, minmax(180px, 1fr))',
                      gap: 12,
                      marginBottom: 12,
                    }}
                  >
                    <label>
                      <Text strong size='small' style={{ display: 'block' }}>
                        {t('规则名称')}
                      </Text>
                      <Input
                        value={rule.name}
                        maxLength={128}
                        placeholder={t('例如：OpenAI 内容政策')}
                        style={{ marginTop: 6 }}
                        onChange={(name) => updateRule(index, { name })}
                      />
                    </label>
                    <label>
                      <Text strong size='small' style={{ display: 'block' }}>
                        {t('匹配字段')}
                      </Text>
                      <Select
                        value={rule.field}
                        optionList={FAILURE_FILTER_FIELDS.map((item) => ({
                          ...item,
                          label: t(item.label),
                        }))}
                        style={{ width: '100%', marginTop: 6 }}
                        onChange={(field) => updateRule(index, { field })}
                      />
                    </label>
                    <label>
                      <Text strong size='small' style={{ display: 'block' }}>
                        {t('匹配方式')}
                      </Text>
                      <Select
                        value={rule.mode}
                        optionList={FAILURE_FILTER_MODES.map((item) => ({
                          ...item,
                          label: t(item.label),
                        }))}
                        style={{ width: '100%', marginTop: 6 }}
                        onChange={(mode) => updateRule(index, { mode })}
                      />
                    </label>
                  </div>

                  <div>
                    <Text strong size='small' style={{ display: 'block' }}>
                      {t('匹配值')}
                    </Text>
                    {rule.values.map((matchValue, valueIndex) => (
                      <div
                        key={`${rule.id}-${valueIndex}`}
                        style={{
                          display: 'flex',
                          alignItems: 'flex-start',
                          gap: 8,
                          marginTop: 6,
                        }}
                      >
                        <TextArea
                          value={matchValue}
                          maxLength={MAX_FAILURE_FILTER_VALUE_LENGTH}
                          rows={rule.mode === 'exact' ? 3 : 2}
                          style={{ fontFamily: 'monospace' }}
                          onChange={(nextValue) =>
                            updateValue(index, valueIndex, nextValue)
                          }
                        />
                        <Button
                          type='danger'
                          theme='borderless'
                          icon={<IconDelete />}
                          aria-label={t('删除匹配值')}
                          onClick={() =>
                            updateRule(index, {
                              values: rule.values.filter(
                                (_, currentIndex) =>
                                  currentIndex !== valueIndex,
                              ),
                            })
                          }
                        />
                      </div>
                    ))}
                    <div
                      style={{
                        display: 'flex',
                        alignItems: 'flex-start',
                        gap: 8,
                        marginTop: 6,
                      }}
                    >
                      <TextArea
                        value={drafts[rule.id] || ''}
                        maxLength={MAX_FAILURE_FILTER_VALUE_LENGTH}
                        rows={rule.mode === 'exact' ? 3 : 2}
                        placeholder={t(
                          '填写匹配值，回车添加；Shift+Enter 换行',
                        )}
                        style={{ fontFamily: 'monospace' }}
                        onChange={(nextValue) =>
                          setDrafts((current) => ({
                            ...current,
                            [rule.id]: nextValue,
                          }))
                        }
                        onKeyDown={(event) => {
                          if (event.key === 'Enter' && !event.shiftKey) {
                            event.preventDefault();
                            addDraftValue(index);
                          }
                        }}
                      />
                      <Button
                        theme='outline'
                        icon={<IconPlus />}
                        aria-label={t('添加匹配值')}
                        disabled={
                          rule.values.length >= MAX_FAILURE_FILTER_VALUES
                        }
                        onClick={() => addDraftValue(index)}
                      />
                    </div>
                    <Text
                      type='tertiary'
                      size='small'
                      style={{ display: 'block', marginTop: 4 }}
                    >
                      {t('{{count}} / {{max}} 个匹配值', {
                        count: rule.values.length,
                        max: MAX_FAILURE_FILTER_VALUES,
                      })}
                    </Text>
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
