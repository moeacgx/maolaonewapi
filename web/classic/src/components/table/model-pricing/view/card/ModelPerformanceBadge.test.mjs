import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));

test('Classic 模型卡片把性能摘要放在右侧并隐藏时间与百分比', () => {
  const source = readFileSync(
    resolve(root, 'ModelPerformanceBadge.jsx'),
    'utf8',
  );
  const sparklineSource = readFileSync(
    resolve(root, '../../performance/SuccessRateSparkline.jsx'),
    'utf8',
  );
  const stylesheet = readFileSync(
    resolve(root, '../../../../../index.css'),
    'utf8',
  );

  assert.match(source, /classic-pricing-model-performance-badge/);
  assert.match(source, /getSuccessRateTextColor\(statusRate\)/);
  assert.match(source, /formatSuccessRate\(statusRate\)/);
  assert.match(source, /classic-pricing-model-performance-status-label/);
  assert.match(source, /performance\.status_rate/);
  assert.match(source, /showOverall=\{false\}/);
  assert.match(source, /availabilityTone/);
  assert.match(source, /signalStyle/);
  assert.match(source, /aggregateWindow/);
  assert.match(source, /maxPoints=\{3\}/);
  assert.match(source, /replace\(' t\/s', 't'\)/);
  assert.match(stylesheet, /\.classic-pricing-model-performance-badge/);
  assert.match(stylesheet, /width:\s*132px/);
  assert.match(stylesheet, /--classic-pricing-performance-status-color/);
  assert.match(sparklineSource, /const gap = signalStyle\s*\?\s*'gap-0\.5'/);
  assert.match(sparklineSource, /signalStyle \? 'rounded-full'/);
  assert.match(sparklineSource, /8 \+ Math\.min\(index, 2\) \* 2/);
  assert.doesNotMatch(source, /latestTimestamp=/);
});

test('Classic 模型卡片在只有聚合状态率时保留语义色状态信号', () => {
  const source = readFileSync(
    resolve(root, 'ModelPerformanceBadge.jsx'),
    'utf8',
  );
  const stylesheet = readFileSync(
    resolve(root, '../../../../../index.css'),
    'utf8',
  );

  assert.match(source, /const hasStatusSeries = statusSeries\.length > 0/);
  assert.match(source, /hasStatusSeries \? \(/);
  assert.match(source, /aria-label=\{statusRateText\}/);
  assert.match(source, /classic-pricing-model-performance-status-fallback/);
  assert.match(
    stylesheet,
    /\.classic-pricing-model-performance-status-fallback > span/,
  );
  assert.match(stylesheet, /--classic-pricing-performance-status-color/);
});

test('Classic 性能详情保留 24 个原始历史点', () => {
  const panelSource = readFileSync(
    resolve(root, '../../performance/ModelPerformancePanel.jsx'),
    'utf8',
  );
  const sparklineSource = readFileSync(
    resolve(root, '../../performance/SuccessRateSparkline.jsx'),
    'utf8',
  );

  assert.match(panelSource, /maxPoints=\{24\}/);
  assert.match(sparklineSource, /aggregateWindow = false/);
  assert.match(sparklineSource, /normalizePerformanceSeries\(series\)\.slice/);
});

test('Classic 模型卡片性能徽标手机端底色透明，桌面端底色规则保持不变', () => {
  const stylesheet = readFileSync(
    resolve(root, '../../../../../index.css'),
    'utf8',
  );

  assert.match(
    stylesheet,
    /@media \(max-width:\s*459px\)\s*\{\s*\.classic-pricing-model-performance-badge\s*\{[^}]*background:\s*transparent[^}]*\}/,
  );
  assert.match(
    stylesheet,
    /@media \(min-width:\s*460px\)\s*\{\s*\.classic-pricing-model-performance-badge\s*\{[^}]*background:\s*transparent[^}]*\}/,
  );
});
