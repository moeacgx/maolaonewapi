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

export const DYNAMIC_ROUTING_ACTION = 'model_redirect';
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

function normalizeString(value) {
  return typeof value === 'string' ? value.trim() : '';
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

function normalizeRule(value) {
  const rule = value && typeof value === 'object' ? value : {};
  const priority = Number(rule.priority);

  return {
    id: normalizeString(rule.id),
    enabled: rule.enabled !== false,
    action: DYNAMIC_ROUTING_ACTION,
    source_model: normalizeString(rule.source_model),
    target_model: normalizeString(rule.target_model),
    channel_types: Array.isArray(rule.channel_types)
      ? rule.channel_types
          .map((channelType) => Number(channelType))
          .filter(
            (channelType) => Number.isInteger(channelType) && channelType > 0,
          )
      : [],
    request_paths: Array.isArray(rule.request_paths)
      ? rule.request_paths.map((path) => normalizeString(path)).filter(Boolean)
      : [],
    conditions: Array.isArray(rule.conditions)
      ? rule.conditions.map(normalizeCondition)
      : [],
    priority: Number.isInteger(priority) ? priority : 0,
  };
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

export function createDynamicRoutingRule() {
  const suffix = `${Date.now().toString(36)}-${Math.random()
    .toString(36)
    .slice(2, 8)}`;

  return {
    id: `route-${suffix}`,
    enabled: true,
    action: DYNAMIC_ROUTING_ACTION,
    source_model: '',
    target_model: '',
    channel_types: [],
    request_paths: [],
    conditions: [],
    priority: 0,
  };
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
      targetModel.length > MAX_DYNAMIC_ROUTING_STRING_LENGTH
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
