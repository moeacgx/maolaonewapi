# Classic 福利活动创建入口与抽屉重开修复

## 问题

Classic “营销福利 -> 时效额度券”页面同时显示了左侧静态 `Plus` 标题图标和右侧“创建活动”按钮。
左侧图标不是交互入口；右侧按钮第一次打开抽屉后，关闭抽屉再点击可能没有反应。

## 根因

`SideSheet` 关闭后，`formApiRef` 仍保留已关闭表单的引用。再次点击“创建活动”时，
`openCreate` 先调用旧引用的 `reset`，该调用可能抛错并阻断后续的 `setEditorVisible(true)`。
编辑入口也存在相同的旧引用依赖。

## 修改范围

- 移除卡片标题左侧非交互 `Plus` 图标，只保留右侧创建按钮。
- 每次创建或编辑活动时递增表单会话 key，使抽屉打开使用新的表单实例和初始值。
- 统一抽屉关闭、取消和保存成功路径，清空旧的 `formApiRef`。
- 未修改 Default 模板、后端接口、活动状态机或计费逻辑。

## 兼容性与安全边界

右侧“创建活动”仍调用原有管理接口；编辑、保存和校验行为保持不变。
表单会话隔离只影响 Classic 抽屉生命周期，不改变活动数据或权限边界。

## 验证

- `node --test web/classic/src/hooks/benefits/__tests__/benefit-contract.test.mjs`
- `git diff --check`
- Classic 前端构建应在发布前通过 CI 验证。
