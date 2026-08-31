# Classic 通知任务弹窗移动端适配与关键词输入优化

## 问题

1. 报错关键词筛选使用 `TagInput`，无法直接粘贴多行关键词；用户需逐个按回车添加，批量操作体验差。
2. Semi Modal 的自定义 class 挂在 Portal 外层，原有宽度规则没有直接约束内层实际对话框和 flex 内容层；在 375px 左右视口下可能出现右侧溢出，保存按钮可能不可达，目标卡片横向溢出。

## 修改

- `web/classic/src/pages/NotificationCenter/index.jsx`：
  - 报错关键词筛选使用 `TextArea`（`autosize={{ minRows: 3, maxRows: 8 }}`），`value` 用 `join('\n')` 回显，`onChange` 用 `split(/\r?\n/)` 更新 form state，归一化仍由 `normalizeNotificationFilterConfig` 在保存时统一处理（trim、去空行、非 locale 小写去重并保留首个原始拼写）。
  - 弹窗宽度使用 `width='min(720px, calc(100vw - 32px))'`，并设置 `classic-notification-task-modal` 专用 class 与 `bodyStyle` 滚动边界；桌面宽度不变，窄屏自适应留 16px 边距，footer 保持可达。
  - 目标卡片标记为 `classic-notification-target-card`，保存 payload 仅在 `channel_disabled` 时携带 `filter_config`。
- `web/classic/src/index.css`：以 `classic-notification-task-modal` 为唯一作用域，直接约束真实 `.semi-modal`、`.semi-modal-content`、`.semi-modal-body-wrapper`、`.semi-modal-body` 和任务 body 的 `box-sizing`、宽度及 flex 收缩；窄屏对话框使用 `calc(100vw - 32px)` 并保留两侧 16px，桌面上限为 720px。TagInput（Semi 的 `.semi-tagInput`）和目标卡片规则仍限定在该作用域，避免影响 Bot Modal、confirm 和其他页面。
- `web/classic/src/i18n/locales/{en,fr,ru,ja,vi,zh-CN,zh-TW}.json`：新增 `"One keyword per line"` 翻译键，删除不再使用的 `"Enter a keyword and press Enter to add multiple"` 键（旧 TagInput placeholder）。
- `web/classic/src/pages/NotificationCenter/__tests__/filter-config.test.mjs`：覆盖 CRLF/LF 多行输入、空行、重复与大小写边界，并静态验证非 `channel_disabled` payload 清除和 Modal/CSS 作用域契约。

## 兼容性与边界

- 后端接口、数据库字段和匹配逻辑不变；`normalizeNotificationFilterConfig` 只在保存时调用，TextArea 中间状态允许存在空行，非 `channel_disabled` 任务不会发送 `filter_config`。
- 桌面布局（≥640px）不退化：弹窗宽度上限仍为 720px。
- 已有任务打开编辑时，`error_keywords` 数组通过 `join('\n')` 正确回显为多行文本。

## 验证

- 本轮验证以 `modal-viewport.test.mjs` 和通知筛选定向 Node 测试、Classic 全量 Node 测试、`git diff --check` 为确定性结果；并使用当前 CSS 与 Semi Portal DOM 结构的临时 fixture，通过 Playwright Chrome 完成 320/375/414px 和 1280px 截图核对。i18n sync/status、Prettier、ESLint 和 Vite build 是否可执行，以当前环境命令输出为准。
- 临时 fixture 的几何验证确认移动对话框宽度为视口减 32px、两侧各 16px，桌面上限为 720px；未在真实登录态页面中验证任务保存交互、极端长文本和软键盘行为。

## 2026-08-31 真实长文本回归修复

### 根因

真实 Semi Portal DOM 中，任务 Modal 仍使用默认 `80px` 上下外边距，内容层的自然高度会把 footer 推出 700px 移动视口。与此同时，事件变量和目标 Chat ID 使用的 Semi Tag 内部摘要节点保留单行省略样式；不可换行的长值会把任务 body 的 `scrollWidth` 撑到 1745px，即使父层设置了 `overflow-x: hidden`，仍可观察到横向滚动。

### 修改

- `NotificationCenter/index.jsx`：任务 Modal 增加 `height='min(720px, calc(100vh - 32px))'`，移除 body 的固定 `maxHeight`，让 CSS flex 约束决定可滚动区域。
- `index.css`：任务 Modal 使用 `height/max-height: min(720px, calc(100vh - 32px))` 和 `margin: 16px auto`；content、body、body wrapper 设置 `min-height: 0`，header/footer 禁止压缩，body 使用 `flex: 1 1 auto` 和 `overflow-y: auto`。
- `index.css`：任务 body 内所有 `.semi-tag`、`.semi-tag-content`、`.semi-tagInput-wrapper-typo` 均允许断词和收缩，确保长 Chat ID、提及名称、模板变量和事件变量保持可见而不撑宽内容层。
- `modal-viewport.test.mjs`：新增视口高度/flex/footer 契约和长不可换行 Tag 契约，并保留原有宽度、作用域和滚动归属测试。

### 运行验证

- `node --test src/pages/NotificationCenter/__tests__/modal-viewport.test.mjs src/pages/NotificationCenter/__tests__/filter-config.test.mjs`：13/13 通过。
- Playwright Chrome 本地模拟登录态 mock：`1280x820`、`414x700`、`375x700`、`320x700` 全部通过；长 Chat ID、提及名称、消息模板和事件变量均保留，document/Modal/body/task body 无横向溢出，footer 确认按钮均位于视口内。
- Classic `npm run build` 和全量契约测试由上游审计任务负责；本轮改动未触碰其他页面或 Default 模板。

### 边界

- Semi 的输入元素自身仍可能报告大于 clientWidth 的文本 scrollWidth，这是浏览器输入控件的内部文本滚动，不会传播到任务 body 或 document，也不产生页面横向滚动。
- 尚未覆盖真实后端登录、软键盘弹出和极端系统字体环境；这些属于后续设备验收范围。
