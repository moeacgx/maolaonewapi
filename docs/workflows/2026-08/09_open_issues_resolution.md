# Open Issues #14/#16/#17/#18/#20 修复记录

## 目标

一次性处理 `moeacgx/maolaonewapi` 当前 open issues：

- #14 客户端错误消息：匹配测试支持包含与精确匹配，并且匹配大小写不敏感。
- #16 任务日志兼容：任务日志补齐模型、分组、用户信息弹出、渠道信息弹出、任务 ID 展开显示。
- #17 订阅套餐消耗顺序：多套餐用户优先消耗最接近刷新的套餐。
- #18 安全审计列表列设置：列展示持久化；正文预览默认隐藏并放入展开详情，同时减少分组/令牌绑定分组默认列。
- #20 图标依赖升级：升级前端图标依赖并同步锁文件。

## 范围

- 后端：客户端错误消息匹配逻辑与测试、订阅套餐消耗排序逻辑。
- 前端：默认主题的安全审计列表、任务日志列表、图标依赖。
- 文档：本记录与 `docs/developer/README.md` 索引。

## 方案

- 保持数据库兼容 SQLite、MySQL、PostgreSQL；涉及排序优先使用 GORM 可跨库表达或 Go 内存排序。
- 所有客户端请求 DTO 的显式零值语义保持不变；不改动上游 relay 请求结构。
- 默认主题新增 UI 文案时必须走 `t()` 并补齐 `en/zh/fr/ja/ru/vi`。
- 安全审计列设置使用浏览器本地持久化，不改变后端 API 契约。
- 任务日志新增展示字段时优先复用使用日志已有弹出详情组件与数据获取方式。

## 安全边界与兼容性

- #14 仅改变后台配置规则的匹配方式；不扩大匹配目标来源，不改变已有错误响应替换的权限边界。
- #17 仅改变同一用户多订阅套餐之间的扣减顺序；不改变套餐额度、重置周期或账单记录语义。
- #18/#16 只调整列表展示与本地偏好，不改变审计事件、任务日志的存储生命周期。
- #20 仅升级图标库依赖，不引入新的运行时服务或网络调用。

## 测试计划

- 后端：运行覆盖客户端错误匹配和订阅套餐消耗排序的 Go 测试。
- 前端：运行 i18n 同步检查和默认主题构建；必要时使用本地测试脚本验证页面。
- 回归：确认任务日志、安全审计列表在刷新后保留可见列偏好；确认正文/任务 ID 展开详情可读。

## 实施结果

- #14: 客户端错误消息规则支持 `matches` 多值 OR,`contains`/`exact` 均大小写不敏感;最终客户端响应继续只改对外状态码与文案,错误日志保留上游详情。
- #16: Default 与 Classic 任务日志补齐管理员渠道列、用户弹窗入口、模型展示、分组展示、任务 ID 展开详情和渠道详情弹窗;移动卡片同步展示模型、分组、渠道和失败/结果信息。
- #17: 订阅预扣从按 `id` 顺序改为按下一次可消费边界优先,先消耗最接近刷新的套餐,再按到期时间和 `id` 稳定兜底;退款仍按原预扣记录回补。
- #18: Default 与 Classic 安全审计事件列表新增列可见性本地持久化,默认隐藏正文预览、路由分组和令牌绑定分组;行展开区展示正文预览、Prompt hash、路由分组和令牌绑定分组。
- #20: Default 与 Classic 升级 `@lobehub/icons` 到 `^5.15.0`,同步前端锁文件。

## 验证结果

- `go test ./common -run TestErrorMessageReplacement -count=1` 通过。
- `go test ./model -run 'TestPreConsumeUserSubscriptionPrioritizesNearestReset|TestGetAllUnFinishSyncTasksSkipsLocalImageWrapperTasks|TestGetTimedOutUnfinishedTasksByPlatformsOnlyReturnsImageWrappers' -count=1` 通过。
- `go test ./controller -run 'TestRecordChannelErrorLogShowsReplacementAndKeepsUpstreamDetail|TestShouldRetryWithReasonUsesStatusBeforeChannelMapping' -count=1` 通过。
- `npx --yes bun test src/features/usage-logs/lib/task-log-format.test.ts src/features/security-audit/event-list-controls.test.ts` 通过,4 个前端契约测试通过。
- `npx --yes bun run i18n:sync` 完成;同步报告显示所有 locale `missingCount=0`、`extrasCount=0`。
- `npx --yes bun run build:check` 通过;构建期间仍有既有 route 文件命名提示和 Rspack 持久缓存访问拒绝提示,本轮没有类型或构建错误。
- 浏览器在固定后端 `http://localhost:3000` 切到 Default 主题后验证:任务日志显示临时任务行、模型 `gpt-image-1`、渠道 `#1`;任务详情弹窗显示 Request Model/Actual Model/Result URL;渠道详情弹窗显示渠道名称、类型、状态、额度、分组和模型列表。
- 浏览器验证安全审计事件列表:默认表格不展示正文预览列;点击展开后显示 Prompt preview、Prompt hash、Group 和 Token-bound groups。
- `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/local-test.ps1 -Action verify` 通过 `/api/status`、`http://localhost:3001/`、root/demo 登录和演示数据接口验证。
- Classic 任务日志补齐模型、分组、用户弹窗、渠道弹窗与展开详情;Classic 安全审计列设置持久化并默认隐藏正文预览/分组列。
- `git diff --check` 无空白错误;Windows 工作区提示 `web/default/bun.lock` 下一次 Git 写入会将 LF 替换为 CRLF。
