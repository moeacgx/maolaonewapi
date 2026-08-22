export const getVisiblePricingColumns = (columns, compactMode) => {
  if (!compactMode) {
    return columns;
  }

  // 新版移动端表格只展示“模型”和“类型”，避免横向滚动破坏首屏层级。
  return columns.slice(0, 2).map(({ fixed, ...column }) => column);
};

export const getActiveRowSelection = (selectionMode, rowSelection) =>
  selectionMode ? rowSelection : undefined;
