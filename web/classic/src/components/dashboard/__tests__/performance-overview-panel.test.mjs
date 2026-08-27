import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));

test('Classic 控制台性能概览卡独立拉取摘要并展示节点调用量', () => {
  const source = readFileSync(
    resolve(root, '../PerformanceOverviewPanel.jsx'),
    'utf8',
  );

  assert.match(source, /API\.get\('\/api\/perf-metrics\/summary'/);
  assert.match(source, /skipErrorHandler:\s*true/);
  assert.match(source, /PERFORMANCE_WINDOW_HOURS\s*=\s*24/);
  assert.match(source, /request_count/);
  assert.match(source, /avg_ttft_ms/);
  assert.match(source, /调用量/);
  assert.match(source, /模型广场性能统计/);
  assert.match(source, /平均延迟、首 Token 延迟、TPS 和成功率/);
  assert.match(source, /平均首 Token 延迟/);
  assert.match(source, /首 Token 延迟/);
  assert.match(source, /暂无性能数据/);
  assert.match(source, /getSuccessRateTextColor/);
  assert.match(source, /formatLatency/);
  assert.match(source, /formatThroughput/);
  assert.match(source, /TimerReset/);
});
