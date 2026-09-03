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
    /setSelectedKeys\(\(prev\) => prev\.filter\(\(id\) => id !== record\.id\)\)/,
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

test('batch delete sends ids directly (no per-row object mapping)', () => {
  // Regression test for the plan's "selectedRowKeys 保存 ID" requirement.
  const body = extractArrowFunctionBody(source, 'batchDeletePromoCodes');

  assert.match(body, /const ids = selectedKeys\.filter\(Boolean\)/);
  assert.doesNotMatch(body, /selectedKeys\.map\(\(record\) => record\.id\)/);
});

test('batch delete reads deleted_ids.length and never substitutes the requested id count', () => {
  // Regression test: /api/promo_code/batch responds with
  // { deleted_ids: number[], skipped: {id,reason}[] }. Neither a bare
  // `data`, nor `data.deleted`, nor a fallback to the requested ids.length
  // is a legitimate source for the count — only deleted_ids.length is.
  const body = extractArrowFunctionBody(source, 'batchDeletePromoCodes');

  assert.match(
    body,
    /const \{ deletedIds, skipped \} = extractBatchDeleteResult\(data\)/,
  );
  assert.match(body, /count: deletedIds\.length/);
  assert.doesNotMatch(body, /\?\? ids\.length/);
  assert.doesNotMatch(body, /\|\| ids\.length/);
  assert.doesNotMatch(body, /data\?\.deleted\b/);
});

test('batch delete shows the admin actual deleted/skipped counts and reasons instead of a generic success toast', () => {
  const body = extractArrowFunctionBody(source, 'batchDeletePromoCodes');

  assert.match(body, /if \(skipped\.length === 0\) \{/);
  assert.match(body, /Modal\.warning\(\{/);
  assert.match(body, /deleted: deletedIds\.length,\s*skipped: skipped\.length/);
  assert.match(body, /\{skipped\.map\(\(item\) => \(/);
  // Reasons must be mapped to admin-readable text, not the raw backend
  // code (e.g. "not_found") interpolated straight into the list.
  assert.match(
    body,
    /#\$\{item\.id\}: \$\{describeSkipReason\(t, item\.reason\)\}/,
  );
  assert.doesNotMatch(body, /#\$\{item\.id\}: \$\{item\.reason\}/);
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

test('deleteInvalidPromoCodes calls /api/promo_code/invalid with loading and error feedback', () => {
  // Task 9 step 4: promo codes need an invalid-cleanup command mirroring
  // redemption's /api/redemption/invalid. /api/promo_code/invalid responds
  // with { deleted_ids: number[], skipped: {id,reason}[] } (confirmed,
  // backend Task 3 is done — this is no longer a guess).
  const body = extractArrowFunctionBody(source, 'deleteInvalidPromoCodes');

  assert.match(body, /\/api\/promo_code\/invalid/);
  assert.match(body, /setLoading\(true\)/);
  assert.match(body, /finally \{[\s\S]*?setLoading\(false\)/);
  assert.match(body, /catch \(error\) \{[\s\S]*?showError\(/);
  assert.match(body, /if \(!success\) \{[\s\S]*?showError\(/);
  assert.match(body, /setSelectedKeys\(\[\]\)/);
  assert.match(body, /await refresh\(\)/);
  assert.match(
    body,
    /const \{ deletedIds, skipped \} = extractBatchDeleteResult\(data\)/,
  );
  assert.match(body, /count: deletedIds\.length/);
  assert.doesNotMatch(body, /\?\? 0/);
  assert.doesNotMatch(body, /\?\? ids\.length/);
});

test('deleteInvalidPromoCodes also surfaces skipped items instead of a generic success toast', () => {
  const body = extractArrowFunctionBody(source, 'deleteInvalidPromoCodes');

  assert.match(body, /if \(skipped\.length === 0\) \{/);
  assert.match(body, /Modal\.warning\(\{/);
  assert.match(body, /deleted: deletedIds\.length,\s*skipped: skipped\.length/);
  assert.match(body, /\{skipped\.map\(\(item\) => \(/);
  assert.match(
    body,
    /#\$\{item\.id\}: \$\{describeSkipReason\(t, item\.reason\)\}/,
  );
  assert.doesNotMatch(body, /#\$\{item\.id\}: \$\{item\.reason\}/);
});

test('describeSkipReason maps known backend codes and falls back to a readable label for unknown ones', () => {
  // Regression test: the skip list used to interpolate the raw backend
  // reason code (e.g. "not_found") directly, which isn't admin-readable UI.
  const helperMatch =
    /const describeSkipReason = \(t, reason\) =>\s*\n?\s*t\(([\s\S]*?)\);/.exec(
      source,
    );
  assert.ok(helperMatch, 'expected to find describeSkipReason helper');
  assert.match(
    helperMatch[1],
    /SKIP_REASON_LABELS\[reason\] \|\| 'Unknown reason'/,
  );

  const labelsMatch = /const SKIP_REASON_LABELS = \{([\s\S]*?)\n\};/.exec(
    source,
  );
  assert.ok(labelsMatch, 'expected to find SKIP_REASON_LABELS');
  assert.match(labelsMatch[1], /not_found: 'Not found'/);
});

test('deleteInvalidPromoCodes is exported so the panel can wire it up', () => {
  assert.match(source, /deleteInvalidPromoCodes,\n\s*selectedKeys,/);
});

test('extractBatchDeleteResult reads deleted_ids/skipped and never a { deleted: N } or bare-number shape', () => {
  // Regression test: the previous extractDeletedCount() assumed a
  // { deleted: N } or bare-number response — neither matches the real
  // contract, so it silently fell back to the requested id count on every
  // real batch/invalid-cleanup response.
  const helperMatch =
    /const extractBatchDeleteResult = \(data\) => \(\{([\s\S]*?)\n\}\);/.exec(
      source,
    );
  assert.ok(helperMatch, 'expected to find extractBatchDeleteResult helper');
  assert.match(helperMatch[1], /deletedIds: data\?\.deleted_ids \|\| \[\]/);
  assert.match(helperMatch[1], /skipped: data\?\.skipped \|\| \[\]/);
  assert.doesNotMatch(source, /extractDeletedCount/);
});

test('the panel wires multi-select ids, and confirms every delete path', () => {
  assert.match(panelSource, /onChange: \(keys\) => setSelectedKeys\(keys\)/);
  assert.doesNotMatch(
    panelSource,
    /onChange: \(keys, rows\) => setSelectedKeys\(rows\)/,
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
    /Modal\.confirm\(\{[\s\S]*?onOk: deleteInvalidPromoCodes/,
  );
});

test('delete-selected only renders with an active selection', () => {
  // Task 9 step 5: batch commands should not sit permanently in the
  // toolbar — disabled or not — when nothing is selected.
  assert.match(
    panelSource,
    /\{selectedKeys\.length > 0 && \([\s\S]*?onOk: batchDeletePromoCodes[\s\S]*?\)\}/,
  );
  assert.doesNotMatch(panelSource, /disabled=\{selectedKeys\.length === 0\}/);
});

test('invalid-code cleanup lives in a "more actions" dropdown, not a standalone button', () => {
  assert.match(panelSource, /<Dropdown[\s\S]*?onOk: deleteInvalidPromoCodes/);
  assert.doesNotMatch(panelSource, /onClick=\{deleteInvalidPromoCodes\}/);
});

test('row actions aggregate edit/enable-disable/delete into one overflow menu', () => {
  // Task 9 step 5: inline row edit/enable-disable/delete should not be
  // three permanently-visible buttons per row.
  assert.match(panelSource, /moreMenuItems = \[/);
  assert.match(panelSource, /name: t\('编辑'\)/);
  assert.match(panelSource, /name: t\('启用'\)/);
  assert.match(panelSource, /name: t\('禁用'\)/);
  assert.match(panelSource, /name: t\('删除'\)/);
  assert.match(
    panelSource,
    /<Dropdown\s+trigger='click'\s+position='bottomRight'\s+menu=\{moreMenuItems\}\s*>/,
  );

  // JSX attribute form (onClick={fn}) would mean a direct standalone
  // button; the aggregated version only uses the object-literal form
  // (onClick: fn) inside moreMenuItems.
  assert.doesNotMatch(panelSource, /onClick=\{\(\) => openEdit\(record\)\}/);
  assert.doesNotMatch(
    panelSource,
    /onClick=\{\(\) =>\s*updatePromoCodeStatus\(/,
  );
});
