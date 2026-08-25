import assert from 'node:assert/strict';
import test from 'node:test';

import { getGroupSemanticColor, getGroupTextColor } from './groupVisuals.js';

test('Classic 模型广场分组颜色稳定且不会退化成固定单色', () => {
  const groups = ['default', 'vip', 'premium', 'standard', 'trial'];
  const colors = groups.map((group) => getGroupSemanticColor(group));

  assert.equal(getGroupSemanticColor('vip'), getGroupSemanticColor('vip'));
  assert.ok(
    new Set(colors).size >= 3,
    `expected representative groups to use varied colors, got ${colors.join(', ')}`,
  );
  assert.equal(getGroupSemanticColor(''), 'grey');
  assert.equal(getGroupSemanticColor(null), 'grey');
});

test('Classic 模型广场分组文本色跟随语义色并为空值回退灰色', () => {
  assert.notEqual(getGroupTextColor('vip'), getGroupTextColor('premium'));
  assert.equal(getGroupTextColor(''), 'var(--semi-color-text-2)');
});
