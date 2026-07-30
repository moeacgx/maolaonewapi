import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';

const root = dirname(fileURLToPath(import.meta.url));

const readSource = (relativePath) =>
  readFileSync(resolve(root, relativePath), 'utf8');

test('VChart Semi 主题监听在 Classic 前端全局只初始化一次', () => {
  const calls = [];
  const source = readSource('helpers/vchartTheme.js')
    .replace(
      /import \{ initVChartSemiTheme \} from '@visactor\/vchart-semi-theme';/,
      '',
    )
    .replace(
      'export const ensureVChartSemiTheme',
      'const ensureVChartSemiTheme',
    )
    .concat('\nglobalThis.ensureVChartSemiTheme = ensureVChartSemiTheme;');
  const context = vm.createContext({
    initVChartSemiTheme: (options) => calls.push(options),
  });

  vm.runInContext(source, context);
  context.ensureVChartSemiTheme();
  context.ensureVChartSemiTheme();

  assert.equal(calls.length, 1);
  assert.equal(calls[0].isWatchingThemeSwitch, true);
});

test('Classic 图表页面统一复用幂等主题初始化入口', () => {
  const consumers = [
    'hooks/dashboard/useDashboardCharts.jsx',
    'components/table/model-pricing/performance/PerformanceCharts.jsx',
    'pages/ChannelObservability/OverviewView.jsx',
  ];

  for (const consumer of consumers) {
    const source = readSource(consumer);
    assert.match(source, /import \{ ensureVChartSemiTheme \}/, consumer);
    assert.match(source, /ensureVChartSemiTheme\(\)/, consumer);
    assert.doesNotMatch(source, /initVChartSemiTheme/, consumer);
  }
});

test('仪表盘标签切换复用同一个 VChart 实例', () => {
  const source = readSource('components/dashboard/ChartsPanel.jsx');
  const chartInstances = source.match(/<VChart\b/g) ?? [];

  assert.equal(chartInstances.length, 1);
  assert.match(source, /const activeSpec =/);
  assert.match(
    source,
    /activeSpec && <VChart spec=\{activeSpec\} options=\{CHART_CONFIG\} \/>/,
  );
  assert.doesNotMatch(source, /<VChart[^>]*\boption=/);
});
