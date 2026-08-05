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

import React, { useEffect, useState } from 'react';
import { IconDelete, IconPlus } from '@douyinfe/semi-icons';
import {
  Banner,
  Button,
  Form,
  Select,
  Space,
  Spin,
  TextArea,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';

const modes = [
  { value: 'contains', label: '包含' },
  { value: 'exact', label: '精确匹配' },
  { value: 'regex', label: '正则表达式' },
];

function parseRules(raw) {
  try {
    const value = JSON.parse(raw || '[]');
    if (!Array.isArray(value)) return [];
    return value
      .filter(
        (rule) =>
          rule &&
          typeof rule.match === 'string' &&
          typeof rule.replace === 'string' &&
          modes.some((mode) => mode.value === rule.mode),
      )
      .slice(0, 100)
      .map((rule) => ({
        match: rule.match,
        mode: rule.mode,
        replace: rule.replace,
      }));
  } catch {
    return [];
  }
}

export default function SettingsErrorMessages(props) {
  const { t } = useTranslation();
  const [rules, setRules] = useState([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    setRules(parseRules(props.options.ErrorMessageReplacementRules));
  }, [props.options.ErrorMessageReplacementRules]);

  const updateRule = (index, changes) => {
    setRules((current) =>
      current.map((rule, currentIndex) =>
        currentIndex === index ? { ...rule, ...changes } : rule,
      ),
    );
  };

  const save = async () => {
    const normalized = rules.map((rule) => ({
      match: rule.match.trim(),
      mode: rule.mode,
      replace: rule.replace.trim(),
    }));
    if (
      normalized.length > 100 ||
      normalized.some((rule) => !rule.match || !rule.replace)
    ) {
      showError(t('每条规则都必须填写匹配内容和替换文案'));
      return;
    }
    setLoading(true);
    try {
      const response = await API.put('/api/option/', {
        key: 'ErrorMessageReplacementRules',
        value: JSON.stringify(normalized),
      });
      if (!response.data.success) {
        showError(response.data.message || t('保存失败，请重试'));
        return;
      }
      setRules(normalized);
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
            '规则按顺序匹配，只修改最终返回给客户端的消息。上游原始错误仍用于重试、渠道禁用和安全审计。',
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
                display: 'flex',
                flexWrap: 'wrap',
                gap: 12,
                width: '100%',
                alignItems: 'end',
              }}
            >
              <div style={{ flex: '1 1 260px', minWidth: 0 }}>
                <div style={{ marginBottom: 6 }}>{t('匹配')}</div>
                <TextArea
                  value={rule.match}
                  maxCount={4096}
                  autosize={{ minRows: 2, maxRows: 6 }}
                  placeholder={t('原始错误消息中的文本')}
                  onChange={(value) => updateRule(index, { match: value })}
                />
              </div>
              <div style={{ flex: '0 1 160px', minWidth: 140 }}>
                <div style={{ marginBottom: 6 }}>{t('匹配模式')}</div>
                <Select
                  value={rule.mode}
                  optionList={modes.map((mode) => ({
                    ...mode,
                    label: t(mode.label),
                  }))}
                  style={{ width: '100%' }}
                  onChange={(value) => updateRule(index, { mode: value })}
                />
              </div>
              <div style={{ flex: '1 1 260px', minWidth: 0 }}>
                <div style={{ marginBottom: 6 }}>{t('替换为')}</div>
                <TextArea
                  value={rule.replace}
                  maxCount={4096}
                  autosize={{ minRows: 2, maxRows: 6 }}
                  placeholder={t('返回给客户端的消息')}
                  onChange={(value) => updateRule(index, { replace: value })}
                />
              </div>
              <Button
                type='danger'
                theme='borderless'
                icon={<IconDelete />}
                aria-label={t('删除规则')}
                onClick={() =>
                  setRules((current) =>
                    current.filter((_, currentIndex) => currentIndex !== index),
                  )
                }
              />
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
              disabled={rules.length >= 100}
              onClick={() =>
                setRules((current) => [
                  ...current,
                  { match: '', mode: 'contains', replace: '' },
                ])
              }
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
