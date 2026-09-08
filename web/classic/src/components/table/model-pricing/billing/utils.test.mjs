/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import assert from 'node:assert/strict';
import * as billingUtils from './utils.js';
import {
  calculateTokenCost,
  formatDynamicUnitPrice,
  getBillingDiscountColor,
  getBillingDiscountText,
  getBillingDynamicUnitPrices,
  getBillingExpressionInfo,
  getBillingExpressionTier,
  getBillingFactors,
  getBillingGuideGroups,
  getBillingGuideModels,
  getBillingGuideStorage,
  getBillingUnitPricesFromPriceData,
  getDetailBillingBadgeClassName,
  getDetailBillingBadgeVariant,
  getDynamicFormattedPricesByTier,
  getDynamicPriceFieldsFromTiers,
  hasBillingPriceAdjustment,
  hasSeenBillingGuide,
  isPrimaryDynamicPriceField,
  markBillingGuideSeen,
  parseBillingPrice,
  pickBillingGuideGroup,
  pickBillingGuideModel,
} from './utils.js';

const models = [
  {
    model_name: 'fixed-image',
    quota_type: 1,
    model_price: 0.1,
  },
  {
    model_name: 'gpt-5.6-sol',
    quota_type: 0,
    billing_mode: 'tiered_expr',
    model_ratio: 1,
    billing_expr:
      'len <= 272000 ? tier("standard", p * 0.5 + c * 3 + cr * 0.05) : tier("long_context", p * 1 + c * 6 + cr * 0.1)',
  },
  {
    model_name: 'invalid-dynamic',
    quota_type: 0,
    billing_mode: 'tiered_expr',
    model_ratio: 1,
    billing_expr: 'tier("invalid", p * max(1, 2))',
  },
  {
    model_name: 'empty-dynamic',
    quota_type: 0,
    billing_mode: 'tiered_expr',
    model_ratio: 1,
    billing_expr: '',
  },
  {
    model_name: 'unpriced',
    quota_type: 0,
    model_ratio: null,
  },
  {
    model_name: 'gpt-5.5',
    quota_type: 0,
    model_ratio: 1.25,
    completion_ratio: 6,
    cache_ratio: 0.1,
    create_cache_ratio: 1.25,
    enable_groups: ['auto', 'standard', 'discount'],
  },
];

assert.deepEqual(getBillingGuideModels(models), [models[1], models[5]]);
assert.equal(pickBillingGuideModel(models)?.model_name, 'gpt-5.5');

const dynamicInfo = getBillingExpressionInfo(models[1].billing_expr);
assert.equal(dynamicInfo.supported, true);
assert.equal(dynamicInfo.tiers.length, 2);
assert.equal(
  getBillingExpressionTier(dynamicInfo, {
    input: 1024,
    output: 500,
    cacheRead: 155,
  }).label,
  'standard',
);
assert.equal(
  getBillingExpressionTier(dynamicInfo, {
    input: 300000,
    output: 500,
    cacheRead: 155,
  }).label,
  'long_context',
);

const dynamicPrices = getBillingDynamicUnitPrices({
  priceData: {
    isDynamicPricing: true,
    billingExpr: models[1].billing_expr,
    usedGroupRatio: 0.5,
  },
  tokenCounts: { input: 1024, output: 500, cacheRead: 155 },
  displayPrice: (value) => `$${value.toFixed(4)}`,
});
assert.equal(dynamicPrices.dynamicTierLabel, 'standard');
assert.equal(dynamicPrices.input.unitPrice, 0.25);
assert.equal(dynamicPrices.output.unitPrice, 1.5);
assert.equal(dynamicPrices.cacheRead.unitPrice, 0.025);

const groups = getBillingGuideGroups(models[5], {
  standard: 1,
  discount: 0.5,
});
assert.deepEqual(groups, [
  { value: 'standard', ratio: 1 },
  { value: 'discount', ratio: 0.5 },
]);
assert.deepEqual(
  pickBillingGuideGroup(models[5], { standard: 1, discount: 0.5 }),
  { value: 'discount', ratio: 0.5 },
);

const factors = getBillingFactors({
  groupRatio: 0.5,
  priceRate: 1,
  usdExchangeRate: 7,
});
assert.equal(factors.forexFactor, 1 / 7);
assert.equal(factors.compositeFactor, 1 / 14);

const translate = (key, values = {}) =>
  key.replace(/\{\{(\w+)\}\}/gu, (_matched, name) => values[name]);
assert.equal(getBillingDiscountText(1 / 14, translate), '0.7折');
const exampleFactors = getBillingFactors({
  groupRatio: 0.15,
  priceRate: 1.03,
  usdExchangeRate: 6.71,
});
assert.equal(
  getBillingDiscountText(exampleFactors.compositeFactor, translate, 2),
  '0.23折',
);
assert.equal(getBillingDiscountText(1, translate), '原价');
assert.equal(getBillingDiscountText(1.25, translate), '1.25倍');
assert.equal(getBillingDiscountColor(0.4999), 'red');
assert.equal(getBillingDiscountColor(0.5), 'green');
assert.equal(hasBillingPriceAdjustment(0.9995), false);
assert.equal(hasBillingPriceAdjustment(1.0005), false);
assert.equal(hasBillingPriceAdjustment(0.8), true);
assert.equal(hasBillingPriceAdjustment(1.25), true);

assert.equal(typeof billingUtils.getBillingPriceRelation, 'function');
assert.equal(billingUtils.getBillingPriceRelation(0.8, 1), 'discount');
assert.equal(billingUtils.getBillingPriceRelation(1, 1), 'same');
assert.equal(billingUtils.getBillingPriceRelation(1.25, 1), 'markup');

const prices = getBillingUnitPricesFromPriceData({
  priceData: {
    isPerToken: true,
    inputPrice: '$1.2500',
    originalInputPrice: '$2.5000',
    completionPrice: '$7.5000',
    originalCompletionPrice: '$15.0000',
    cachePrice: '$0.1250',
    originalCachePrice: '$0.2500',
    createCachePrice: '$1.5625',
    originalCreateCachePrice: '$3.1250',
  },
  currency: 'USD',
});
assert.equal(prices.input.unitPrice, 1.25);
assert.equal(prices.output.unitPrice, 7.5);
assert.equal(prices.cacheRead.unitPrice, 0.125);
assert.equal(prices.cacheWrite.unitPrice, 1.5625);
assert.ok(
  Math.abs(calculateTokenCost(1024, prices.input.unitPrice) - 0.00128) < 1e-12,
);

const rechargePrices = getBillingUnitPricesFromPriceData({
  priceData: {
    isPerToken: true,
    inputPrice: '¥1.2500',
    originalInputPrice: '¥2.5000',
  },
  currency: 'CNY',
  usdExchangeRate: 7,
});
assert.equal(rechargePrices.input.unitPrice, 1.25);
assert.equal(rechargePrices.input.officialPrice, 17.5);
assert.equal(parseBillingPrice('EUR1.25'), 1.25);
assert.equal(parseBillingPrice('AED 1,234.50'), 1234.5);
assert.equal(parseBillingPrice('价格待定'), null);

const storageValues = new Map();
const storage = {
  getItem: (key) => storageValues.get(key),
  setItem: (key, value) => storageValues.set(key, value),
};
assert.equal(hasSeenBillingGuide(storage), false);
assert.equal(markBillingGuideSeen(storage), true);
assert.equal(hasSeenBillingGuide(storage), true);

const throwingStorage = {
  getItem: () => {
    throw new Error('blocked');
  },
  setItem: () => {
    throw new Error('blocked');
  },
};
assert.equal(hasSeenBillingGuide(throwingStorage), false);
assert.equal(markBillingGuideSeen(throwingStorage), false);
assert.equal(markBillingGuideSeen(undefined), false);
assert.equal(
  getBillingGuideStorage({
    get localStorage() {
      throw new Error('blocked');
    },
  }),
  undefined,
);

const displayPrice = (value) => `$${Number(value).toFixed(3)}`;
const claudeTiers = [
  {
    label: 'base',
    inputPrice: 10,
    outputPrice: 50,
    cacheReadPrice: 0.25,
    cacheCreatePrice: 12.5,
    cacheCreate1hPrice: 20,
    imagePrice: 0,
  },
];
const claudePriceVars = [
  { key: 'p', field: 'inputPrice', shortLabel: '输入' },
  { key: 'c', field: 'outputPrice', shortLabel: '补全' },
  { key: 'cr', field: 'cacheReadPrice', shortLabel: '缓存读' },
  { key: 'cc', field: 'cacheCreatePrice', shortLabel: '缓存创建' },
  { key: 'cc1h', field: 'cacheCreate1hPrice', shortLabel: '1h缓存创建' },
  { key: 'img', field: 'imagePrice', shortLabel: '图片输入' },
];

assert.equal(
  formatDynamicUnitPrice({
    valuePerMillionTokens: 10,
    groupRatio: 0.9,
    displayPrice,
  }),
  '$9',
);
assert.equal(
  formatDynamicUnitPrice({
    valuePerMillionTokens: 0.25,
    groupRatio: 0.9,
    displayPrice,
  }),
  '$0.225',
);
assert.equal(
  formatDynamicUnitPrice({
    valuePerMillionTokens: 12.5,
    groupRatio: 0.9,
    displayPrice,
  }),
  '$11.25',
);
assert.equal(
  formatDynamicUnitPrice({
    valuePerMillionTokens: 10,
    groupRatio: 0.5,
    displayPrice,
  }),
  '$5',
);
assert.equal(
  formatDynamicUnitPrice({
    valuePerMillionTokens: 10,
    groupRatio: 0.9,
    tokenUnit: 'K',
    displayPrice,
  }),
  '$0.009',
);
assert.equal(
  formatDynamicUnitPrice({
    valuePerMillionTokens: 0,
    groupRatio: 0.9,
    displayPrice,
  }),
  '',
);
assert.equal(
  formatDynamicUnitPrice({
    valuePerMillionTokens: 10,
    groupRatio: 0.5,
  }),
  '$5',
);

const dynamicFields = getDynamicPriceFieldsFromTiers(
  claudeTiers,
  claudePriceVars,
);
assert.deepEqual(
  dynamicFields.map((variable) => variable.field),
  [
    'inputPrice',
    'outputPrice',
    'cacheReadPrice',
    'cacheCreatePrice',
    'cacheCreate1hPrice',
  ],
);
assert.equal(isPrimaryDynamicPriceField('inputPrice'), true);
assert.equal(isPrimaryDynamicPriceField('cacheReadPrice'), false);

const discountedClaudePrices = getDynamicFormattedPricesByTier(
  claudeTiers,
  dynamicFields,
  { groupRatio: 0.5, displayPrice },
);
assert.equal(discountedClaudePrices[0].prices.inputPrice, '$5');
assert.equal(discountedClaudePrices[0].prices.outputPrice, '$25');
assert.equal(discountedClaudePrices[0].prices.cacheReadPrice, '$0.125');
assert.equal(discountedClaudePrices[0].prices.cacheCreatePrice, '$6.25');
assert.equal(discountedClaudePrices[0].prices.cacheCreate1hPrice, '$10');
assert.equal(
  getDetailBillingBadgeVariant({ billing_mode: 'tiered_expr', quota_type: 0 }),
  'warning',
);
assert.equal(getDetailBillingBadgeVariant({ quota_type: 0 }), 'info');
assert.equal(getDetailBillingBadgeVariant({ quota_type: 1 }), 'purple');
assert.equal(
  getDetailBillingBadgeVariant({ quota_type: 1, model_price_unit: 'second' }),
  'neutral',
);
assert.match(
  getDetailBillingBadgeClassName({ billing_mode: 'tiered_expr' }),
  /classic-pricing-detail-billing-badge-warning/,
);

assert.deepEqual(getDynamicPriceFieldsFromTiers([], claudePriceVars), []);
assert.deepEqual(
  getDynamicFormattedPricesByTier([], dynamicFields, { displayPrice }),
  [],
);

console.log('billing utils tests passed');
