# v1.0.0-rc.10.1.10.287 生产发版与滚动更新

日期：2026-08-31

## 目标

将 `custom-main` 上已合并的 PR #132、#137、#138 发布为
`v1.0.0-rc.10.1.10.287`，并逐个更新 MaoLaoAPI 生产网关实例。

## 发布内容

- #132：排除支付回调 IP 对请求 IP 和返佣风控日志的污染。
- #137：补充最新基线审计派发计划文档。
- #138：区分客户端取消与零用量响应，修复渠道亲和性缓存命中率和性能失败统计口径。

## GitHub 发布

- 目标仓库：`moeacgx/maolaonewapi`
- 目标分支：`custom-main`
- 版本文件：根目录 `VERSION` 更新为 `v1.0.0-rc.10.1.10.287`。
- 发布触发：推送 `custom-main` 后创建同名 Git 标签，触发 Linux Release 和多架构 Docker 镜像工作流。
- Release 正文需说明客户端取消、零用量统计、支付回调 IP 日志和文档更新，并链接 `.286... .287` 变更记录。

## 生产滚动更新

- 目标主机：CloudSSH `serverId=38`，项目“API中转站”。
- Compose 目录：`/home/docker/maolaoapi`。
- 应用容器：`maolaoapi`、`maolaoapi-slave-1`、`maolaoapi-slave-2`。
- 更新方式：先拉取目标 `.287` 镜像，仅使用 `docker compose up -d --no-deps --force-recreate <service>` 更新应用容器。
- 更新顺序：`maolaoapi` → `maolaoapi-slave-1` → `maolaoapi-slave-2`；每个实例确认 `running/healthy` 后再继续。
- 不重启或修改 PostgreSQL、Redis、Nginx 和其他无关容器。

## 验收与回滚

- 更新前后记录三个应用容器的镜像标签、状态和健康检查结果。
- 确认三个实例继续使用相同的 `TRUSTED_PROXIES`、Redis 和数据库配置。
- 发布后检查 `/api/status`、容器日志中的启动版本和客户端取消/零用量相关错误分类。
- 任一实例健康检查失败时停止后续更新，保留已更新实例并先排查；回滚使用上一已验证镜像 `.286`，不自动重放升级命令。

## 实际结果

- PR #132 已合并，合并提交为 `780b08f50920131217c69a65b15d8ff28064ba67`。
- PR #137 已合并，合并提交为 `13a3223a15609f9a1ea07aefc166d29966674450`。
- PR #138 已合并，合并提交为 `47ccdd9bf5f80beac8ea3a0108db546be7815037`。
- 版本提交为 `e790429c3089de17d846b6e0aa09685c4fbf242c`，已推送到 `custom-main`。
- GitHub Release `v1.0.0-rc.10.1.10.287` 已发布；Linux Release 工作流 `33325063438` 成功，Docker 多架构工作流 `33325063468` 成功。
- GHCR `.287` 多架构 manifest 已创建，包含 Linux amd64/arm64；线上 `latest` 拉取后的镜像 ID 为 `sha256:864945fa8eb3e272ae0dc19aedded4e79d38d2cc365b3714887362a2097332e2`。
- CloudSSH `serverId=38` 预检确认三个应用实例及 PostgreSQL/Redis 均正常运行。
- 按 `maolaoapi` → `maolaoapi-slave-1` → `maolaoapi-slave-2` 顺序逐个执行 `docker compose up -d --no-deps --force-recreate`；每一步完成后再继续下一实例。
- 最终三个应用容器均为 `running|healthy`，并统一使用 `ghcr.io/moeacgx/maolaonewapi:latest` 和上述新镜像 ID；PostgreSQL、Redis 未重建，分别保持 `running`。
- 三个实例端口 `18095`、`18100`、`18101` 的 `/api/status` 均返回成功，响应版本为 `v1.0.0-rc.10.1.10.287`。
- 未执行自动回滚、重复升级或整个 Compose 栈重启。
