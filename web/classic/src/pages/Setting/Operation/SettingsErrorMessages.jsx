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

import React, { useEffect, useRef, useState } from 'react';
import {
  IconChevronDown,
  IconChevronRight,
  IconDelete,
  IconPlus,
} from '@douyinfe/semi-icons';
import {
  Banner,
  Button,
  Form,
  InputNumber,
  Popconfirm,
  Select,
  Space,
  Spin,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';
import {
  createErrorMessageReplacementRule,
  MAX_ERROR_MESSAGE_MATCHES_PER_RULE,
  MAX_ERROR_MESSAGE_REPLACEMENT_RULES,
  parseErrorMessageReplacementRules,
  serializeErrorMessageReplacementRules,
  validateErrorMessageReplacementRules,
} from './error-message-rules';

const { Text } = Typography;

const modes = [
  { value: 'contains', label: '包含' },
  { value: 'exact', label: '精确匹配' },
  { value: 'regex', label: '正则表达式' },
];

const normalizeStatusCode = (value) => {
  if (value === '' || value === null || value === undefined) return undefined;
  return Number(value);
};

export default function SettingsErrorMessages(props) {
  const { t } = useTranslation();
  const [rules, setRules] = useState([]);
  const [expandedRules, setExpandedRules] = useState({});
  const [loading, setLoading] = useState(false);
  const matchInputRefs = useRef({});
  const pendingMatchFocus = useRef(null);

  useEffect(() => {
    setRules(
      parseErrorMessageReplacementRules(
        props.options.ErrorMessageReplacementRules,
      ),
    );
    setExpandedRules({});
  }, [props.options.ErrorMessageReplacementRules]);

  const updateRule = (index, changes) => {
    setRules((current) =>
      current.map((rule, currentIndex) =>
        currentIndex === index ? { ...rule, ...changes } : rule,
      ),
    );
  };

  const updateMatch = (ruleIndex, matchIndex, value) => {
    const matches = [...rules[ruleIndex].matches];
    matches[matchIndex] = value;
    updateRule(ruleIndex, { matches });
  };

  const addMatch = (ruleIndex, afterIndex) => {
    const matches = [...rules[ruleIndex].matches];
    if (matches.length >= MAX_ERROR_MESSAGE_MATCHES_PER_RULE) return;
    const insertAt = afterIndex === undefined ? matches.length : afterIndex + 1;
    matches.splice(insertAt, 0, '');
    pendingMatchFocus.current = `${ruleIndex}-${insertAt}`;
    updateRule(ruleIndex, { matches });
  };

  useEffect(() => {
    if (pendingMatchFocus.current === null) return;
    const input = matchInputRefs.current[pendingMatchFocus.current];
    pendingMatchFocus.current = null;
    input?.focus();
  }, [rules]);

  const removeMatch = (ruleIndex, matchIndex) => {
    const matches = rules[ruleIndex].matches.filter(
      (_, currentIndex) => currentIndex !== matchIndex,
    );
    updateRule(ruleIndex, { matches: matches.length > 0 ? matches : [''] });
  };

  const removeRule = (ruleIndex) => {
    setRules((current) =>
      current.filter((_, currentIndex) => currentIndex !== ruleIndex),
    );
    setExpandedRules({});
  };

  const addRule = () => {
    const nextIndex = rules.length;
    setRules((current) => [...current, createErrorMessageReplacementRule()]);
    setExpandedRules((current) => ({ ...current, [nextIndex]: true }));
  };

  const save = async () => {
    if (!validateErrorMessageReplacementRules(rules)) {
      showError(
        t(
          '每条规则至少需要一个非空匹配值和替换文案，原状态码必须在 100 到 599 之间，替换状态码必须在 400 到 599 之间',
        ),
      );
      return;
    }
    const serialized = serializeErrorMessageReplacementRules(rules);
    setLoading(true);
    try {
      const response = await API.put('/api/option/', {
        key: 'ErrorMessageReplacementRules',
        value: serialized,
      });
      if (!response.data.success) {
        showError(response.data.message || t('保存失败，请重试'));
        return;
      }
      setRules(parseErrorMessageReplacementRules(serialized));
      showSuccess(t('保存成功'));
      props.refresh();
    } catch (error) {
      showError(error.message || t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Spin spinning={loading}>
      <Form.Section text={t('客户端错误消息')}>
        <Banner
          type='info'
          description={t(
            '规则按顺序匹配，同一规则内的匹配值使用或关系，可选原状态码与匹配值使用且关系，并可同时替换最终返回给客户端的状态码和提示词。重试、渠道禁用、安全审计和内部指标仍使用上游原始错误；错误日志默认显示替换后文案，展开详情保留上游原文。'
          )}
          style={{ marginBottom: 16 }}
        />
        <Space
          vertical
          align='start'
          spacing='medium'
          style={{ width: '100%' }}
        >
          {rules.map((rule, index) => (
            <div
              key={index}
              style={{
                width: '100%',
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
                }}
              >
                <Button
                  theme='borderless'
                  type='tertiary'
                  icon={
                    expandedRules[index] ? (
                      <IconChevronDown />
                    ) : (
                      <IconChevronRight />
                    )
                  }
                  aria-expanded={expandedRules[index] === true}
                  aria-controls={`error-message-rule-${index}`}
                  onClick={() =>
                    setExpandedRules((current) => ({
                      ...current,
                      [index]: !current[index],
                    }))
                  }
                  style={{
                    minWidth: 0,
                    flex: 1,
                    justifyContent: 'flex-start',
                    padding: 0,
                  }}
                >
                  <Text strong>
                    {t('规则 {{number}}', { number: index + 1 })}
                  </Text>
                  <Text type='tertiary' size='small' style={{ marginLeft: 8 }}>
                    {t(
                      modes.find((mode) => mode.value === rule.mode)?.label ||
                        '匹配方式',
                    )}{' '}
                    ·{' '}
                    {t('{{count}} / {{max}} 个匹配值', {
                      count: rule.matches.length,
                      max: MAX_ERROR_MESSAGE_MATCHES_PER_RULE,
                    })}
                  </Text>
                </Button>
                <Popconfirm
                  title={t('确认删除该规则？')}
                  onConfirm={() => removeRule(index)}
                >
                  <Button
                    type='danger'
                    theme='borderless'
                    icon={<IconDelete />}
                    aria-label={t('删除规则')}
                  />
                </Popconfirm>
              </div>

              {expandedRules[index] && (
                <div
                  id={`error-message-rule-${index}`}
                  style={{ display: 'grid', gap: 12, marginTop: 12 }}
                >
                  <div
                    style={{
                      display: 'grid',
                      gridTemplateColumns:
                        'repeat(auto-fit, minmax(180px, 1fr))',
                      gap: 12,
                    }}
                  >
                    <label>
                      <Text strong size='small' style={{ display: 'block' }}>
                        {t('原状态码（可选）')}
                      </Text>
                      <InputNumber
                        value={rule.status_code}
                        min={100}
                        max={599}
                        precision={0}
                        placeholder={t('例如 403')}
                        style={{ width: '100%', marginTop: 6 }}
                        onChange={(value) =>
                          updateRule(index, {
                            status_code: normalizeStatusCode(value),
                          })
                        }
                      />
                    </label>
                    <label>
                      <Text strong size='small' style={{ display: 'block' }}>
                        {t('匹配模式')}
                      </Text>
                      <Select
                        value={rule.mode}
                        optionList={modes.map((mode) => ({
                          ...mode,
                          label: t(mode.label),
                        }))}
                        style={{ width: '100%', marginTop: 6 }}
                        onChange={(value) => updateRule(index, { mode: value })}
                      />
                    </label>
                    <label>
                      <Text strong size='small' style={{ display: 'block' }}>
                        {t('新状态码（可选）')}
                      </Text>
                      <InputNumber
                        value={rule.replace_status_code}
                        min={400}
                        max={599}
                        precision={0}
                        placeholder={t('例如 429')}
                        style={{ width: '100%', marginTop: 6 }}
                        onChange={(value) =>
                          updateRule(index, {
                            replace_status_code: normalizeStatusCode(value),
                          })
                        }
                      />
                    </label>
                  </div>

                  <div>
                    <Text strong size='small' style={{ display: 'block' }}>
                      {t('匹配值')}
                    </Text>
                    {rule.matches.map((match, matchIndex) => (
                      <div
                        key={matchIndex}
                        style={{
                          display: 'flex',
                          alignItems: 'flex-start',
                          gap: 8,
                          marginTop: 6,
                        }}
                      >
                        <TextArea
                          ref={(element) => {
                            matchInputRefs.current[`${index}-${matchIndex}`] =
                              element;
                          }}
                          value={match}
                          maxLength={4096}
                          autosize={{ minRows: 2, maxRows: 6 }}
                          placeholder={t('原始错误消息中的文本')}
                          onChange={(value) =>
                            updateMatch(index, matchIndex, value)
                          }
                          onKeyDown={(event) => {
                            if (
                              event.key === 'Enter' &&
                              !event.shiftKey &&
                              match.trim()
                            ) {
                              event.preventDefault();
                              addMatch(index, matchIndex);
                            }
                          }}
                        />
                        <Button
                          type='danger'
                          theme='borderless'
                          icon={<IconDelete />}
                          aria-label={t('删除匹配值')}
                          onClick={() => removeMatch(index, matchIndex)}
                        />
                      </div>
                    ))}
                    <div
                      style={{
                        display: 'flex',
                        flexWrap: 'wrap',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        gap: 8,
                        marginTop: 6,
                      }}
                    >
                      <Text type='tertiary' size='small'>
                        {t('{{count}} / {{max}} 个匹配值', {
                          count: rule.matches.length,
                          max: MAX_ERROR_MESSAGE_MATCHES_PER_RULE,
                        })}
                      </Text>
                      <Button
                        theme='outline'
                        icon={<IconPlus />}
                        disabled={
                          rule.matches.length >=
                          MAX_ERROR_MESSAGE_MATCHES_PER_RULE
                        }
                        onClick={() => addMatch(index)}
                      >
                        {t('添加匹配值')}
                      </Button>
                    </div>
                  </div>

                  <label>
                    <Text strong size='small' style={{ display: 'block' }}>
                      {t('替换为')}
                    </Text>
                    <TextArea
                      value={rule.replace}
                      maxLength={4096}
                      autosize={{ minRows: 2, maxRows: 6 }}
                      placeholder={t('返回给客户端的消息')}
                      style={{ marginTop: 6 }}
                      onChange={(value) =>
                        updateRule(index, { replace: value })
                      }
                    />
                  </label>
                </div>
              )}
            </div>
          ))}
          {rules.length === 0 && (
            <div style={{ width: '100%', padding: 24, textAlign: 'center' }}>
              {t('暂无替换规则')}
            </div>
          )}
          <Space>
            <Button
              icon={<IconPlus />}
              disabled={rules.length >= MAX_ERROR_MESSAGE_REPLACEMENT_RULES}
              onClick={addRule}
            >
              {t('添加规则')}
            </Button>
            <Button type='primary' onClick={save}>
              {t('保存错误消息规则')}
            </Button>
          </Space>
        </Space>
      </Form.Section>
    </Spin>
  );
}
