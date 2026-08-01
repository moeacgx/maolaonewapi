import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

import {
  AUDIT_EVENT_ORIGIN_ASSIGNED,
  AUDIT_EVENT_ORIGIN_HISTORICAL,
  AUDIT_EVENT_ORIGIN_UNASSIGNED,
  getAuditEventChannelOrigin,
  getAuditEventChannelGroupsOrigin,
  getAuditEventRouteGroupOrigin,
} from './pages/SecurityAudit/event-origin.js';

const root = dirname(fileURLToPath(import.meta.url));
const readSource = (...parts) => readFileSync(resolve(root, ...parts), 'utf8');

test('Classic 审计事件显示已选渠道的名称和 ID', () => {
  assert.deepEqual(
    getAuditEventChannelOrigin({
      stage: 'response',
      channel_id: 42,
      channel_name: '生产渠道',
    }),
    {
      state: AUDIT_EVENT_ORIGIN_ASSIGNED,
      id: 42,
      name: '生产渠道',
    },
  );
});

test('Classic 区分选渠前事件和未记录渠道的历史事件', () => {
  assert.equal(
    getAuditEventChannelOrigin({
      stage: 'request',
      channel_id: 0,
      channel_name: '',
      channel_groups: [],
    }).state,
    AUDIT_EVENT_ORIGIN_UNASSIGNED,
  );
  assert.equal(
    getAuditEventChannelOrigin({ stage: 'response' }).state,
    AUDIT_EVENT_ORIGIN_HISTORICAL,
  );
  assert.equal(
    getAuditEventChannelOrigin({
      stage: 'response',
      channel_id: 0,
      channel_name: '',
    }).state,
    AUDIT_EVENT_ORIGIN_HISTORICAL,
  );
});

test('Classic 分开展示实际路由分组和渠道绑定分组', () => {
  const event = {
    channel_id: 42,
    channel_groups: [
      { id: 7, code: 'vip', name: '贵宾分组' },
      { id: 7, code: 'vip-old', name: '重复分组' },
      { id: 8, code: 'backup', name: '备用分组' },
    ],
    group_id: 99,
    group_name: '实际路由分组',
  };

  assert.deepEqual(getAuditEventRouteGroupOrigin(event), {
    state: AUDIT_EVENT_ORIGIN_ASSIGNED,
    items: [{ id: 99, code: '', name: '实际路由分组' }],
  });

  const result = getAuditEventChannelGroupsOrigin(event);
  assert.equal(result.state, AUDIT_EVENT_ORIGIN_ASSIGNED);
  assert.deepEqual(result.items, [
    { id: 7, code: 'vip', name: '贵宾分组' },
    { id: 8, code: 'backup', name: '备用分组' },
  ]);
});

test('Classic 渠道绑定分组为空时不会冒充实际路由分组', () => {
  assert.deepEqual(
    getAuditEventChannelGroupsOrigin({
      channel_id: 42,
      channel_groups: [],
      group_id: 3,
      group_name: '默认分组',
    }),
    {
      state: AUDIT_EVENT_ORIGIN_ASSIGNED,
      items: [],
    },
  );
});

test('Classic 列表和详情只渲染实际渠道与实际分组', () => {
  const source = readSource('pages/SecurityAudit/EventsTab.jsx');
  assert.match(source, /title: t\('渠道'\)[\s\S]*?renderChannelOrigin/);
  assert.match(source, /title: t\('分组'\)[\s\S]*?renderGroupOrigin/);
  assert.match(source, /\['渠道', renderChannelOrigin\(detail, t\)\]/);
  assert.match(source, /\['分组', renderGroupOrigin\(detail, t\)\]/);
  assert.doesNotMatch(source, /渠道绑定分组/);
  assert.match(source, /t\('尚未分配'\)/);
  assert.match(source, /t\('历史事件未记录'\)/);
});
