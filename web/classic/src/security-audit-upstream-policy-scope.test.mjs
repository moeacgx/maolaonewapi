import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { runInNewContext } from 'node:vm';

const root = dirname(fileURLToPath(import.meta.url));
const readSource = (...parts) => readFileSync(resolve(root, ...parts), 'utf8');

const apiSource = readSource('pages/SecurityAudit/api.js');
const tabSource = readSource('pages/SecurityAudit/BuiltinPolicyTab.jsx');

function loadPolicyNormalizer() {
  const helpersStart = apiSource.indexOf(
    'const UPSTREAM_POLICY_TARGET_TYPES = new Set',
  );
  const helpersEnd = apiSource.indexOf('export const cleanSecurityAuditFilter');
  assert.notEqual(helpersStart, -1);
  assert.notEqual(helpersEnd, -1);

  const context = {};
  const source = apiSource
    .slice(helpersStart, helpersEnd)
    .replace(
      'export const builtinPolicyConfigToDraft',
      'const builtinPolicyConfigToDraft',
    );
  runInNewContext(
    `${source}
globalThis.builtinPolicyConfigToDraft = builtinPolicyConfigToDraft;`,
    context,
  );
  return context.builtinPolicyConfigToDraft;
}

test('Classic 规范化官方风控作用范围并默认作用于全部渠道', () => {
  const normalize = loadPolicyNormalizer();
  const normalized = JSON.parse(
    JSON.stringify(
      normalize({
        upstream_policy_target_type: 'invalid',
        upstream_policy_channel_ids: ['2', 2, -1, 7],
        upstream_policy_group_codes: [' primary ', '', 'backup', 'primary'],
      }),
    ),
  );

  assert.equal(normalized.upstream_policy_target_type, 'all');
  assert.deepEqual(normalized.upstream_policy_channel_ids, [2, 7]);
  assert.deepEqual(normalized.upstream_policy_group_codes, [
    'primary',
    'backup',
  ]);
});

test('Classic 保存官方风控范围完整契约', () => {
  assert.match(apiSource, /upstream_policy_target_type:/);
  assert.match(apiSource, /upstream_policy_channel_ids:/);
  assert.match(apiSource, /upstream_policy_group_codes:/);
  assert.match(
    apiSource,
    /API\.get\(`\$\{API_ROOT\}\/builtin-policy\/channels`, requestConfig\)/,
  );
  assert.match(apiSource, /API\.get\('\/api\/group\/details', requestConfig\)/);
});

test('Classic 支持全部渠道、多选渠道和多选稳定分组编码', () => {
  assert.match(tabSource, /const TARGET_ALL = 'all'/);
  assert.match(tabSource, /const TARGET_CHANNELS = 'channels'/);
  assert.match(tabSource, /const TARGET_GROUPS = 'groups'/);
  assert.match(tabSource, /<Radio value=\{TARGET_ALL\}>/);
  assert.match(tabSource, /<Radio value=\{TARGET_CHANNELS\}>/);
  assert.match(tabSource, /<Radio value=\{TARGET_GROUPS\}>/);
  assert.match(tabSource, /multiple[\s\S]*?upstream_policy_channel_ids/);
  assert.match(tabSource, /multiple[\s\S]*?upstream_policy_group_codes/);
  assert.match(
    tabSource,
    /<Select\.Option[\s\S]*?key=\{group\.code\}[\s\S]*?value=\{group\.code\}/,
  );
  assert.match(tabSource, /getSecurityAuditBuiltinPolicyGroups\(\)/);
  assert.match(apiSource, /\/builtin-policy\/groups/);
});

test('Classic 切换官方风控模式保留其他选择并纳入 dirty', () => {
  assert.match(tabSource, /upstream_policy_target_type: event\.target\.value/);
  assert.match(
    tabSource,
    /!arraysEqual\([\s\S]*?draft\.upstream_policy_channel_ids,[\s\S]*?baseline\.upstream_policy_channel_ids/,
  );
  assert.match(
    tabSource,
    /!arraysEqual\([\s\S]*?draft\.upstream_policy_group_codes,[\s\S]*?baseline\.upstream_policy_group_codes/,
  );
  assert.doesNotMatch(
    tabSource,
    /upstream_policy_target_type: event\.target\.value,[\s\S]{0,180}upstream_policy_(?:channel_ids|group_codes): \[\]/,
  );
});

test('Classic 渠道和分组模式必须至少选择一项', () => {
  assert.match(tabSource, /请至少选择一个官方风控生效渠道/);
  assert.match(tabSource, /请至少选择一个官方风控生效分组/);
  assert.match(
    tabSource,
    /upstream_policy_target_type === TARGET_ALL \? \([\s\S]*?该策略对所有渠道生效[\s\S]*?: draft\.upstream_policy_target_type ===/,
  );
});
