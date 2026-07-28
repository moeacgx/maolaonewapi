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
  getEmptyRuleTarget,
  parseRulesConfig,
  serializeRules,
};`,
    context,
  );
  return context.editorHelpers;
}

test('Classic 屏蔽词规则使用全部渠道、渠道或业务分组范围', () => {
  const source = readSource(
    'pages/Setting/Operation/SettingsSensitiveWords.jsx',
  );

  assert.match(source, /const TARGET_CHANNELS = 'channels'/);
  assert.match(source, /const TARGET_GROUPS = 'groups'/);
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
  assert.match(source, /<Radio value=\{TARGET_CHANNELS\}>/);
  assert.match(source, /<Radio value=\{TARGET_GROUPS\}>/);
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
    /<Select\.Option key=\{group\.code\} value=\{group\.code\}>/,
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
    /rule\.target_type === TARGET_CHANNELS[\s\S]*?: legacyChannelIds/,
  );
  assert.match(editor, /getEmptyRuleTarget/);
  assert.match(editor, /hasInvalidTargets/);
  assert.match(editor, /启用的规则必须至少选择一个渠道、分组或选择全部渠道/);
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

test('Classic 规则转换实际迁移旧渠道并只序列化当前目标类型', () => {
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
  assert.equal(normalizedDrafts[0].targetType, 'channels');
  assert.deepEqual(normalizedDrafts[1].groupCodes, ['backup', 'primary']);
  assert.equal(normalizedDrafts[1].targetType, 'groups');

  const serialized = JSON.parse(serializeRules(drafts)).rules;
  assert.deepEqual(serialized[0].channel_ids, [3, 9]);
  assert.equal(serialized[0].channel_tags, undefined);
  assert.deepEqual(serialized[1].group_codes, ['backup', 'primary']);
  assert.equal(serialized[1].channel_ids, undefined);
});

test('Classic 仅阻止有内容且已启用的空目标规则', () => {
  const { getEmptyRuleTarget } = loadEditorHelpers();
  const rule = {
    enabled: true,
    keywordsText: 'blocked',
    groupRefs: [],
    targetType: 'channels',
    channelIds: [],
    channelTags: [],
  };

  assert.equal(getEmptyRuleTarget(rule), 'channels');
  assert.equal(
    getEmptyRuleTarget({ ...rule, targetType: 'channel_tags' }),
    'channel_tags',
  );
  assert.equal(getEmptyRuleTarget({ ...rule, enabled: false }), null);
  assert.equal(getEmptyRuleTarget({ ...rule, keywordsText: '' }), null);
});
