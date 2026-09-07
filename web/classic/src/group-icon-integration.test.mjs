import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));
const readSource = (...parts) => readFileSync(resolve(root, ...parts), 'utf8');

test('Classic 分组数据和主要分组选择器接入图标字段', () => {
  const helperSource = readSource('helpers/groupDetails.js');
  const tokenSource = readSource(
    'components/table/tokens/modals/EditTokenModal.jsx',
  );
  const channelSource = readSource(
    'components/table/channels/modals/EditChannelModal.jsx',
  );
  const tagSource = readSource(
    'components/table/channels/modals/EditTagModal.jsx',
  );

  assert.match(helperSource, /icon:\s*normalizeIcon/);
  assert.match(helperSource, /const icon = normalizeIcon\(info\.icon\)/);
  assert.match(tokenSource, /getLobeHubIcon\(info\.icon|g\.icon/);
  assert.match(channelSource, /group\.icon/);
  assert.match(tagSource, /renderGroupOption\([\s\S]*icon:\s*group\?\.icon/);
});

test('Classic 分组管理表提供精选图标选择入口', () => {
  const source = readSource('pages/Setting/Ratio/components/GroupTable.jsx');

  assert.match(source, /GROUP_ICON_OPTIONS/);
  assert.match(source, /filteredIconOptions/);
  assert.match(source, /placeholder=\{t\('搜索图标'\)\}/);
  assert.match(source, /getLobeHubIcon/);
  assert.match(source, /onClick=.*icon/);
  assert.match(source, /title=.*图标/);
});
