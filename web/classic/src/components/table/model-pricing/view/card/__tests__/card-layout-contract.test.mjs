import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));

const readSource = (relativePath) =>
  readFileSync(resolve(root, relativePath), 'utf8');

const sectionBetween = (source, startMarker, endMarker) => {
  const start = source.indexOf(startMarker);
  assert.notEqual(start, -1, `missing start marker: ${startMarker}`);

  const end = source.indexOf(endMarker, start);
  assert.notEqual(end, -1, `missing end marker: ${endMarker}`);

  return source.slice(start, end);
};

test('Classic 模型卡片页脚不再渲染分组，只保留计费类型和性能摘要', () => {
  const source = readSource('../PricingCardView.jsx');
  const footer = sectionBetween(
    source,
    "<div className='classic-pricing-model-card-footer'>",
    '{showRatio &&',
  );

  assert.doesNotMatch(footer, /getGroupDisplayName/);
  assert.doesNotMatch(footer, /resolveCardDisplayedGroup/);
  assert.doesNotMatch(footer, /classic-pricing-card-group/);
  assert.doesNotMatch(footer, /displayedGroup/);

  assert.match(footer, /renderBillingTag\(model\)/);
  assert.match(
    footer,
    /<ModelPerformanceBadge[\s\S]*?performance=\{performanceMap\[model\.model_name\]\}/,
  );
  assert.doesNotMatch(footer, /classic-pricing-card-metadata/);
  assert.doesNotMatch(footer, /supported_endpoint_types|model\.tags/);

  assert.doesNotMatch(source, /renderCardMetadata|getCardUnitLabel/);
  assert.doesNotMatch(source, /classic-pricing-discount-tag|<Tag/);
  assert.doesNotMatch(source, /card-display/);
  assert.doesNotMatch(source, /getGroupTextColor/);
  assert.doesNotMatch(source, /groupNames/);
});

test('Classic 模型卡片详情保留灰色分组并隐藏模型标签', () => {
  const detailSource = readSource(
    '../../../modal/components/ModelBasicInfo.jsx',
  );
  const stylesheet = readSource('../../../../../../index.css');

  assert.match(detailSource, /getGroupDisplayName/);
  assert.match(detailSource, /classic-pricing-detail-pill/);
  assert.doesNotMatch(detailSource, /getGroupTextColor/);
  assert.match(detailSource, /supported_endpoint_types/);
  assert.doesNotMatch(detailSource, /modelData\.tags|模型标签/);
  assert.match(stylesheet, /--classic-pricing-group-color/);
});

test('Classic 模型卡片计费类型向左收拢，与性能摘要共享底部基线', () => {
  const source = readSource('../PricingCardView.jsx');
  const stylesheet = readSource('../../../../../../index.css');

  const billingBlock = sectionBetween(
    source,
    "<div className='classic-pricing-model-card-billing'>",
    '</div>',
  );
  assert.doesNotMatch(billingBlock, /justify-content|justify-end/);

  assert.match(
    stylesheet,
    /\.classic-pricing-model-card-footer-info\s*\{[^}]*align-items:\s*flex-end[^}]*\}/,
  );
  assert.match(
    stylesheet,
    /\.classic-pricing-model-card-billing\s*\{[^}]*display:\s*flex[^}]*align-items:\s*center[^}]*\}/,
  );
});

test('Classic 模型卡片计费类型文案覆盖按量、按次、按秒和动态计费，并保留 i18n', () => {
  const source = readSource('../PricingCardView.jsx');

  const renderBillingTag = sectionBetween(
    source,
    'const renderBillingTag = (record) => {',
    '};',
  );

  assert.match(renderBillingTag, /t\(\s*['"]按量计费['"]\s*\)/);
  assert.match(renderBillingTag, /['"]按次计费['"]/);
  assert.match(renderBillingTag, /['"]按秒计费['"]/);
  assert.match(renderBillingTag, /t\(\s*['"]动态计费['"]\s*\)/);
  assert.match(
    renderBillingTag,
    /label = t\(\s*isModelPriceUnitSecond\(record\.model_price_unit\)/,
  );

  assert.match(renderBillingTag, /quota_type === 1/);
  assert.match(renderBillingTag, /quota_type === 0/);
  assert.match(renderBillingTag, /billing_mode === ['"]tiered_expr['"]/);

  assert.match(renderBillingTag, /label = ['"]-['"];/);
  assert.match(
    renderBillingTag,
    /billingMode = ['"]classic-pricing-billing-mode-neutral['"];/,
  );
});
