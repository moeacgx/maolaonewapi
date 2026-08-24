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

export const DYNAMIC_ROUTING_ACTION_MODEL_REDIRECT = 'model_redirect';
export const DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE =
  'responses_image_tool_bridge';
export const DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_FUNCTION_BRIDGE =
  'responses_image_function_bridge';
export const DYNAMIC_ROUTING_ACTIONS = [
  DYNAMIC_ROUTING_ACTION_MODEL_REDIRECT,
  DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE,
  DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_FUNCTION_BRIDGE,
];
export const DYNAMIC_ROUTING_OPERATORS = [
  'equals',
  'not_equals',
  'exists',
  'not_exists',
];
export const MAX_DYNAMIC_ROUTING_RULES = 100;
export const MAX_DYNAMIC_ROUTING_CONDITIONS = 8;
export const MAX_DYNAMIC_ROUTING_PRIORITY = 1000;
export const MAX_DYNAMIC_ROUTING_STRING_LENGTH = 256;
export const DYNAMIC_ROUTING_IMAGE_GENERATION_PATH = '/v1/images/generations';
export const DYNAMIC_ROUTING_RESPONSES_PATH = '/v1/responses';
export const DYNAMIC_ROUTING_IMAGE_TARGET_PATHS = [
  DYNAMIC_ROUTING_RESPONSES_PATH,
  DYNAMIC_ROUTING_IMAGE_GENERATION_PATH,
];
export const DYNAMIC_ROUTING_PRESETS = [
  {
    id: 'model_redirect',
    label: '基础模型重定向',
    description: '保持当前请求端点，只改写最终上游模型。',
  },
  {
    id: 'reasoning_high',
    label: '思考等级重定向',
    description: '预填 reasoning_effort=high，路由到专用上游模型。',
  },
  {
    id: 'responses_image_tool',
    label: 'Responses 图片工具转 Responses',
    description:
      '将明确选择的 image_generation 工具桥接到支持 Responses 的图片模型。',
  },
  {
    id: 'images_api_image_tool',
    label: 'Responses 图片工具转 Images API',
    description:
      '将明确选择的 image_generation 工具桥接到 /v1/images/generations。',
  },
  {
    id: 'responses_image_function',
    label: 'Text function call to Images API',
    description:
      '向 /v1/responses 注入私有图片函数；仅当文本模型实际调用时，才请求 /v1/images/generations。流式响应会缓冲，文本和图片分别计费。',
  },
];

function isResponsesImageBridgeAction(action) {
  return (
    action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE ||
    action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_FUNCTION_BRIDGE
  );
}

function normalizeString(value) {
  return typeof value === 'string' ? value.trim() : '';
}

export function normalizeDynamicRoutingGroupOptions(groups) {
  if (!Array.isArray(groups)) return [];

  const seenCodes = new Set();
  return groups.reduce((options, group) => {
    const code = normalizeString(group?.code);
    if (!code || seenCodes.has(code)) return options;

    seenCodes.add(code);
    options.push({
      value: code,
      label: normalizeString(group?.name) || code,
    });
    return options;
  }, []);
}

export function addDynamicRoutingConfiguredGroupOption(
  options,
  configuredTargetGroup,
  unknownLabel = '未知的已配置分组',
) {
  const normalizedOptions = Array.isArray(options) ? [...options] : [];
  const configuredCode = normalizeString(configuredTargetGroup);
  if (
    !configuredCode ||
    normalizedOptions.some((option) => option?.value === configuredCode)
  ) {
    return normalizedOptions;
  }

  normalizedOptions.unshift({
    value: configuredCode,
    label: unknownLabel,
  });
  return normalizedOptions;
}

function normalizeCondition(value) {
  const condition = value && typeof value === 'object' ? value : {};
  const operator = DYNAMIC_ROUTING_OPERATORS.includes(condition.operator)
    ? condition.operator
    : 'equals';
  const normalized = {
    field: normalizeString(condition.field),
    operator,
  };

  if (operator !== 'exists' && operator !== 'not_exists') {
    normalized.value =
      typeof condition.value === 'string' ? condition.value : '';
  }

  return normalized;
}

function normalizeAction(value) {
  return DYNAMIC_ROUTING_ACTIONS.includes(value)
    ? value
    : DYNAMIC_ROUTING_ACTION_MODEL_REDIRECT;
}

function normalizeRule(value) {
  const rule = value && typeof value === 'object' ? value : {};
  const priority = Number(rule.priority);

  const normalized = {
    id: normalizeString(rule.id),
    enabled: rule.enabled !== false,
    action: normalizeAction(rule.action),
    source_model: normalizeString(rule.source_model),
    target_model: normalizeString(rule.target_model),
    source_groups: Array.isArray(rule.source_groups)
      ? rule.source_groups.map(normalizeString).filter(Boolean)
      : [],
    target_group: normalizeString(rule.target_group),
    channel_types: Array.isArray(rule.channel_types)
      ? rule.channel_types
          .map((channelType) => Number(channelType))
          .filter(
            (channelType) => Number.isInteger(channelType) && channelType > 0,
          )
      : [],
    request_paths: isResponsesImageBridgeAction(normalizeAction(rule.action))
      ? ['/v1/responses']
      : Array.isArray(rule.request_paths)
        ? rule.request_paths.map((path) => normalizeString(path)).filter(Boolean)
        : [],
    conditions: Array.isArray(rule.conditions)
      ? rule.conditions.map(normalizeCondition)
      : [],
    priority: Number.isInteger(priority) ? priority : 0,
  };

  if (normalized.source_groups.length === 0) delete normalized.source_groups;
  if (!normalized.target_group) delete normalized.target_group;

  if (
    normalized.action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE
  ) {
    normalized.target_path =
      normalizeString(rule.target_path) ||
      DYNAMIC_ROUTING_IMAGE_GENERATION_PATH;
    if (!DYNAMIC_ROUTING_IMAGE_TARGET_PATHS.includes(normalized.target_path)) {
      normalized.target_path = DYNAMIC_ROUTING_IMAGE_GENERATION_PATH;
    }
  }
  if (
    normalized.action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_FUNCTION_BRIDGE
  ) {
    normalized.target_path = DYNAMIC_ROUTING_IMAGE_GENERATION_PATH;
  }
  return normalized;
}

export function parseDynamicRoutingRules(rawValue) {
  if (Array.isArray(rawValue)) {
    return rawValue.map(normalizeRule);
  }
  if (typeof rawValue !== 'string' || rawValue.trim() === '') {
    return [];
  }

  try {
    const parsed = JSON.parse(rawValue);
    return Array.isArray(parsed) ? parsed.map(normalizeRule) : null;
  } catch {
    return null;
  }
}

function createDynamicRoutingRuleBase(prefix) {
  const suffix = `${Date.now().toString(36)}-${Math.random()
    .toString(36)
    .slice(2, 8)}`;

  return {
    id: `${prefix}-${suffix}`,
    enabled: true,
    action: DYNAMIC_ROUTING_ACTION_MODEL_REDIRECT,
    source_model: '',
    target_model: '',
    channel_types: [],
    request_paths: [],
    conditions: [],
    priority: 0,
  };
}

export function createDynamicRoutingRule() {
  return createDynamicRoutingRuleBase('route');
}

export function createDynamicRoutingRuleFromPreset(preset) {
  switch (preset) {
    case 'reasoning_high':
      return {
        ...createDynamicRoutingRuleBase('reasoning-high'),
        conditions: [
          {
            field: 'reasoning_effort',
            operator: 'equals',
            value: 'high',
          },
        ],
      };
    case 'responses_image_tool':
      return {
        ...createDynamicRoutingRuleBase('responses-image'),
        action: DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE,
        request_paths: [DYNAMIC_ROUTING_RESPONSES_PATH],
        target_path: DYNAMIC_ROUTING_RESPONSES_PATH,
      };
    case 'images_api_image_tool':
      return {
        ...createDynamicRoutingRuleBase('images-api-image'),
        action: DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE,
        request_paths: [DYNAMIC_ROUTING_RESPONSES_PATH],
        target_path: DYNAMIC_ROUTING_IMAGE_GENERATION_PATH,
      };
    case 'responses_image_function':
      return {
        ...createDynamicRoutingRuleBase('responses-image-function'),
        action: DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_FUNCTION_BRIDGE,
        request_paths: [DYNAMIC_ROUTING_RESPONSES_PATH],
        target_path: DYNAMIC_ROUTING_IMAGE_GENERATION_PATH,
      };
    case 'model_redirect':
      return createDynamicRoutingRuleBase('model-redirect');
    default:
      return createDynamicRoutingRule();
  }
}

function validationError(key, options) {
  let normalizedOptions = options;
  if (typeof options === 'number') {
    normalizedOptions = { number: options };
  }

  return {
    key,
    options: normalizedOptions,
  };
}

export function validateDynamicRoutingRules(rules) {
  if (!Array.isArray(rules)) {
    return validationError('动态路由规则配置无效');
  }
  if (rules.length > MAX_DYNAMIC_ROUTING_RULES) {
    return validationError('动态路由最多只能添加 {{count}} 条规则', {
      count: MAX_DYNAMIC_ROUTING_RULES,
    });
  }

  const enabledIds = new Set();
  for (let index = 0; index < rules.length; index += 1) {
    const rule = rules[index] || {};
    const number = index + 1;
    const priority = Number(rule.priority ?? 0);
    const channelTypes = Array.isArray(rule.channel_types)
      ? rule.channel_types
      : [];
    const requestPaths = Array.isArray(rule.request_paths)
      ? rule.request_paths
      : [];
    const conditions = Array.isArray(rule.conditions) ? rule.conditions : [];
    const action = normalizeAction(rule.action);

    if (
      !Number.isInteger(priority) ||
      priority < -MAX_DYNAMIC_ROUTING_PRIORITY ||
      priority > MAX_DYNAMIC_ROUTING_PRIORITY
    ) {
      return validationError(
        '第 {{number}} 条动态路由规则的优先级必须是 -1000 到 1000 之间的整数',
        number,
      );
    }
    if (conditions.length > MAX_DYNAMIC_ROUTING_CONDITIONS) {
      return validationError(
        '第 {{number}} 条动态路由规则最多只能添加 8 个条件',
        number,
      );
    }

    const channelTypeSet = new Set();
    for (const channelType of channelTypes) {
      if (
        !Number.isInteger(channelType) ||
        channelType <= 0 ||
        channelTypeSet.has(channelType)
      ) {
        return validationError(
          '第 {{number}} 条动态路由规则包含无效或重复的渠道类型',
          number,
        );
      }
      channelTypeSet.add(channelType);
    }

    const requestPathSet = new Set();
    for (const requestPath of requestPaths) {
      if (
        typeof requestPath !== 'string' ||
        !requestPath.startsWith('/') ||
        requestPath.includes('?') ||
        requestPath.length > MAX_DYNAMIC_ROUTING_STRING_LENGTH ||
        requestPathSet.has(requestPath)
      ) {
        return validationError(
          '第 {{number}} 条动态路由规则包含无效或重复的请求路径',
          number,
        );
      }
      requestPathSet.add(requestPath);
    }
    if (
      isResponsesImageBridgeAction(action) &&
      (requestPaths.length !== 1 || requestPaths[0] !== '/v1/responses')
    ) {
      return validationError(
        '第 {{number}} 条图片桥接规则必须固定使用 /v1/responses 请求路径',
        number,
      );
    }
    const sourceGroups = Array.isArray(rule.source_groups)
      ? rule.source_groups
      : [];
    const sourceGroupSet = new Set();
    for (const sourceGroup of sourceGroups) {
      const normalizedGroup = normalizeString(sourceGroup);
      if (
        !normalizedGroup ||
        normalizedGroup.length > MAX_DYNAMIC_ROUTING_STRING_LENGTH ||
        normalizedGroup.includes(',') ||
        normalizedGroup.toLowerCase() === 'auto' ||
        sourceGroupSet.has(normalizedGroup)
      ) {
        return validationError(
          '第 {{number}} 条动态路由规则包含无效或重复的来源分组',
          number,
        );
      }
      sourceGroupSet.add(normalizedGroup);
    }
    const targetGroup = normalizeString(rule.target_group);
    if (
      targetGroup.length > MAX_DYNAMIC_ROUTING_STRING_LENGTH ||
      targetGroup.includes(',') ||
      targetGroup.toLowerCase() === 'auto'
    ) {
      return validationError(
        '第 {{number}} 条动态路由规则的目标分组必须是单个有效分组',
        number,
      );
    }
    if (
      action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE &&
      !DYNAMIC_ROUTING_IMAGE_TARGET_PATHS.includes(
        normalizeString(rule.target_path) ||
          DYNAMIC_ROUTING_IMAGE_GENERATION_PATH,
      )
    ) {
      return validationError(
        '第 {{number}} 条图片工具桥接规则的目标路径只能是 /v1/responses 或 /v1/images/generations',
        number,
      );
    }
    if (
      action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_FUNCTION_BRIDGE &&
      normalizeString(rule.target_path) &&
      normalizeString(rule.target_path) !== DYNAMIC_ROUTING_IMAGE_GENERATION_PATH
    ) {
      return validationError(
        '第 {{number}} 条文本函数图片桥接规则的目标路径只能是 /v1/images/generations',
        number,
      );
    }
    for (const condition of conditions) {
      const field = normalizeString(condition?.field);
      const operator = condition?.operator || 'equals';
      const isRequestField =
        field.startsWith('request.') &&
        field.length > 'request.'.length &&
        /^[A-Za-z0-9_.]+$/.test(field.slice('request.'.length)) &&
        !field.includes('..') &&
        !field.endsWith('.');

      if (field !== 'reasoning_effort' && !isRequestField) {
        return validationError(
          '第 {{number}} 条动态路由规则的条件字段只能是 reasoning_effort 或 request.<简单 JSON 路径>',
          number,
        );
      }
      if (!DYNAMIC_ROUTING_OPERATORS.includes(operator)) {
        return validationError(
          '第 {{number}} 条动态路由规则使用了不支持的条件运算符',
          number,
        );
      }
      if (
        typeof condition?.value === 'string' &&
        condition.value.length > MAX_DYNAMIC_ROUTING_STRING_LENGTH
      ) {
        return validationError(
          '第 {{number}} 条动态路由规则的条件值不能超过 256 个字符',
          number,
        );
      }
    }

    if (!rule.enabled) {
      continue;
    }

    const id = normalizeString(rule.id);
    const sourceModel = normalizeString(rule.source_model);
    const targetModel = normalizeString(rule.target_model);
    if (!id || !sourceModel || !targetModel) {
      return validationError(
        '第 {{number}} 条启用的动态路由规则必须填写规则 ID、公开模型和最终上游模型',
        number,
      );
    }
    if (
      id.length > MAX_DYNAMIC_ROUTING_STRING_LENGTH ||
      sourceModel.length > MAX_DYNAMIC_ROUTING_STRING_LENGTH ||
      targetModel.length > MAX_DYNAMIC_ROUTING_STRING_LENGTH ||
      targetGroup.length > MAX_DYNAMIC_ROUTING_STRING_LENGTH
    ) {
      return validationError(
        '第 {{number}} 条动态路由规则的 ID 和模型名不能超过 256 个字符',
        number,
      );
    }
    if (enabledIds.has(id)) {
      return validationError(
        '启用的动态路由规则 ID 不能重复（第 {{number}} 条）',
        number,
      );
    }
    enabledIds.add(id);
  }

  return null;
}
