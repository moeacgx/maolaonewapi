# Classic Semi UI 样式恢复

日期：2026-08-31

## 问题

Classic 入口在 Issue #77 的提交中将 `@douyinfe/semi-ui/dist/css/semi.css`
替换为只提供基础 reset 的 `_base/base.css`。这会让组件自身的样式仍能渲染，
但 Card 边框、Form 标题分隔线、按钮背景和尺寸等完整视觉契约全部缺失。

## 修复

- 将 `web/classic/src/index.jsx` 恢复为引入完整的
  `@douyinfe/semi-ui/dist/css/semi.css`。
- 不修改设置页 DOM、Classic 路由、Default 前端或后端逻辑。
- 保留既有全局平铺页面的专用规则；它们只作用于明确标记了
  `classic-flat-page` 的页面级 Card，不会影响设置页 Card。

## 验证

- `node --test src/build-contract.test.mjs`
- `npx --no-install prettier --check src/index.jsx src/build-contract.test.mjs`
- `npx --no-install eslint src/index.jsx`
- `git diff --check`

完整样式入口恢复后，设置页的系统信息、通用设置、个性化设置区块会恢复边框和
标题线，“检查更新”“切换到新版前端”及各保存按钮会恢复 Semi UI 按钮样式。
