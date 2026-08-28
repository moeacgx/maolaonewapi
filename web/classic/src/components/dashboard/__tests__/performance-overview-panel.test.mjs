import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));

const readSource = (relativePath) =>
  readFileSync(resolve(root, relativePath), 'utf8');

test('Classic 控制台性能概览面板保留摘要、状态和明细契约', () => {
  const panelSource = readSource('../PerformanceOverviewPanel.jsx');

  assert.match(panelSource, /API\.get\('\/api\/perf-metrics\/summary'/);
  assert.match(
    panelSource,
    /params:\s*\{\s*hours:\s*PERFORMANCE_WINDOW_HOURS\s*\}/,
  );
  assert.match(panelSource, /PERFORMANCE_WINDOW_HOURS\s*=\s*24/);
  assert.match(panelSource, /useState\(true\)/);
  assert.match(panelSource, /const \[failed, setFailed\] = useState\(false\)/);
  assert.match(panelSource, /setFailed\(true\)/);
  assert.match(panelSource, /setLoading\(false\)/);
  assert.match(panelSource, /AbortController/);
  assert.match(panelSource, /暂无性能数据/);
  assert.match(panelSource, /加载失败/);
  assert.match(panelSource, /failed \|\| rows\.length === 0/);
  assert.match(panelSource, /request_count/);
  assert.match(panelSource, /avg_ttft_ms/);
  assert.match(panelSource, /平均首 Token 延迟/);
  assert.match(panelSource, /首 Token 延迟/);
  assert.match(panelSource, /getSuccessRateTextColor/);
  assert.match(panelSource, /formatLatency/);
  assert.match(panelSource, /formatThroughput/);
  assert.match(panelSource, /TimerReset/);
});

test('Classic 首页把性能概览放在统计卡片后、收入面板前且限管理员', () => {
  const dashboardSource = readSource('../index.jsx');
  const statsIndex = dashboardSource.indexOf('<StatsCards');
  const performanceIndex = dashboardSource.indexOf(
    '<PerformanceOverviewPanel',
  );
  const revenueIndex = dashboardSource.indexOf('<RevenuePanel');

  assert.match(dashboardSource, /import PerformanceOverviewPanel/);
  assert.ok(statsIndex >= 0, '应挂载 StatsCards');
  assert.ok(
    performanceIndex > statsIndex,
    '性能概览应位于 StatsCards 之后',
  );
  assert.ok(
    revenueIndex > performanceIndex,
    '性能概览应位于 RevenuePanel 之前',
  );

  const performanceBlock = dashboardSource.slice(
    performanceIndex - 160,
    revenueIndex,
  );
  assert.match(performanceBlock, /dashboardData\.isAdminUser/);
});
