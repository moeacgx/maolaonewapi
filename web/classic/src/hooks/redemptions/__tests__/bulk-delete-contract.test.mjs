import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const hookSource = readFileSync(
  new URL('../useRedemptionsData.jsx', import.meta.url),
  'utf8',
);
const pageSource = readFileSync(
  new URL('../../../components/table/redemptions/index.jsx', import.meta.url),
  'utf8',
);
const actionsSource = readFileSync(
  new URL(
    '../../../components/table/redemptions/RedemptionsActions.jsx',
    import.meta.url,
  ),
  'utf8',
);
const deleteModalSource = readFileSync(
  new URL(
    '../../../components/table/redemptions/modals/DeleteRedemptionModal.jsx',
    import.meta.url,
  ),
  'utf8',
);

// Extracts the body of `const <fnName> = [async] (...) => { ... }` by
// matching braces, so assertions target one function instead of the whole
// file (which would make them blind to which function actually does what).
function extractArrowFunctionBody(source, fnName) {
  const signature = new RegExp(
    `const ${fnName} = (?:async )?\\([^)]*\\) => \\{`,
  );
  const match = signature.exec(source);
  assert.ok(match, `expected to find function ${fnName}`);
  const start = match.index + match[0].length;
  let depth = 1;
  let i = start;
  while (i < source.length && depth > 0) {
    if (source[i] === '{') depth++;
    else if (source[i] === '}') depth--;
    i++;
  }
  return source.slice(start, i - 1);
}

test('selected-delete and invalid-cleanup are distinct commands hitting distinct endpoints', () => {
  const selected = extractArrowFunctionBody(
    hookSource,
    'batchDeleteSelectedRedemptions',
  );
  const invalidCleanup = extractArrowFunctionBody(
    hookSource,
    'batchDeleteRedemptions',
  );

  assert.match(selected, /\/api\/redemption\/batch/);
  assert.doesNotMatch(selected, /\/api\/redemption\/invalid/);

  assert.match(invalidCleanup, /\/api\/redemption\/invalid/);
  assert.doesNotMatch(invalidCleanup, /\/api\/redemption\/batch/);
});

test('both batch delete commands require confirmation before firing the request', () => {
  const selected = extractArrowFunctionBody(
    hookSource,
    'batchDeleteSelectedRedemptions',
  );
  const invalidCleanup = extractArrowFunctionBody(
    hookSource,
    'batchDeleteRedemptions',
  );

  assert.match(selected, /Modal\.confirm\(/);
  assert.match(invalidCleanup, /Modal\.confirm\(/);
});

test('the page actually wires the selected-delete handler into the action buttons', () => {
  // Regression test: index.jsx previously destructured
  // batchDeleteSelectedRedemptions from the hook but never passed it down,
  // so the "delete selected" button's onClick was undefined and clicking it
  // silently did nothing.
  assert.match(
    pageSource,
    /const \{[\s\S]*?batchDeleteSelectedRedemptions[\s\S]*?\} = redemptionsData;/,
  );
  assert.match(
    pageSource,
    /<RedemptionsActions[\s\S]*?batchDeleteSelectedRedemptions=\{batchDeleteSelectedRedemptions\}[\s\S]*?\/>/,
  );
  assert.match(actionsSource, /onClick=\{batchDeleteSelectedRedemptions\}/);
});

test('batch delete selected redemptions toggles loading, clears selection, and reports failures', () => {
  const body = extractArrowFunctionBody(
    hookSource,
    'batchDeleteSelectedRedemptions',
  );

  assert.match(body, /setLoading\(true\)/);
  assert.match(body, /setLoading\(false\)/);
  assert.match(body, /setSelectedKeys\(\[\]\)/);
  assert.match(body, /catch \(error\) \{[\s\S]*?showError\(/);
});

test('invalid redemption cleanup does not silently swallow request failures', () => {
  const body = extractArrowFunctionBody(hookSource, 'batchDeleteRedemptions');

  // Guards against the pre-existing bug where a rejected API.delete() call
  // left the page stuck in a loading state forever with no error shown.
  assert.match(body, /try \{/);
  assert.match(body, /catch \(error\) \{[\s\S]*?showError\(/);
  assert.match(body, /finally \{[\s\S]*?setLoading\(false\)/);
});

test('loadRedemptions falls back a page when a delete empties the current page', () => {
  const body = extractArrowFunctionBody(hookSource, 'loadRedemptions');

  assert.match(body, /data\.items\.length === 0 && page > 1/);
  assert.match(body, /loadRedemptions\(page - 1, pageSize\)/);
});

test('page, page-size, and search changes clear any stale bulk selection', () => {
  const handlePageChange = extractArrowFunctionBody(
    hookSource,
    'handlePageChange',
  );
  const handlePageSizeChange = extractArrowFunctionBody(
    hookSource,
    'handlePageSizeChange',
  );
  const searchRedemptions = extractArrowFunctionBody(
    hookSource,
    'searchRedemptions',
  );

  assert.match(handlePageChange, /setSelectedKeys\(\[\]\)/);
  assert.match(handlePageSizeChange, /setSelectedKeys\(\[\]\)/);
  assert.match(searchRedemptions, /setSelectedKeys\(\[\]\)/);
});

test('a single delete removes its own id from any active bulk selection', () => {
  const body = extractArrowFunctionBody(hookSource, 'manageRedemption');

  assert.match(
    body,
    /setSelectedKeys\(\(prev\) =>\s*prev\.filter\(\(selectedId\) => selectedId !== id\),?\s*\)/,
  );
});

test('selectedKeys stores redemption ids, not row objects', () => {
  // Regression test for the plan's "selectedRowKeys 保存 ID" requirement:
  // rowSelection.onChange used to store full row objects (selectedRows),
  // which pinned stale snapshots across refreshes.
  assert.match(
    hookSource,
    /onChange: \(selectedRowKeys\) => \{\s*setSelectedKeys\(selectedRowKeys\);/,
  );
  assert.doesNotMatch(hookSource, /setSelectedKeys\(selectedRows\)/);

  // The table must key rows by id so selectedRowKeys are actually ids.
  const tableSource = readFileSync(
    new URL(
      '../../../components/table/redemptions/RedemptionsTable.jsx',
      import.meta.url,
    ),
    'utf8',
  );
  assert.match(tableSource, /rowKey='id'/);
});

test('batch-selected delete sends ids directly (no per-row object mapping)', () => {
  const body = extractArrowFunctionBody(
    hookSource,
    'batchDeleteSelectedRedemptions',
  );

  assert.match(body, /const ids = selectedKeys\.filter\(Boolean\)/);
  assert.doesNotMatch(body, /selectedKeys\.map\(\(record\) => record\.id\)/);
});

test('bulk copy still works under id-only selection by resolving full records', () => {
  // Regression test: copy needs each record's name + actual code, which
  // aren't available once selectedKeys only holds ids.
  const body = extractArrowFunctionBody(hookSource, 'batchCopyRedemptions');

  assert.match(
    body,
    /redemptions\.filter\(\(record\) =>\s*selectedKeys\.includes\(record\.id\),?\s*\)/,
  );
  assert.match(body, /\.name.*\.key|\.key.*\.name/s);
});

test('batch delete never substitutes a legitimate 0 deleted count for the requested count', () => {
  // Regression test: `data || ids.length` treats a real 0 as falsy and
  // shows "deleted N" (the request size) even when the backend deleted
  // nothing.
  const body = extractArrowFunctionBody(
    hookSource,
    'batchDeleteSelectedRedemptions',
  );

  assert.match(body, /count: data\?\.deleted \?\? ids\.length/);
  assert.doesNotMatch(body, /count: data \|\| ids\.length/);
});

test('the delete-selected button only renders when something is selected', () => {
  assert.match(
    actionsSource,
    /\{selectedKeys\.length > 0 && \([\s\S]*?onClick=\{batchDeleteSelectedRedemptions\}[\s\S]*?\)\}/,
  );
});

test('invalid-code cleanup lives in an overflow menu, not a permanently-visible danger button', () => {
  assert.match(actionsSource, /Dropdown/);
  assert.match(
    actionsSource,
    /moreMenuItems = \[[\s\S]*?onClick: batchDeleteRedemptions[\s\S]*?\]/,
  );
  // JSX attribute form (onClick={fn}) would mean it's still a standalone
  // button; the menu-item form (onClick: fn) is the aggregated version.
  assert.doesNotMatch(actionsSource, /onClick=\{batchDeleteRedemptions\}/);
});

test('single delete no longer depends on a stale-closure pagination guess', () => {
  // Regression test: the modal used to read `redemptions`/`activePage`
  // props captured at render time inside a setTimeout, which almost never
  // reflected the post-delete state. Pagination fallback now lives in
  // loadRedemptions() itself, driven by the fresh API response.
  assert.doesNotMatch(deleteModalSource, /setTimeout/);
  assert.doesNotMatch(deleteModalSource, /redemptions\.length === 0/);
  assert.match(
    deleteModalSource,
    /await manageRedemption\([\s\S]*?\);[\s\S]*?await refresh\(\);/,
  );
});
