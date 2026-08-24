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

import { IconDelete, IconPlus } from '@douyinfe/semi-icons';
import {
  Banner,
  Button,
  Input,
  InputNumber,
  Popconfirm,
  Select,
  Spin,
  Switch,
  Typography,
} from '@douyinfe/semi-ui';
import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { CHANNEL_OPTIONS } from '../../constants';
import { API, showError, showSuccess, toBoolean } from '../../helpers';
import { extractGroupDetailsResponse } from '../../helpers/groupDetails';
import {
  addDynamicRoutingConfiguredGroupOption,
  createDynamicRoutingRule,
  createDynamicRoutingRuleFromPreset,
  DYNAMIC_ROUTING_ACTION_MODEL_REDIRECT,
  DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_FUNCTION_BRIDGE,
  DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE,
  DYNAMIC_ROUTING_IMAGE_TARGET_PATHS,
  DYNAMIC_ROUTING_IMAGE_GENERATION_PATH,
  DYNAMIC_ROUTING_PRESETS,
  MAX_DYNAMIC_ROUTING_CONDITIONS,
  MAX_DYNAMIC_ROUTING_RULES,
  normalizeDynamicRoutingGroupOptions,
  parseDynamicRoutingRules,
  validateDynamicRoutingRules,
} from './dynamic-routing-rules';

const { Text } = Typography;

const REQUEST_PATH_OPTIONS = [
  '/v1/chat/completions',
  '/v1/responses',
  '/v1/messages',
  '/v1/images/generations',
  '/v1/images/edits',
  '/v1/images/tasks',
];

const OPERATOR_OPTIONS = [
  { value: 'equals', label: '等于' },
  { value: 'not_equals', label: '不等于' },
  { value: 'exists', label: '存在' },
  { value: 'not_exists', label: '不存在' },
];

const ACTION_OPTIONS = [
  { value: DYNAMIC_ROUTING_ACTION_MODEL_REDIRECT, label: '模型重定向' },
  {
    value: DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE,
    label: 'Responses 图片工具桥接',
  },
  {
    value: DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_FUNCTION_BRIDGE,
    label: 'Text function call to Images API',
  },
];

const formGridStyle = {
  display: 'grid',
  gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))',
  gap: 12,
};

const INHERIT_TARGET_GROUP_VALUE = '__dynamic_routing_inherit_group__';

function DynamicRoutingRuleEditor(props) {
  const { t } = useTranslation();
  const conditions = Array.isArray(props.rule.conditions)
    ? props.rule.conditions
    : [];
  const action = props.rule.action || DYNAMIC_ROUTING_ACTION_MODEL_REDIRECT;
  const isImageToolBridge =
    action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE;
  const isImageFunctionBridge =
    action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_FUNCTION_BRIDGE;
  const isImageBridge = isImageToolBridge || isImageFunctionBridge;
  const sourceModelListId = `dynamic-routing-${props.index}-source-model-options`;
  const targetModelListId = `dynamic-routing-${props.index}-target-model-options`;
  const configuredTargetGroup = String(props.rule.target_group || '').trim();
  const targetGroupOptions = addDynamicRoutingConfiguredGroupOption(
    props.targetGroupOptions,
    configuredTargetGroup,
    t('未知的已配置分组'),
  );
  const configuredSourceGroups = [
    ...new Set(
      (Array.isArray(props.rule.source_groups) ? props.rule.source_groups : [])
        .map((group) => String(group).trim())
        .filter(Boolean),
    ),
  ];
  const sourceGroupOptions = addDynamicRoutingConfiguredGroupOption(
    targetGroupOptions,
    configuredSourceGroups.find(
      (group) => !targetGroupOptions.some((option) => option.value === group),
    ),
    t('未知的已配置分组'),
  );
  configuredSourceGroups.forEach((group) => {
    if (!sourceGroupOptions.some((option) => option.value === group)) {
      sourceGroupOptions.push({
        value: group,
        label: t('未知的已配置分组'),
      });
    }
  });
  const missingRequiredFields = props.rule.enabled
    ? [
        !String(props.rule.id || '').trim() ? t('规则 ID') : '',
        !String(props.rule.source_model || '').trim() ? t('公开模型') : '',
        !String(props.rule.target_model || '').trim()
          ? t(isImageBridge ? '目标图片模型' : '最终上游模型')
          : '',
      ].filter(Boolean)
    : [];

  const updateRule = (patch) => {
    props.onChange({ ...props.rule, ...patch });
  };

  const updateCondition = (conditionIndex, patch) => {
    const nextConditions = conditions.map((condition, currentIndex) => {
      if (currentIndex !== conditionIndex) {
        return condition;
      }
      const nextCondition = { ...condition, ...patch };
      if (
        nextCondition.operator === 'exists' ||
        nextCondition.operator === 'not_exists'
      ) {
        delete nextCondition.value;
      }
      return nextCondition;
    });
    updateRule({ conditions: nextConditions });
  };

  const removeCondition = (conditionIndex) => {
    updateRule({
      conditions: conditions.filter(
        (_, currentIndex) => currentIndex !== conditionIndex,
      ),
    });
  };

  return (
    <div
      style={{
        border: '1px solid var(--semi-color-border)',
        borderRadius: 8,
        padding: 16,
      }}
    >
      <div
        style={{
          display: 'flex',
          flexWrap: 'wrap',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 12,
          marginBottom: 16,
        }}
      >
        <div>
          <Text strong>
            {t('动态路由规则 {{number}}', { number: props.index + 1 })}
          </Text>
          <Text
            type='tertiary'
            size='small'
            style={{ display: 'block', marginTop: 4 }}
          >
            {t(
              isImageToolBridge
                ? '动作：Responses 图片工具桥接'
                : isImageFunctionBridge
                  ? '动作：Text function call to Images API'
                  : '动作：模型重定向',
            )}
          </Text>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <Text size='small'>{t('启用规则')}</Text>
          <Switch
            checked={props.rule.enabled}
            disabled={props.disabled}
            aria-label={t('启用规则')}
            onChange={(enabled) => updateRule({ enabled })}
          />
          <Popconfirm
            title={t('确认删除该动态路由规则？')}
            onConfirm={props.onRemove}
            disabled={props.disabled}
          >
            <Button
              type='danger'
              theme='borderless'
              icon={<IconDelete />}
              aria-label={t('删除规则')}
              disabled={props.disabled}
            />
          </Popconfirm>
        </div>
      </div>

      {missingRequiredFields.length > 0 && (
        <Text
          type='warning'
          size='small'
          style={{
            display: 'block',
            marginBottom: 16,
            border: '1px solid var(--semi-color-warning-light-default)',
            borderRadius: 6,
            padding: '8px 10px',
          }}
        >
          {t('该启用规则尚未完成，请填写：{{fields}}。', {
            fields: missingRequiredFields.join('、'),
          })}
        </Text>
      )}

      <div style={formGridStyle}>
        <label>
          <Text strong size='small'>
            {t('规则 ID')}
          </Text>
          <Input
            value={props.rule.id}
            maxLength={256}
            placeholder='gemini-flash-high'
            style={{ marginTop: 6 }}
            disabled={props.disabled}
            onChange={(id) => updateRule({ id })}
          />
        </label>
        <label>
          <Text strong size='small'>
            {t('优先级')}
          </Text>
          <InputNumber
            value={props.rule.priority ?? 0}
            min={-1000}
            max={1000}
            precision={0}
            style={{ marginTop: 6, width: '100%' }}
            disabled={props.disabled}
            onChange={(priority) =>
              updateRule({
                priority:
                  priority === undefined || priority === null
                    ? 0
                    : Number(priority),
              })
            }
          />
        </label>
        <label>
          <Text strong size='small'>
            {t('动作')}
          </Text>
          <Select
            value={action}
            optionList={ACTION_OPTIONS.map((option) => ({
              value: option.value,
              label: t(option.label),
            }))}
            style={{ width: '100%', marginTop: 6 }}
            disabled={props.disabled}
            onChange={(nextAction) =>
              updateRule({
                action: nextAction,
                target_path:
                  nextAction ===
                    DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE ||
                  nextAction ===
                    DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_FUNCTION_BRIDGE
                    ? DYNAMIC_ROUTING_IMAGE_GENERATION_PATH
                    : undefined,
                request_paths:
                  nextAction ===
                    DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE ||
                  nextAction ===
                    DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_FUNCTION_BRIDGE
                    ? ['/v1/responses']
                    : props.rule.request_paths,
              })
            }
          />
        </label>
        <label>
          <Text strong size='small'>
            {t('公开模型 *')}
          </Text>
          <Input
            value={props.rule.source_model}
            maxLength={256}
            placeholder={t('客户端请求中的 model，例如 gpt-5.6-sol')}
            attr={{ list: sourceModelListId }}
            style={{ marginTop: 6 }}
            disabled={props.disabled}
            onChange={(source_model) => updateRule({ source_model })}
          />
          <Text
            type='tertiary'
            size='small'
            style={{ display: 'block', marginTop: 4 }}
          >
            {t('填写客户端实际发送的 model；下方列表只是提示。')}
          </Text>
        </label>
        <label>
          <Text strong size='small'>
            {t(
              isImageBridge
                ? '目标图片模型 *'
                : '最终上游模型 *',
            )}
          </Text>
          <Input
            value={props.rule.target_model}
            maxLength={256}
            placeholder={
              isImageBridge
                ? t('目标渠道中的图片模型，例如 gpt-image-2')
                : t('目标渠道中的模型，例如 gemini-3.7-flash-high')
            }
            attr={{ list: targetModelListId }}
            style={{ marginTop: 6 }}
            disabled={props.disabled}
            onChange={(target_model) => updateRule({ target_model })}
          />
          <Text
            type='tertiary'
            size='small'
            style={{ display: 'block', marginTop: 4 }}
          >
            {t('该模型必须已配置在将要选中的上游渠道中。')}
          </Text>
        </label>
        {isImageBridge && (
          <>
            <label>
              <Text strong size='small'>
                {t('图片目标路径')}
              </Text>
              {isImageFunctionBridge ? (
                <Input
                  value={DYNAMIC_ROUTING_IMAGE_GENERATION_PATH}
                  readOnly
                  style={{ marginTop: 6 }}
                />
              ) : (
                <Select
                  value={
                    props.rule.target_path ||
                    DYNAMIC_ROUTING_IMAGE_GENERATION_PATH
                  }
                  optionList={DYNAMIC_ROUTING_IMAGE_TARGET_PATHS.map((path) => ({
                    value: path,
                    label: path,
                  }))}
                  style={{ width: '100%', marginTop: 6 }}
                  disabled={props.disabled}
                  onChange={(target_path) => updateRule({ target_path })}
                />
              )}
              {isImageFunctionBridge && (
                <Text
                  type='tertiary'
                  size='small'
                  style={{ display: 'block', marginTop: 4 }}
                >
                  {t('文本函数桥接固定调用 Images API，不能改为 Responses 端点。')}
                </Text>
              )}
            </label>
            <label>
              <Text strong size='small'>
                {t('目标分组（可选）')}
              </Text>
              <Select
                value={configuredTargetGroup || INHERIT_TARGET_GROUP_VALUE}
                optionList={[
                  {
                    value: INHERIT_TARGET_GROUP_VALUE,
                    label: t('继承当前生效分组'),
                  },
                  ...targetGroupOptions,
                ]}
                style={{ width: '100%', marginTop: 6 }}
                disabled={props.disabled}
                emptyContent={t('暂无分组')}
                onChange={(target_group) =>
                  updateRule({
                    target_group:
                      target_group === INHERIT_TARGET_GROUP_VALUE
                        ? undefined
                        : target_group,
                  })
                }
                placeholder={t('选择目标分组')}
              />
              <Text
                type='tertiary'
                size='small'
                style={{ display: 'block', marginTop: 4 }}
              >
                {t(
                  '按分组名称选择即可，系统会自动保存对应的分组 code；选择继承则沿用当前生效分组。',
                )}
              </Text>
            </label>
          </>
        )}
      </div>

      <div style={{ ...formGridStyle, marginTop: 16 }}>
        <label>
          <Text strong size='small'>
            {t('来源分组')}
          </Text>
          <Select
            multiple
            filter
            value={configuredSourceGroups}
            optionList={sourceGroupOptions}
            placeholder={t('全部来源分组')}
            style={{ width: '100%', marginTop: 6 }}
            disabled={props.disabled}
            onChange={(source_groups) =>
              updateRule({
                source_groups: Array.isArray(source_groups)
                  ? source_groups
                  : [],
              })
            }
          />
          <Text
            type='tertiary'
            size='small'
            style={{ display: 'block', marginTop: 4 }}
          >
            {t(
              '按分组名称选择即可，系统会自动保存对应的分组 code；留空时匹配所有来源生效分组。',
            )}
          </Text>
        </label>
        <label>
          <Text strong size='small'>
            {t('上游渠道类型')}
          </Text>
          <Select
            multiple
            filter
            optionList={CHANNEL_OPTIONS.map((option) => ({
              value: String(option.value),
              label: t(option.label),
            }))}
            value={(props.rule.channel_types || []).map(String)}
            placeholder={t('全部渠道类型')}
            style={{ width: '100%', marginTop: 6 }}
            disabled={props.disabled}
            onChange={(channelTypes) =>
              updateRule({
                channel_types: (Array.isArray(channelTypes) ? channelTypes : [])
                  .map(Number)
                  .filter(
                    (channelType) =>
                      Number.isInteger(channelType) && channelType > 0,
                  ),
              })
            }
          />
          <Text
            type='tertiary'
            size='small'
            style={{ display: 'block', marginTop: 4 }}
          >
            {t('留空时匹配所有上游渠道类型。')}
          </Text>
        </label>
        <label>
          <Text strong size='small'>
            {t('请求路径')}
          </Text>
          <Select
            multiple
            filter
            optionList={REQUEST_PATH_OPTIONS.map((requestPath) => ({
              value: requestPath,
              label: requestPath,
            }))}
            value={
              isImageBridge
                ? ['/v1/responses']
                : props.rule.request_paths || []
            }
            style={{ width: '100%', marginTop: 6 }}
            disabled={props.disabled || isImageBridge}
            onChange={(request_paths) =>
              updateRule({
                request_paths: isImageBridge
                  ? ['/v1/responses']
                  : request_paths,
              })
            }
          />
          <Text
            type='tertiary'
            size='small'
            style={{ display: 'block', marginTop: 4 }}
          >
            {t(
              isImageToolBridge
                ? '图片工具桥接使用 /v1/responses，下游目标路径由规则控制。'
                : isImageFunctionBridge
                  ? '文本函数桥接固定使用 /v1/responses；仅当文本模型调用注入函数时，系统才会请求目标 Images API。'
                : '可选。对请求路径进行匹配；不填表示匹配所有路径。',
            )}
          </Text>
        </label>
      </div>

      <div
        style={{
          display: 'flex',
          flexWrap: 'wrap',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 12,
          marginTop: 20,
          marginBottom: 10,
        }}
      >
        <div>
          <Text strong>{t('请求条件')}</Text>
          <Text
            type='tertiary'
            size='small'
            style={{ display: 'block', marginTop: 4 }}
          >
            {t(
              isImageToolBridge
                ? '仅当 tool_choice 明确指定 image_generation 时命中，模型、路径和分组均由规则控制。'
                : isImageFunctionBridge
                  ? '系统会向文本模型注入私有图片函数。模型实际调用后，目标图片请求会独立选渠、计费；流式响应会先缓冲。'
                : '所有条件都满足时才命中。可使用 reasoning_effort 或 request.<简单 JSON 路径>。',
            )}
          </Text>
        </div>
        <Button
          theme='outline'
          icon={<IconPlus />}
          disabled={
            props.disabled ||
            conditions.length >= MAX_DYNAMIC_ROUTING_CONDITIONS
          }
          onClick={() =>
            updateRule({
              conditions: [
                ...conditions,
                {
                  field: 'reasoning_effort',
                  operator: 'equals',
                  value: 'high',
                },
              ],
            })
          }
        >
          {t('添加条件')}
        </Button>
      </div>

      {conditions.length === 0 ? (
        <div
          style={{
            border: '1px dashed var(--semi-color-border)',
            borderRadius: 6,
            padding: 12,
          }}
        >
          <Text type='tertiary' size='small'>
            {t('未设置请求条件，该规则会匹配其作用范围内的所有请求。')}
          </Text>
        </div>
      ) : (
        <div style={{ display: 'grid', gap: 10 }}>
          {conditions.map((condition, conditionIndex) => {
            const operator = condition.operator || 'equals';
            const requiresValue =
              operator !== 'exists' && operator !== 'not_exists';
            return (
              <div
                key={`${conditionIndex}-${condition.field}-${operator}`}
                style={{
                  ...formGridStyle,
                  alignItems: 'end',
                  border: '1px solid var(--semi-color-border)',
                  borderRadius: 6,
                  padding: 12,
                }}
              >
                <label>
                  <Text strong size='small'>
                    {t('条件字段')}
                  </Text>
                  <Input
                    value={condition.field}
                    maxLength={256}
                    placeholder='reasoning_effort'
                    style={{ marginTop: 6 }}
                    disabled={props.disabled}
                    onChange={(field) =>
                      updateCondition(conditionIndex, { field })
                    }
                  />
                </label>
                <label>
                  <Text strong size='small'>
                    {t('运算符')}
                  </Text>
                  <Select
                    value={operator}
                    optionList={OPERATOR_OPTIONS.map((option) => ({
                      value: option.value,
                      label: t(option.label),
                    }))}
                    style={{ width: '100%', marginTop: 6 }}
                    disabled={props.disabled}
                    onChange={(nextOperator) =>
                      updateCondition(conditionIndex, {
                        operator: nextOperator,
                      })
                    }
                  />
                </label>
                <label>
                  <Text strong size='small'>
                    {t('条件值')}
                  </Text>
                  <Input
                    value={condition.value || ''}
                    maxLength={256}
                    placeholder='high'
                    style={{ marginTop: 6 }}
                    disabled={props.disabled || !requiresValue}
                    onChange={(value) =>
                      updateCondition(conditionIndex, { value })
                    }
                  />
                </label>
                <Button
                  type='danger'
                  theme='borderless'
                  icon={<IconDelete />}
                  aria-label={t('删除条件')}
                  disabled={props.disabled}
                  onClick={() => removeCondition(conditionIndex)}
                />
              </div>
            );
          })}
        </div>
      )}

      <datalist id={sourceModelListId}>
        {(props.sourceModelOptions || []).map((model) => (
          <option key={model} value={model} />
        ))}
      </datalist>
      <datalist id={targetModelListId}>
        {(props.targetModelOptions || []).map((model) => (
          <option key={model} value={model} />
        ))}
      </datalist>
    </div>
  );
}

export default function DynamicRoutingSetting() {
  const { t } = useTranslation();
  const [enabled, setEnabled] = useState(false);
  const [rules, setRules] = useState([]);
  const [modelOptions, setModelOptions] = useState([]);
  const [targetGroupOptions, setTargetGroupOptions] = useState([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [parseError, setParseError] = useState(false);

  const refresh = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/option/');
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }

      const options = new Map(
        (Array.isArray(data) ? data : []).map((item) => [item.key, item.value]),
      );
      const parsedRules = parseDynamicRoutingRules(
        options.get('dynamic_routing.rules') || '[]',
      );

      setEnabled(toBoolean(options.get('dynamic_routing.enabled')));
      setParseError(parsedRules === null);
      setRules(parsedRules || []);

      const [modelsResult, groupsResult] = await Promise.allSettled([
        API.get('/api/channel/models_enabled'),
        API.get('/api/group/details'),
      ]);
      if (modelsResult.status === 'fulfilled') {
        const models = modelsResult.value?.data?.data;
        setModelOptions(
          Array.isArray(models)
            ? [...new Set(models.map((model) => String(model).trim()))].filter(
                Boolean,
              )
            : [],
        );
      }
      if (groupsResult.status === 'fulfilled') {
        const groups = extractGroupDetailsResponse(groupsResult.value?.data);
        setTargetGroupOptions(normalizeDynamicRoutingGroupOptions(groups));
      }
    } catch (error) {
      console.error('加载动态路由设置失败:', error);
      showError(t('加载动态路由设置失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    refresh();
  }, []);

  const save = async () => {
    if (parseError) {
      showError(t('现有动态路由规则不是有效 JSON，已阻止覆盖保存。'));
      return;
    }

    const validationError = validateDynamicRoutingRules(rules);
    if (validationError) {
      showError(t(validationError.key, validationError.options));
      return;
    }

    setSaving(true);
    try {
      const rulesResponse = await API.put('/api/option/', {
        key: 'dynamic_routing.rules',
        value: JSON.stringify(rules),
      });
      if (!rulesResponse.data.success) {
        showError(rulesResponse.data.message);
        return;
      }

      const enabledResponse = await API.put('/api/option/', {
        key: 'dynamic_routing.enabled',
        value: String(enabled),
      });
      if (!enabledResponse.data.success) {
        showError(enabledResponse.data.message);
        await refresh();
        return;
      }

      showSuccess(t('动态路由设置已保存'));
    } catch (error) {
      console.error('保存动态路由设置失败:', error);
      showError(t('保存动态路由设置失败'));
    } finally {
      setSaving(false);
    }
  };

  const updateRule = (ruleIndex, nextRule) => {
    setRules((current) =>
      current.map((rule, currentIndex) =>
        currentIndex === ruleIndex ? nextRule : rule,
      ),
    );
  };

  return (
    <Spin spinning={loading} size='large'>
      <div style={{ display: 'grid', gap: 12, marginTop: 10 }}>
        <Banner
          type='info'
          description={t(
            '默认的模型重定向仅改写最终上游模型。图片工具桥接处理显式 image_generation；Text function call to Images API 会向文本模型注入私有函数，实际调用后再向目标 Images API 发请求。两段调用独立计费，默认关闭。',
          )}
          closeIcon={null}
        />

        {parseError && (
          <Banner
            type='danger'
            description={t(
              '当前保存的动态路由规则不是有效 JSON。为防止覆盖现有配置，本页面暂时不能保存，请先通过配置接口修复后刷新。',
            )}
            closeIcon={null}
          />
        )}

        <div
          style={{
            display: 'flex',
            flexWrap: 'wrap',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 12,
          }}
        >
          <div>
            <Text strong>{t('启用动态路由')}</Text>
            <Text
              type='tertiary'
              size='small'
              style={{ display: 'block', marginTop: 4 }}
            >
              {t('关闭后，未被渠道独立启用的规则都不会生效。')}
            </Text>
          </div>
          <Switch
            checked={enabled}
            disabled={saving || parseError}
            aria-label={t('启用动态路由')}
            onChange={setEnabled}
          />
        </div>

        <div
          style={{
            display: 'flex',
            flexWrap: 'wrap',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 12,
          }}
        >
          <div>
            <Text strong>{t('全局路由规则')}</Text>
            <Text
              type='tertiary'
              size='small'
              style={{ display: 'block', marginTop: 4 }}
            >
              {t('数值更高的优先级会先匹配；相同优先级按规则列表顺序匹配。')}
            </Text>
          </div>
          <div style={{ display: 'flex', gap: 8 }}>
            <Button
              theme='outline'
              disabled={loading || saving}
              onClick={refresh}
            >
              {t('刷新')}
            </Button>
            <Button
              theme='outline'
              icon={<IconPlus />}
              disabled={
                saving ||
                parseError ||
                rules.length >= MAX_DYNAMIC_ROUTING_RULES
              }
              onClick={() =>
                setRules((current) => [...current, createDynamicRoutingRule()])
              }
            >
              {t('添加路由规则')}
            </Button>
          </div>
        </div>

        <div
          style={{
            border: '1px dashed var(--semi-color-border)',
            borderRadius: 8,
            padding: 16,
          }}
        >
          <Text strong>{t('快速应用预设')}</Text>
          <Text
            type='tertiary'
            size='small'
            style={{ display: 'block', marginTop: 4 }}
          >
            {t(
              '模板会预填动作、端点和安全条件；启用前必须填写公开模型和目标模型，跨分组时再从分组名称下拉框选择目标分组。模型列表来自当前已启用模型。',
            )}
          </Text>
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))',
              gap: 8,
              marginTop: 12,
            }}
          >
            {DYNAMIC_ROUTING_PRESETS.map((preset) => (
              <Button
                key={preset.id}
                theme='outline'
                disabled={
                  saving ||
                  parseError ||
                  rules.length >= MAX_DYNAMIC_ROUTING_RULES
                }
                style={{
                  alignItems: 'flex-start',
                  display: 'flex',
                  flexDirection: 'column',
                  height: 'auto',
                  padding: '10px 12px',
                  textAlign: 'left',
                  whiteSpace: 'normal',
                }}
                onClick={() =>
                  setRules((current) => [
                    ...current,
                    createDynamicRoutingRuleFromPreset(preset.id),
                  ])
                }
              >
                <Text strong size='small'>
                  {t(preset.label)}
                </Text>
                <Text
                  type='tertiary'
                  size='small'
                  style={{ marginTop: 4, whiteSpace: 'normal' }}
                >
                  {t(preset.description)}
                </Text>
              </Button>
            ))}
          </div>
        </div>

        {rules.length === 0 ? (
          <div
            style={{
              border: '1px dashed var(--semi-color-border)',
              borderRadius: 8,
              padding: 20,
              textAlign: 'center',
            }}
          >
            <Text type='tertiary'>
              {t('暂未配置动态路由规则。添加规则后即可开始使用。')}
            </Text>
          </div>
        ) : (
          rules.map((rule, index) => (
            <DynamicRoutingRuleEditor
              key={`${rule.id || 'route'}-${index}`}
              rule={rule}
              index={index}
              disabled={saving || parseError}
              sourceModelOptions={modelOptions}
              targetModelOptions={modelOptions}
              targetGroupOptions={targetGroupOptions}
              onChange={(nextRule) => updateRule(index, nextRule)}
              onRemove={() =>
                setRules((current) =>
                  current.filter((_, currentIndex) => currentIndex !== index),
                )
              }
            />
          ))
        )}

        <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
          <Button
            type='primary'
            loading={saving}
            disabled={parseError}
            onClick={save}
          >
            {t('保存动态路由设置')}
          </Button>
        </div>
      </div>
    </Spin>
  );
}
