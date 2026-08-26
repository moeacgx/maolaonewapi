import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

import { getGroupDisplayName } from '../../../../../../helpers/groupDetails.js';
import { resolveCardDisplayedGroup } from '../card-display.js';

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

test('Classic 模型卡片只在介绍下保留统一的分组、计费和性能信息', () => {
  const source = readSource('../PricingCardView.jsx');
  const footer = sectionBetween(
    source,
    "<div className='classic-pricing-model-card-footer'>",
    '{showRatio &&',
  );

  assert.match(footer, /getGroupDisplayName\(displayedGroup, groupNames\)/);
  assert.match(footer, /renderBillingTag\(model\)/);
  assert.match(
    footer,
    /<ModelPerformanceBadge[\s\S]*?performance=\{performanceMap\[model\.model_name\]\}/,
  );
  assert.doesNotMatch(footer, /classic-pricing-card-metadata/);
  assert.doesNotMatch(footer, /supported_endpoint_types|model\.tags/);

  assert.doesNotMatch(source, /renderCardMetadata|getCardUnitLabel/);
  assert.doesNotMatch(source, /classic-pricing-discount-tag|<Tag/);
});

test('Classic 模型卡片分组名称映射和详情能力信息各自保留', () => {
  const cardSource = readSource('../PricingCardView.jsx');
  const detailSource = readSource('../../../modal/components/ModelBasicInfo.jsx');
  const stylesheet = readSource('../../../../../../index.css');

  assert.equal(
    getGroupDisplayName('group_1', { group_1: '高级分组' }),
    '高级分组',
  );
  assert.equal(getGroupDisplayName('legacy-code', {}), 'legacy-code');
  assert.match(cardSource, /getGroupTextColor\(\s*displayedGroup/);
  assert.match(stylesheet, /--classic-pricing-group-color/);
  assert.match(detailSource, /supported_endpoint_types/);
  assert.match(detailSource, /modelData\.tags/);
});

test('Classic 模型卡片只显示当前有效分组，不展示 all、auto 或多分组计数', () => {
  const source = readSource('../PricingCardView.jsx');

  assert.equal(
    resolveCardDisplayedGroup('all', ['all', 'auto', ' group_2 ']),
    'group_2',
  );
  assert.equal(resolveCardDisplayedGroup('auto', ['all', 'auto']), undefined);
  assert.equal(resolveCardDisplayedGroup('group_3', ['group_2']), 'group_3');
  assert.match(source, /resolveCardDisplayedGroup\(\s*priceData\.usedGroup/);
  assert.doesNotMatch(source, /\+\{hiddenCount\}/);
});
