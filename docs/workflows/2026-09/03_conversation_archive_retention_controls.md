# 对话归档容量控制与本地化

## 目标

对话归档是数据库持久化能力，不应无限增长。Root 需要能够设置全局保留的
最近会话数量，超出时自动删除最旧归档，并在不再需要排查数据时手动清空。
Classic 与 Default 原生页面必须显示当前语言的文案，不能因缺少翻译键回退为英文。

## 方案

- 配置增加全局 `max_archive_count`。默认保留最近 1000 条，允许设置为
  1 至 100000 条；单条清洗正文上限和按天过期清理保持原有语义。
- 每次归档写入、定时过期清理和手动清空均在同一数据库事务内锁定配置单例；
  写入后按 `created_at ASC, id ASC` 删除溢出的最旧记录。配置降低上限时，
  在同一配置 CAS 事务内立即裁剪已有记录。裁剪每批重新计数，删除零行会使
  事务回滚，避免与非预期外部删除竞争时循环不退出。
- Root 清空接口要求显式确认、关键操作限流，并与写入操作使用同一配置行锁；
  清空过程中不读取或记录归档正文。采集未关闭时，之后命中的请求仍会再次归档。
- 原生页面新增会话上限与清空操作；Default 语言包通过规定的同步脚本维护，
  Classic 语言包逐语言补齐与页面实际 `t()` 键一致的文案。

## 兼容性与验证计划

模型迁移只使用 GORM 标准字段和查询，兼容 SQLite、MySQL 5.7.8+ 与
PostgreSQL 9.6+；不使用 `DELETE ... LIMIT`、窗口函数或方言 SQL。

验证覆盖配置 CAS 后即时裁剪、写入后仅保留最新记录、过期清理的有界批次、
Root 确认清空与权限边界，以及 Default/Classic 所有归档页面翻译键。Classic
原生测试入口包含翻译回归测试。交付前执行对应 Go、Node 原生页面测试、格式化、
`git diff --check` 和可用的前端 i18n 同步检查。

## 验证结果

- `go test ./model -count=1 -timeout 60s`、`go test ./service -count=1 -timeout 60s`、
  `go test ./router -count=1 -timeout 60s` 与 `go test ./extension -count=1 -timeout 60s`
  均通过。
- `node --test scripts/conversation-archive-native.test.mjs
  scripts/conversation-archive-i18n.test.mjs scripts/native-sdk-live-api.test.mjs
  scripts/security-audit-load-data.test.mjs` 通过；本地化测试会拒绝中文键缺失或直接
  回退为英文原文。
- `node web/scripts/sync-i18n.mjs` 执行成功。完整 `go test ./...` 仅根包因工作树不含
  `web/classic/dist` 的嵌入目录而无法建立，未发现本次改动引入的编译错误。

## Classic 页面壳修正

用户反馈 Classic 页面内容直接铺在背景上，采集设置、会话列表和预览缺少统一的
业务边界。修正后由 `archive-page-shell` 提供唯一可见的外层卡片（边框、圆角、背景
和轻阴影），内部各业务区使用分隔线与稳定内边距，避免重复嵌套卡片；表单和筛选区
分别使用响应式栅格，窄屏降为单列并保持按钮可达。新增结构回归测试会检查外层壳、
内容区及其边框/圆角/背景样式。

Classic 原生扩展不经过 `.classic-console-page` 的公共顶部占位。宿主 `Content` 在桌面端
已有 24px、窄屏端已有 5px 顶部内边距；页面壳分别补充 48px 与 67px，使其始终位于
固定 64px 顶栏下方 8px，避免归档标题被顶栏遮挡。窄屏覆盖点与宿主的 767px 边界一致，
而不是只在归档内部布局变为单列的 720px 后才生效。对应结构回归测试固定这两个间距契约。

## Classic 预览浮窗

对话预览不再作为页面内业务区渲染。选择会话后，`ArchivePreview` 会在页面壳
外创建固定定位的全屏遮罩，并将清洗后的消息放入居中的 `role="dialog"` 浮窗；
因此预览不会占用归档列表的页面高度。点击遮罩空白处、关闭按钮或按 `Escape`
均可关闭；打开时焦点进入浮窗，`Tab` 保持在浮窗内，关闭后恢复到原触发位置，
同时锁定页面背景滚动。消息区域在内容过长时独立滚动，窄屏也保持在可视区域居中
并保留可达的关闭操作。消息正文继续通过 React 文本节点渲染，不使用
`dangerouslySetInnerHTML`。

结构回归测试同时检查遮罩固定定位、全屏覆盖、居中布局、浮窗最大高度和滚动
边界，以及对话框无障碍、焦点管理和键盘关闭契约。
