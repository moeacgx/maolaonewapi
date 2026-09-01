import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const testDirectory = path.dirname(fileURLToPath(import.meta.url));
const headerSource = fs.readFileSync(
  path.resolve(testDirectory, '..', 'useHeaderBar.js'),
  'utf8',
);

test('Classic header logout uses the current auth logout endpoint', () => {
  assert.match(headerSource, /API\.post\('\/api\/user\/auth\/logout'/);
  assert.doesNotMatch(headerSource, /API\.get\('\/api\/user\/logout'/);
});
