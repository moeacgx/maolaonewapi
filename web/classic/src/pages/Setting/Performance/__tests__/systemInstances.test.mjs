import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

import {
  SYSTEM_INSTANCE_POLL_INTERVAL_MS,
  formatBytes,
  formatPercent,
  getInstanceDisplayName,
  getInstanceRoleDescription,
  getInstanceRoleLabel,
  getInstanceRuntimeLabel,
  getInstanceStatusLabel,
  getInstanceStatusTagColor,
  getInstanceActiveRequests,
  getInstanceRpm,
  getSystemInstancesFromResponse,
  isStaleInstance,
  normalizePercent,
  summarizeInstanceTraffic,
  shouldConfigureNodeName,
} from '../systemInstances.js';

const __dirname = dirname(fileURLToPath(import.meta.url));

test('Classic 多节点面板读取系统实例列表而不是模型性能摘要', () => {
  const panelSource = readFileSync(
    resolve(__dirname, '../SystemInstancesPanel.jsx'),
    'utf8',
  );
  const settingsSource = readFileSync(
    resolve(__dirname, '../SettingsPerformance.jsx'),
    'utf8',
  );

  assert.match(panelSource, /\/api\/system-info\/instances/);
  assert.doesNotMatch(panelSource, /\/api\/perf-metrics\/summary/);
  assert.match(panelSource, /Active concurrency/);
  assert.match(panelSource, /Online RPM/);
  assert.match(settingsSource, /<SystemInstancesPanel \/>/);
  assert.equal(SYSTEM_INSTANCE_POLL_INTERVAL_MS, 30000);
});

test('系统实例响应只接受后端数组数据', () => {
  const response = {
    data: {
      success: true,
      data: [{ node_name: 'node-a' }],
    },
  };

  assert.deepEqual(getSystemInstancesFromResponse(response), [
    { node_name: 'node-a' },
  ]);
  assert.deepEqual(getSystemInstancesFromResponse({ data: {} }), []);
  assert.deepEqual(getSystemInstancesFromResponse(null), []);
});

test('节点显示名优先使用上报的稳定 NODE_NAME 并保留 hostname 提醒', () => {
  const instance = {
    node_name: 'fallback-node',
    info: {
      node: {
        name: 'stable-node',
        should_configure_manually: true,
      },
    },
  };

  assert.equal(getInstanceDisplayName(instance), 'stable-node');
  assert.equal(shouldConfigureNodeName(instance), true);
  assert.equal(
    getInstanceDisplayName({ node_name: 'fallback-node' }),
    'fallback-node',
  );
});

test('实例状态和删除权限只对失联实例生效', () => {
  assert.equal(getInstanceStatusLabel('online'), 'Online');
  assert.equal(getInstanceStatusTagColor('online'), 'green');
  assert.equal(isStaleInstance({ status: 'online' }), false);

  assert.equal(getInstanceStatusLabel('stale'), 'Stale');
  assert.equal(getInstanceStatusTagColor('stale'), 'orange');
  assert.equal(isStaleInstance({ status: 'stale' }), true);
});

test('节点角色和运行环境沿用后端实例心跳字段', () => {
  const master = {
    info: {
      role: { is_master: true },
      runtime: { goos: 'linux', goarch: 'amd64' },
    },
  };

  assert.equal(getInstanceRoleLabel(master), 'master');
  assert.equal(
    getInstanceRoleDescription(master),
    'Master instances run scheduled background tasks.',
  );
  assert.equal(getInstanceRuntimeLabel(master), 'linux/amd64');
  assert.equal(getInstanceRoleLabel({}), 'worker');
  assert.equal(getInstanceRuntimeLabel({}), '-');
});

test('资源数值格式化为稳定的百分比和字节单位', () => {
  assert.equal(normalizePercent(-10), 0);
  assert.equal(normalizePercent(120), 100);
  assert.equal(normalizePercent(Number.NaN), null);
  assert.equal(formatPercent(12.34), '12.3%');
  assert.equal(formatPercent(undefined), '-');
  assert.equal(formatBytes(0), '0 Bytes');
  assert.equal(formatBytes(1536), '1.5 KB');
});

test('多节点流量摘要只汇总在线实例的 RPM 和当前并发', () => {
  const instances = [
    { status: 'online', info: { metrics: { rpm: 12, active_requests: 3 } } },
    { status: 'online', info: { metrics: { rpm: 8, active_requests: 2 } } },
    { status: 'stale', info: { metrics: { rpm: 100, active_requests: 50 } } },
  ];

  assert.equal(getInstanceRpm(instances[0]), 12);
  assert.equal(getInstanceActiveRequests(instances[0]), 3);
  assert.deepEqual(summarizeInstanceTraffic(instances), {
    rpm: 20,
    activeRequests: 5,
    onlineInstances: 2,
    instancesWithMetrics: 2,
  });
});
