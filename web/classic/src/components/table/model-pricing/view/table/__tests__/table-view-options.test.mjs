import assert from 'node:assert/strict';
import test from 'node:test';

import {
  getActiveRowSelection,
  getVisiblePricingColumns,
} from '../table-view-options.js';

test('移动端模型广场表格只保留新版首屏列且不修改原列定义', () => {
  const columns = [
    { dataIndex: 'model_name', fixed: 'left' },
    { dataIndex: 'quota_type' },
    { dataIndex: 'model_price' },
  ];

  const result = getVisiblePricingColumns(columns, true);

  assert.deepEqual(result, [
    { dataIndex: 'model_name' },
    { dataIndex: 'quota_type' },
  ]);
  assert.equal(columns.length, 3);
  assert.equal(columns[0].fixed, 'left');
});

test('批量选择模式关闭时不向表格注入勾选列', () => {
  const rowSelection = { selectedRowKeys: ['gpt-5'] };

  assert.equal(getActiveRowSelection(false, rowSelection), undefined);
  assert.equal(getActiveRowSelection(true, rowSelection), rowSelection);
});
