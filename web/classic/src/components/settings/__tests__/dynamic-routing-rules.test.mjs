import assert from 'node:assert/strict';
import test from 'node:test';

import {
  createDynamicRoutingRule,
  createDynamicRoutingRuleFromPreset,
  parseDynamicRoutingRules,
  validateDynamicRoutingRules,
} from '../dynamic-routing-rules.js';

test('动态路由规则解析会补齐模型重定向动作并保留高思考条件', () => {
  const rules = parseDynamicRoutingRules(
    JSON.stringify([
      {
        id: 'gemini-high',
        enabled: true,
        source_model: 'gemini-3.7-flash',
        target_model: 'gemini-3.7-flash-high',
        conditions: [
          {
            field: 'reasoning_effort',
            operator: 'equals',
            value: 'high',
          },
        ],
      },
    ]),
  );

  assert.deepEqual(rules, [
    {
      id: 'gemini-high',
      enabled: true,
      action: 'model_redirect',
      source_model: 'gemini-3.7-flash',
      target_model: 'gemini-3.7-flash-high',
      channel_types: [],
      request_paths: [],
      conditions: [
        {
          field: 'reasoning_effort',
          operator: 'equals',
          value: 'high',
        },
      ],
      priority: 0,
    },
  ]);
  assert.equal(validateDynamicRoutingRules(rules), null);
});

test('启用的动态路由规则拒绝重复 ID，禁用规则不参与重复校验', () => {
  const first = createDynamicRoutingRule();
  first.id = 'same-route';
  first.source_model = 'gemini-3.7-flash';
  first.target_model = 'gemini-3.7-flash-high';

  const duplicate = { ...first };
  const duplicateError = validateDynamicRoutingRules([first, duplicate]);
  assert.equal(
    duplicateError.key,
    '启用的动态路由规则 ID 不能重复（第 {{number}} 条）',
  );
  assert.deepEqual(duplicateError.options, { number: 2 });

  duplicate.enabled = false;
  assert.equal(validateDynamicRoutingRules([first, duplicate]), null);
});

test('动态路由规则拒绝不安全的请求字段和带查询串的请求路径', () => {
  const rule = createDynamicRoutingRule();
  rule.source_model = 'gpt-image-2';
  rule.target_model = 'gpt-image-2-enterprise';
  rule.request_paths = ['/v1/images/generations?size=1024x1024'];

  const pathError = validateDynamicRoutingRules([rule]);
  assert.equal(
    pathError.key,
    '第 {{number}} 条动态路由规则包含无效或重复的请求路径',
  );

  rule.request_paths = ['/v1/images/generations'];
  rule.conditions = [
    {
      field: 'request.reasoning..effort',
      operator: 'equals',
      value: 'high',
    },
  ];
  const fieldError = validateDynamicRoutingRules([rule]);
  assert.equal(
    fieldError.key,
    '第 {{number}} 条动态路由规则的条件字段只能是 reasoning_effort 或 request.<简单 JSON 路径>',
  );
});

test('图片工具桥接规则固定下游 Responses 路径', () => {
  const rules = parseDynamicRoutingRules([
    {
      id: 'image-bridge',
      enabled: true,
      action: 'responses_image_tool_bridge',
      source_model: 'gpt-5.6-sol',
      target_model: 'gpt-image-2',
      request_paths: [],
    },
  ]);

  assert.deepEqual(rules[0].request_paths, ['/v1/responses']);
  assert.equal(rules[0].target_path, '/v1/images/generations');
  assert.equal(validateDynamicRoutingRules(rules), null);
});

test('动态路由预设只填充动作、端点和安全条件，不猜测模型或分组', () => {
  const reasoningRule = createDynamicRoutingRuleFromPreset('reasoning_high');
  const responsesImageRule = createDynamicRoutingRuleFromPreset(
    'responses_image_tool',
  );
  const imagesApiRule = createDynamicRoutingRuleFromPreset(
    'images_api_image_tool',
  );

  assert.deepEqual(reasoningRule.conditions, [
    {
      field: 'reasoning_effort',
      operator: 'equals',
      value: 'high',
    },
  ]);
  assert.equal(reasoningRule.source_model, '');
  assert.equal(reasoningRule.target_model, '');
  assert.equal(responsesImageRule.target_path, '/v1/responses');
  assert.deepEqual(responsesImageRule.request_paths, ['/v1/responses']);
  assert.equal(imagesApiRule.target_path, '/v1/images/generations');
  assert.deepEqual(imagesApiRule.request_paths, ['/v1/responses']);
});
