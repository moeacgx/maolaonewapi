# Classic 通知任务弹窗移动端适配与关键词输入优化

## 问题

1. 报错关键词筛选使用 `TagInput`，无法直接粘贴多行关键词；用户需逐个按回车添加，批量操作体验差。
2. 任务编辑弹窗硬编码 `width={720}`，在 375px 左右视口下超出屏幕，保存按钮可能不可达，目标卡片横向溢出。

## 修改

- `web/classic/src/pages/NotificationCenter/index.jsx`：
  - 报错关键词筛选使用 `TextArea`（`autosize={{ minRows: 3, maxRows: 8 }}`），`value` 用 `join('\n')` 回显，`onChange` 用 `split(/\r?\n/)` 更新 form state，归一化仍由 `normalizeNotificationFilterConfig` 在保存时统一处理（trim、去空行、非 locale 小写去重并保留首个原始拼写）。
  - 弹窗宽度使用 `width='min(720px, calc(100vw - 32px))'`，并设置 `classic-notification-task-modal` 专用 class 与 `bodyStyle` 滚动边界；桌面宽度不变，窄屏自适应留 16px 边距，footer 保持可达。
  - 目标卡片标记为 `classic-notification-target-card`，保存 payload 仅在 `channel_disabled` 时携带 `filter_config`。
- `web/classic/src/index.css`：所有任务 Modal 的宽度、margin、内容滚动、TagInput（Semi 的 `.semi-tagInput`）和目标卡片规则均以 `classic-notification-task-modal` 作用域，避免影响 Bot Modal、confirm 和其他页面。
- `web/classic/src/i18n/locales/{en,fr,ru,ja,vi,zh-CN,zh-TW}.json`：新增 `"One keyword per line"` 翻译键，删除不再使用的 `"Enter a keyword and press Enter to add multiple"` 键（旧 TagInput placeholder）。
- `web/classic/src/pages/NotificationCenter/__tests__/filter-config.test.mjs`：覆盖 CRLF/LF 多行输入、空行、重复与大小写边界，并静态验证非 `channel_disabled` payload 清除和 Modal/CSS 作用域契约。

## 兼容性与边界

- 后端接口、数据库字段和匹配逻辑不变；`normalizeNotificationFilterConfig` 只在保存时调用，TextArea 中间状态允许存在空行，非 `channel_disabled` 任务不会发送 `filter_config`。
- 桌面布局（≥640px）不退化：弹窗宽度上限仍为 720px。
- 已有任务打开编辑时，`error_keywords` 数组通过 `join('\n')` 正确回显为多行文本。

## 验证

- 本轮验证以 Node 定向契约测试、Classic 全量 Node 测试、`git diff --check` 为确定性结果；i18n sync/status、Prettier、ESLint 和 Vite build 是否可执行，以当前环境命令输出为准。
- 未进行真实浏览器截图或 320/375/414px 视觉回归；静态契约覆盖专用 class、窄屏宽度/滚动规则和 footer 可达所需的 CSS/Modal 配置。
