import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

import { getBillingDiscountText } from './billing/utils.js';

const root = dirname(fileURLToPath(import.meta.url));

const readSource = (relativePath) =>
  readFileSync(resolve(root, relativePath), 'utf8');

const cssBlocks = (stylesheet, selector) =>
  Array.from(
    stylesheet.matchAll(
      new RegExp(`${selector.replace(/[.*+?^${}()|[\]\\]/gu, '\\$&')}\\s*\\{([^}]*)\\}`, 'gu'),
    ),
  ).map((match) => match[1]);

test('模型卡片页脚让计费行与性能摘要底部对齐且不恢复噪声字段', () => {
  const cardSource = readSource('view/card/PricingCardView.jsx');
  const stylesheet = readSource('../../../index.css');

  assert.ok(
    cssBlocks(stylesheet, '.classic-pricing-model-card-footer-info').some(
      (block) => /align-items:\s*(?:end|flex-end);/u.test(block),
    ),
    '卡片页脚网格必须让计费行与性能摘要底部对齐',
  );
  assert.doesNotMatch(cardSource, /classic-pricing-card-metadata/u);
  assert.doesNotMatch(cardSource, /supported_endpoint_types|model\.tags/u);
  assert.doesNotMatch(cardSource, /classic-pricing-discount-tag|<Tag/u);
});

test('详情按分组定价不重复展开 variant rules，并给自动分组使用稳定颜色', () => {
  const pricingSource = readSource('modal/components/ModelPricingTable.jsx');

  assert.doesNotMatch(pricingSource, /includeVariantRules:\s*true/u);
  assert.match(pricingSource, /includeVariantRules:\s*false/u);
  assert.match(
    pricingSource,
    /classic-pricing-detail-auto-chain[\s\S]*?getGroupTextColor\(group\)/u,
  );
  assert.match(
    pricingSource,
    /getBillingDiscountText\(row\.discountFactor,\s*t\)/u,
  );
});

test('详情倍率调整沿用折与倍的语义文案', () => {
  const translate = (key, values) =>
    key.replace('{{discount}}', values?.discount ?? '').replace(
      '{{ratio}}',
      values?.ratio ?? '',
    );

  assert.equal(getBillingDiscountText(0.5, translate), '5折');
  assert.equal(getBillingDiscountText(1.25, translate), '1.25倍');
});
