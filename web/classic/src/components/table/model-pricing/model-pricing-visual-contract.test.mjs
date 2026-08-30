import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));

test('模型详情性能分组保留稳定色且供应商分组使用灰色 pill', () => {
  const performancePanel = readFileSync(
    resolve(root, 'performance/ModelPerformancePanel.jsx'),
    'utf8',
  );
  const pricingTable = readFileSync(
    resolve(root, 'modal/components/ModelPricingTable.jsx'),
    'utf8',
  );
  const basicInfo = readFileSync(
    resolve(root, 'modal/components/ModelBasicInfo.jsx'),
    'utf8',
  );

  assert.doesNotMatch(performancePanel, /Tag color='blue'/);
  assert.match(performancePanel, /getGroupSemanticColor\(row\.group\)/);
  assert.match(pricingTable, /getGroupTextColor\(row\.group\)/);
  assert.match(basicInfo, /className='classic-pricing-detail-pill'/);
  assert.doesNotMatch(basicInfo, /getGroupTextColor\(group\)/);
  assert.doesNotMatch(basicInfo, /classic-pricing-detail-group-pill/);
  assert.doesNotMatch(basicInfo, /模型标签|modelData\.tags/);
});

test('模型详情成功率和折扣徽标使用语义色而不是固定橙色', () => {
  const performancePanel = readFileSync(
    resolve(root, 'performance/ModelPerformancePanel.jsx'),
    'utf8',
  );
  const basicInfo = readFileSync(
    resolve(root, 'modal/components/ModelBasicInfo.jsx'),
    'utf8',
  );
  const sparkline = readFileSync(
    resolve(root, 'performance/SuccessRateSparkline.jsx'),
    'utf8',
  );
  const stylesheet = readFileSync(resolve(root, '../../../index.css'), 'utf8');

  assert.match(
    performancePanel,
    /getSuccessRateTextColor\(view\.successRate\)/,
  );
  assert.match(
    basicInfo,
    /getSuccessRateTextColor\(performance\?\.success_rate\)/,
  );
  assert.match(sparkline, /getSuccessRateTextColor\(computedOverall\)/);
  assert.doesNotMatch(
    stylesheet,
    /\.classic-pricing-detail-discount-badge\s*\{\s*color:\s*var\(--semi-color-warning\);/,
  );
  assert.match(stylesheet, /--classic-pricing-group-color/);
});
