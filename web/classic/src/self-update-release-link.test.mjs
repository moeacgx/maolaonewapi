import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const root = dirname(fileURLToPath(import.meta.url));
const readSource = (...parts) => readFileSync(resolve(root, ...parts), 'utf8');

test('Classic 更新详情使用后端 Release URL 并回退 maolaonewapi 仓库', () => {
  const source = readSource('components/settings/OtherSetting.jsx');

  assert.match(source, /html_url:\s*data\.html_url/);
  assert.match(source, /updateData\.html_url\s*\|\|/);
  assert.match(
    source,
    /https:\/\/github\.com\/moeacgx\/maolaonewapi\/releases\/tag\//,
  );
  assert.doesNotMatch(source, /github\.com\/moeacgx\/new-api\/releases\/tag/);
});
