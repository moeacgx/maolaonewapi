import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { cleanSecurityAuditFilter } from './pages/SecurityAudit/eventFilter.js';

const root = dirname(fileURLToPath(import.meta.url));
const source = readFileSync(
  resolve(root, 'pages/SecurityAudit/EventsTab.jsx'),
  'utf8',
);
test('Classic 安全审计按用户名筛选且不再提交用户 ID', () => {
  assert.match(source, /username: ''/);
  assert.match(
    source,
    /<Input\s+[\s\S]*?value=\{filter\.username\}[\s\S]*?placeholder=\{t\('用户名'\)\}[\s\S]*?username: value/,
  );
  assert.doesNotMatch(source, /filter\.user_id|user_id: undefined/);
  assert.match(source, /value=\{filter\.username\}[\s\S]*?maxLength=\{128\}/);
});

test('Classic 用户名条件复用于列表查询和筛选删除', () => {
  assert.match(
    source,
    /getSecurityAuditEvents\(\s*appliedFilter,\s*page,\s*pageSize/,
  );
  assert.match(source, /cleanSecurityAuditFilter\(appliedFilter\)/);
  assert.match(source, /previewSecurityAuditDelete\(filterSnapshot\)/);
  assert.match(
    source,
    /deleteSecurityAuditEventsByFilter\(filterSnapshot, preview\)/,
  );
  assert.deepEqual(
    cleanSecurityAuditFilter({
      username: '  audit-reviewer  ',
      decision: ' flag ',
      token_id: 0,
    }),
    { username: 'audit-reviewer', decision: 'flag' },
  );
  assert.deepEqual(cleanSecurityAuditFilter({ username: '   ' }), {});
});

test('Classic 处理结果条件复用于列表查询和筛选删除', () => {
  assert.match(source, /action: ''/);
  assert.match(source, /value=\{filter\.action \|\| undefined\}/);
  assert.match(source, /dataIndex: 'action'/);
  assert.deepEqual(
    cleanSecurityAuditFilter({ action: ' block ', username: ' ' }),
    { action: 'block' },
  );
});
