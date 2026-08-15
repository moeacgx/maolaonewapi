import assert from 'node:assert/strict';
import test from 'node:test';

import {
  invertBooleanOptionValue,
  normalizeBooleanOptionValue,
} from './boolean.js';

test('Classic 设置把 Enabled 和 Disabled 后缀的字符串值解析为布尔值', () => {
  assert.equal(normalizeBooleanOptionValue('InvoiceEnabled', 'false'), false);
  assert.equal(
    normalizeBooleanOptionValue('InvoiceDiscountDisabled', 'false'),
    false,
  );
  assert.equal(
    normalizeBooleanOptionValue('InvoiceDiscountDisabled', 'true'),
    true,
  );
  assert.equal(normalizeBooleanOptionValue('ServerAddress', 'false'), 'false');
});

test('正向折扣开关与底层 Disabled 配置保持反向映射', () => {
  assert.equal(invertBooleanOptionValue(false), true);
  assert.equal(invertBooleanOptionValue(true), false);
  assert.equal(invertBooleanOptionValue('false'), true);
  assert.equal(invertBooleanOptionValue('true'), false);
});
