# Default 操练场分组权限回归修复

日期：2026-09-02

## 问题与根因

Default 操练场通过 `POST /pg/chat/completions` 提交当前选择的 `group`。该路由使用
面板 `UserAuth`，没有 API 令牌分组绑定；但请求解析曾把页面选择的分组写入
`ContextKeyTokenGroup`。通用显式分组校验上线后，该值被误认为令牌绑定分组，因缺少
`ContextKeyTokenGroups` 而统一返回 403“无权访问该分组”。

`auto` 还会经过旧的操练场重复校验，而普通可选分组判定明确排除 `auto`，因此也会
被错误拒绝。线上 request id `202609020540331340654398268d9d6KBpqs3qL` 已只读确认在
`maolaoapi-slave-2` 的 `/pg/chat/completions` 命中该 403；本次未修改生产环境。

## 修改范围与契约

- `/pg/chat/completions` 不再把请求体分组写入 API 令牌绑定上下文。
- 操练场显式命名分组仍必须属于当前用户可选分组。
- `auto` 仅在当前用户的可用分组配置包含 `auto` 时允许，并保持非显式福利分组语义。
- 删除操练场第二次解析和重复校验，分组选择在进入选渠前只处理一次。
- 标准 `/v1/**` 请求继续执行原有 API 令牌分组绑定门禁，未放宽令牌权限。
- 鉴权、分发器和 relay 重试的分组错误将内部分组 code/alias 转换为当前分组名称；
  无法查询名称时回退原标识。多分组错误保留 `multi(...)` 包装，自动分组在已确定
  具体分组时保留 `auto(名称)` 形式。内部路由、计费、日志筛选仍使用稳定 code。

本次修复后端共享路由逻辑，直接覆盖截图中的 Default 模板；未修改 Default 或 Classic
前端源码。Classic 使用同一 `/pg/chat/completions` 后端接口，因此同时受益，但未做
Classic 页面改造。

## 验证计划

- 回归普通操练场分组：解析后不得污染 `ContextKeyTokenGroup`，用户可用分组可以进入
  选渠并保留显式福利分组标记。
- 回归自动分组：配置可用时允许，未配置时拒绝。
- 回归标准 `/v1/chat/completions`：显式令牌分组绑定和越权拒绝语义保持不变。
- 回归错误展示：当前 code、历史 alias 和逗号分隔的多分组均显示当前名称，未知标识
  保留原值；`auto` 不猜测未确定的具体分组。
- 执行 middleware 定向测试、完整 middleware 测试、相关 controller 测试、Go 格式化、
  `go vet`、`git diff --check`；仓库构建条件允许时执行根模块构建。

## 兼容性与发布边界

没有数据库迁移、配置变更或前端接口变更。部署后现有分组配置立即生效；回滚仅需恢复
本次后端提交。当前工作项只完成代码与本地验证，不包含 zzapi 或 maolaoapi 发布。
