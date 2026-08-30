import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const source = readFileSync(
  new URL('../usePromoCodesData.jsx', import.meta.url),
  'utf8',
);

test('优惠码数据钩子只调用后端实际的 /api/promo_code/ 路由', () => {
  assert.match(source, /\/api\/promo_code\//);
  assert.doesNotMatch(source, /\/api\/promo-code\//);
  assert.match(source, /API\.delete\(`\/api\/promo_code\/\$\{record\.id\}`\)/);
});
