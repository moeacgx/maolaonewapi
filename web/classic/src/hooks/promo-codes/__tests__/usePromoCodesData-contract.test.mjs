import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const source = readFileSync(
  new URL('../usePromoCodesData.jsx', import.meta.url),
  'utf8',
);
const panelSource = readFileSync(
  new URL(
    '../../../components/table/promo-codes/PromoCodesPanel.jsx',
    import.meta.url,
  ),
  'utf8',
);

// Extracts the body of `const <fnName> = [async] (...) => { ... }` by
// matching braces, so assertions target one function instead of the whole
// file (which would make them blind to which function actually does what).
function extractArrowFunctionBody(text, fnName) {
  const signature = new RegExp(
    `const ${fnName} = (?:async )?\\([^)]*\\) => \\{`,
  );
  const match = signature.exec(text);
  assert.ok(match, `expected to find function ${fnName}`);
  const start = match.index + match[0].length;
  let depth = 1;
  let i = start;
  while (i < text.length && depth > 0) {
    if (text[i] === '{') depth++;
    else if (text[i] === '}') depth--;
    i++;
  }
  return text.slice(start, i - 1);
}

test('优惠码数据钩子只调用后端实际的 /api/promo_code/ 路由', () => {
  assert.match(source, /\/api\/promo_code\//);
  assert.doesNotMatch(source, /\/api\/promo-code\//);
  assert.match(source, /API\.delete\(`\/api\/promo_code\/\$\{record\.id\}`\)/);
});

test('single delete does not silently swallow request failures', () => {
  // Regression test: deletePromoCode used to call API.delete() with no
  // try/catch, so a rejected request (network error, 401, 500, ...) became
  // an unhandled promise rejection with no error shown and no loading reset.
  const body = extractArrowFunctionBody(source, 'deletePromoCode');

  assert.match(body, /try \{/);
  assert.match(body, /catch \(error\) \{[\s\S]*?showError\(/);
  assert.match(body, /setLoading\(true\)/);
  assert.match(body, /finally \{[\s\S]*?setLoading\(false\)/);
});

test('single delete removes its own id from any active bulk selection', () => {
  const body = extractArrowFunctionBody(source, 'deletePromoCode');

  assert.match(
    body,
    /setSelectedKeys\(\(prev\) => prev\.filter\(\(item\) => item\.id !== record\.id\)\)/,
  );
});

test('batch delete toggles the shared loading state around the request', () => {
  // Regression test: batchDeletePromoCodes never set loading, so the table
  // gave no visual feedback while the batch request was in flight.
  const body = extractArrowFunctionBody(source, 'batchDeletePromoCodes');

  assert.match(body, /setLoading\(true\)/);
  assert.match(body, /finally \{[\s\S]*?setLoading\(false\)/);
  assert.match(body, /setSelectedKeys\(\[\]\)/);
  assert.match(body, /catch \(error\) \{[\s\S]*?showError\(/);
});

test('loadPromoCodes falls back a page when a delete empties the current page', () => {
  const body = extractArrowFunctionBody(source, 'loadPromoCodes');

  assert.match(body, /items\.length === 0 && page > 1/);
  assert.match(body, /loadPromoCodes\(page - 1, size\)/);
});

test('searchPromoCodes falls back a page when a delete empties the current search page', () => {
  const body = extractArrowFunctionBody(source, 'searchPromoCodes');

  assert.match(body, /items\.length === 0 && page > 1/);
  assert.match(body, /searchPromoCodes\(keyword, page - 1, size\)/);
});

test('page, page-size, and search changes clear any stale bulk selection', () => {
  const handlePageChange = extractArrowFunctionBody(source, 'handlePageChange');
  const handlePageSizeChange = extractArrowFunctionBody(
    source,
    'handlePageSizeChange',
  );
  const searchPromoCodes = extractArrowFunctionBody(source, 'searchPromoCodes');

  assert.match(handlePageChange, /setSelectedKeys\(\[\]\)/);
  assert.match(handlePageSizeChange, /setSelectedKeys\(\[\]\)/);
  assert.match(searchPromoCodes, /setSelectedKeys\(\[\]\)/);
});

test('the panel wires multi-select, confirmation, and loading feedback for both delete paths', () => {
  assert.match(
    panelSource,
    /rowSelection[\s\S]*?onChange: \(keys, rows\) => setSelectedKeys\(rows\)/,
  );
  assert.match(
    panelSource,
    /Modal\.confirm\(\{[\s\S]*?onOk: \(\) => deletePromoCode\(record\)/,
  );
  assert.match(
    panelSource,
    /Modal\.confirm\(\{[\s\S]*?onOk: batchDeletePromoCodes/,
  );
  assert.match(
    panelSource,
    /disabled=\{selectedKeys\.length === 0\}\s*\n\s*loading=\{loading\}/,
  );
});
