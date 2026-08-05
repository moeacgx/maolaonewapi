import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { runInNewContext } from 'node:vm';

const root = dirname(fileURLToPath(import.meta.url));
const readSource = (...parts) => readFileSync(resolve(root, ...parts), 'utf8');

const editorSource = readSource(
  'pages/Setting/Operation/SettingsSensitiveWords.jsx',
);

function loadEditorHelpers() {
  const helpersStart = editorSource.indexOf("const ACTION_MASK = 'mask'");
  const helpersEnd = editorSource.indexOf(
    'export default function SettingsSensitiveWords',
  );
  assert.notEqual(helpersStart, -1);
  assert.notEqual(helpersEnd, -1);

  const context = {
    crypto: { randomUUID: () => 'test-rule-id' },
  };
  runInNewContext(
    `${editorSource.slice(helpersStart, helpersEnd)}
globalThis.editorHelpers = {
  expandInvalidRule,
  getEmptyRuleTarget,
  getInitialExpandedRuleIds,
  parseRulesConfig,
  removeExpandedRuleId,
  serializeRules,
  toggleExpandedRuleId,
};`,
    context,
  );
  return context.editorHelpers;
}

test('Classic 屏蔽词规则使用全部渠道或渠道与业务分组组合范围', () => {
  const source = readSource(
    'pages/Setting/Operation/SettingsSensitiveWords.jsx',
  );

  assert.match(source, /const TARGET_ROUTES = 'routes'/);
  assert.match(source, /const TARGET_ALL = 'all'/);
  assert.match(source, /target_type: targetType/);
  assert.match(
    source,
    /channel_ids:[\s\S]*?normalizeChannelIds\(rule\.channelIds\)/,
  );
  assert.match(
    source,
    /group_codes:[\s\S]*?normalizeGroupCodes\(rule\.groupCodes\)/,
  );
  assert.match(source, /<Radio value=\{TARGET_ROUTES\}>/);
  assert.match(source, /<Radio value=\{TARGET_ALL\}>/);
  assert.doesNotMatch(source, /selectedChannelIds/);
  assert.doesNotMatch(source, /t\('应用渠道'\)/);
});

test('Classic 使用分组管理中的业务分组并保留失效分组供清理', () => {
  const source = readSource(
    'pages/Setting/Operation/SettingsSensitiveWords.jsx',
  );

  assert.match(source, /\/api\/security-audit\/builtin-policy\/groups/);
  assert.match(source, /normalizeGroupCodes/);
  assert.match(
    source,
    /<Select\.Option\s+key=\{group\.code\}\s+value=\{group\.code\}\s*>/,
  );
  assert.match(source, /!channelIdSet\.has\(id\)/);
  assert.match(source, /!groupCodeSet\.has\(code\)/);
  assert.match(source, /t\('失效渠道'\)/);
  assert.match(source, /t\('失效分组'\)/);
  assert.match(source, /groupsError/);
  assert.match(source, /onClick=\{fetchGroups\}/);
  assert.doesNotMatch(
    source,
    /\{getChannelLabel\(channel\)\} #\{channel\.id\}/,
  );
  assert.equal((source.match(/maxTagCount=\{1\}/g) || []).length, 2);
  assert.equal((source.match(/ellipsisTrigger/g) || []).length, 2);
  assert.equal((source.match(/showRestTagsPopover/g) || []).length, 2);
});

test('Classic 将历史全局渠道复制到规则草稿并校验启用规则目标', () => {
  const editor = readSource(
    'pages/Setting/Operation/SettingsSensitiveWords.jsx',
  );
  const wrapper = readSource('pages/SecurityAudit/BuiltinPolicyTab.jsx');

  assert.match(
    editor,
    /parseRulesConfig\([\s\S]*?rawInputs\.SensitiveRules,[\s\S]*?rawInputs\.SensitiveWords,[\s\S]*?nextChannelIds/,
  );
  assert.match(
    editor,
    /rule\.target_type[\s\S]*?rule\.channel_ids[\s\S]*?: legacyChannelIds/,
  );
  assert.match(editor, /getEmptyRuleTarget/);
  assert.match(editor, /hasInvalidTargets/);
  assert.match(editor, /启用的规则必须至少选择一个渠道或分组，或选择全部渠道/);
  assert.doesNotMatch(editor, /关键词组引用/);
  assert.match(editor, /group_refs:/);
  assert.match(
    wrapper,
    /sensitive_rule_channel_ids: values\.SensitiveRuleChannelIds/,
  );
  assert.match(
    wrapper,
    /SensitiveRuleChannelIds: draft\.sensitive_rule_channel_ids/,
  );
});

test('Classic 规则转换会迁移旧范围并同时序列化渠道与业务分组', () => {
  const { parseRulesConfig, serializeRules } = loadEditorHelpers();
  const drafts = parseRulesConfig(
    JSON.stringify({
      rules: [
        {
          id: 'legacy',
          enabled: true,
          action: 'block',
          keywords: ['legacy'],
        },
        {
          id: 'tags',
          enabled: true,
          action: 'block',
          keywords: ['tags'],
          target_type: 'groups',
          group_codes: [' backup ', 'primary', 'backup'],
        },
      ],
    }),
    '',
    [9, 3, 9],
  );
  const normalizedDrafts = JSON.parse(JSON.stringify(drafts));

  assert.deepEqual(normalizedDrafts[0].channelIds, [3, 9]);
  assert.equal(normalizedDrafts[0].targetType, 'routes');
  assert.deepEqual(normalizedDrafts[1].groupCodes, ['backup', 'primary']);
  assert.equal(normalizedDrafts[1].targetType, 'routes');

  const serialized = JSON.parse(serializeRules(drafts)).rules;
  assert.deepEqual(serialized[0].channel_ids, [3, 9]);
  assert.deepEqual(serialized[0].group_codes, []);
  assert.equal(serialized[0].channel_tags, undefined);
  assert.deepEqual(serialized[1].group_codes, ['backup', 'primary']);
  assert.deepEqual(serialized[1].channel_ids, []);
});

test('Classic 仅阻止有内容且已启用的空目标规则', () => {
  const { getEmptyRuleTarget } = loadEditorHelpers();
  const rule = {
    enabled: true,
    keywordsText: 'blocked',
    groupRefs: [],
    targetType: 'routes',
    channelIds: [],
    channelTags: [],
  };

  assert.equal(getEmptyRuleTarget(rule), 'routes');
  assert.equal(
    getEmptyRuleTarget({ ...rule, targetType: 'channel_tags' }),
    'channel_tags',
  );
  assert.equal(
    getEmptyRuleTarget({
      ...rule,
      targetType: 'routes',
      groupCodes: [],
    }),
    'routes',
  );
  assert.equal(
    getEmptyRuleTarget({
      ...rule,
      targetType: 'routes',
      groupCodes: ['306'],
    }),
    null,
  );
  assert.equal(getEmptyRuleTarget({ ...rule, channelIds: [306] }), null);
  assert.equal(getEmptyRuleTarget({ ...rule, targetType: 'all' }), null);
  assert.equal(getEmptyRuleTarget({ ...rule, enabled: false }), null);
  assert.equal(getEmptyRuleTarget({ ...rule, keywordsText: '' }), null);
});

test('Classic 屏蔽词规则默认折叠并独立控制展开状态', () => {
  const source = readSource(
    'pages/Setting/Operation/SettingsSensitiveWords.jsx',
  );

  assert.match(source, /IconChevronDown/);
  assert.match(source, /IconChevronRight/);
  assert.match(
    source,
    /const \[expandedRuleIds, setExpandedRuleIds\] = useState\(\(\) => new Set\(\)\)/,
  );
  assert.match(source, /getInitialExpandedRuleIds\(nextRules\)/);
  assert.match(
    source,
    /const addRule = \(\) => \{[\s\S]*?next\.add\(nextRule\.id\)/,
  );
  assert.match(
    source,
    /const removeRule = \(id\) => \{[\s\S]*?removeExpandedRuleId\(prev, id\)/,
  );
  assert.match(source, /const isExpanded = expandedRuleIds\.has\(rule\.id\)/);
  assert.match(source, /expandInvalidRule\(current, nextRule\)/);
  assert.match(source, /aria-expanded=\{isExpanded\}/);
  assert.match(source, /aria-controls=\{panelId\}/);
  assert.match(source, /onClick=\{\(\) => toggleRuleExpanded\(rule\.id\)\}/);
  assert.match(source, /id=\{panelId\}[\s\S]*?role='region'/);
  assert.match(source, /\{isExpanded \? \(/);
  assert.match(source, /\{getRuleSummary\(rule\)\}/);
  assert.match(source, /aria-label=\{t\('启用规则'\)\}/);
  assert.match(source, /onClick=\{\(\) => removeRule\(rule\.id\)\}/);
});

test('Classic 折叠状态允许收起错误规则且不会串扰其他规则', () => {
  const {
    expandInvalidRule,
    getInitialExpandedRuleIds,
    removeExpandedRuleId,
    toggleExpandedRuleId,
  } = loadEditorHelpers();
  const invalidRule = {
    id: 'invalid',
    enabled: true,
    keywordsText: 'blocked',
    groupRefs: [],
    targetType: 'routes',
    channelIds: [],
    channelTags: [],
    groupCodes: [],
  };
  const validRule = {
    ...invalidRule,
    id: 'valid',
    targetType: 'all',
  };

  let expanded = getInitialExpandedRuleIds([invalidRule, validRule]);
  assert.equal(expanded.has('invalid'), true);
  assert.equal(expanded.has('valid'), false);
  assert.equal(expandInvalidRule(expanded, validRule).has('invalid'), true);

  expanded = toggleExpandedRuleId(expanded, 'invalid');
  assert.equal(expanded.has('invalid'), false);
  expanded = expandInvalidRule(expanded, invalidRule);
  assert.equal(expanded.has('invalid'), true);

  expanded = toggleExpandedRuleId(expanded, 'invalid');
  expanded = expandInvalidRule(expanded, validRule);
  assert.equal(expanded.has('invalid'), false);
  expanded = toggleExpandedRuleId(expanded, 'valid');
  assert.equal(expanded.has('valid'), true);
  assert.equal(expanded.has('invalid'), false);

  expanded = removeExpandedRuleId(expanded, 'valid');
  assert.equal(expanded.has('valid'), false);
});

test('Classic 审计详情显示命中关键词并保留 Markdown 渲染', () => {
  const source = readSource('pages/SecurityAudit/EventsTab.jsx');

  assert.match(source, /matched_keywords/);
  assert.match(source, /createKeywordHighlightPlugin/);
  assert.match(source, /rehypePlugins/);
  assert.match(source, /color='red'/);
  assert.match(source, /ReactMarkdown/);
});

test('Classic 审计列表显示用户官方风控窗口累计次数', () => {
  const source = readSource('pages/SecurityAudit/EventsTab.jsx');

  assert.match(source, /t\('窗口内累计'\)/);
  assert.match(source, /dataIndex: 'user_cyber_policy_count'/);
  assert.match(source, /record\.cyber_policy_window_hours/);
  assert.match(source, /t\('\{\{count\}\} 次', \{ count \}\)/);
  assert.match(source, /t\('\{\{hours\}\} 小时内', \{ hours \}\)/);
});
