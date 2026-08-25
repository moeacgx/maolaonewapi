# 模型广场顶栏与渠道分组显示名回归修复

日期：2026-08-26

## 问题

用户反馈两个 UI 回归：

- Classic 模型广场顶部、顶栏或 hero 区域存在样式和间距异常。
- 渠道列表“分组”列不应直接展示稳定分组 code；多分组需要逐个映射显示名称，缺失显示名称时才回退 code。

## 历史核验

- PR `#68` 已合入 `release/v1.0.0-rc.10.1.10.250`，但未进入 `custom-main`，其渠道列表显示名修复不能视为当前主线已覆盖。
- PR `#72` 已合入 `custom-main`，对应 `v1.0.0-rc.10.1.10.252`，完成 Classic 模型广场新版模板迁移。
- PR `#73`、`#74`、`#75` 已合入 `custom-main`，分别覆盖模型广场顶栏图标白屏、模板细节对齐和嵌套滚动容器收缩。
- PR `#76` 仍打开未合入，包含 Classic 模型广场顶栏精修；本次只提取其中与顶栏用户名空间、容器宽度和断点相关的文件，不带入价格表 badge、卡片元信息和版本号改动。
- PR `#78` 已关闭未合入，且包含动态路由代码；本次没有恢复动态路由相关改动。
- `#78` 中与本次相关但未在当前主线覆盖的片段包括：渠道列表响应补齐 `group_details`，以及 Classic 模型广场长导航/用户名区的顶栏布局恢复；动态路由相关文件全部排除。

## 修复范围

- `GetAllChannels` 在普通列表和 tag mode 列表返回前统一水合 `group_ids` 与 `group_details`。
- `SearchChannels` 的 tag mode 搜索分支返回前同样补齐 `group_details`；非 tag 搜索继续使用 `model.SearchChannels` 现有水合逻辑。
- Classic 模型广场顶栏桌面容器恢复到 `1440px`，收缩态保持完整容器宽度，并为用户名区单独恢复自然按钮宽度和间距。
- Classic 模型广场在小于 `1100px` 时提前收纳为移动菜单，JS resize 关闭逻辑和 CSS 断点保持一致，避免自定义导航、语言/主题/通知按钮和用户区互相挤压。
- Default 渠道卡片视图复用渠道分组显示名映射，避免桌面列显示名称而移动卡片仍展示 code。
- Default 渠道 tag mode 聚合行合并每个子渠道的 `group_details`，避免父级聚合行只保留首个子渠道的显示名。

## 兼容性

- `group` 字段继续保留稳定 code，用于筛选、兼容旧客户端、计费和颜色稳定。
- 展示层优先使用 `group_details[].name`；当响应缺失详情或名称为空时才回退 code。
- 本次不修改模型、分组、汇率配置、后端计价、额度数据或动态路由能力。

## 验证结果

- `go test ./controller -run "Test(GetAllChannelsIncludesGroupDisplayDetails|GetAllChannelsIncludesMultipleGroupDisplayDetails|GetAllChannelsTagModeIncludesGroupDisplayDetails|SearchChannelsTagModeIncludesGroupDisplayDetails)" -count=1 -timeout 60s`：通过。
- `go test` 排除本地缺失前端 dist 的 root 包后，对 92 个后端包执行 `-timeout 60s`：通过；`go test ./... -timeout 60s` 仅 root 包因 `web/classic/dist` 未构建而失败。
- `node --test web/classic/src/channel-group-copy-display-name.test.mjs`：通过。
- `node --test web/classic/src/group-display-name-integration.test.mjs`：通过，确认模型广场、用户、任务日志等既有显示名链路仍消费显示名称并提交内部 code。
- Classic 顶栏静态断言：通过，确认用户名容器、`1440px` 容器宽度、收缩态完整宽度、用户名按钮自然宽度、`1099px` CSS 移动菜单断点和 `1100` JS resize 断点均存在。
- Default 渠道分组静态断言：通过，确认表格列、卡片视图和 tag mode 聚合行均使用/保留显示名映射。
- Classic 渠道分组静态断言：通过，确认渠道表格从 `record.group_details[].name` 取显示名，tag mode 聚合行合并子渠道 `group_details`。
- `git diff --check`：通过。
- 当前工作区未安装 `bun`，且 `web/`、`web/classic/` 均无 `node_modules/.bin`，因此未能执行 Default `typecheck`/`lint`、Vitest 和 Classic `bun test`。本次以 Go 专项测试、现有 Node 测试和静态断言覆盖最小可行验证。
