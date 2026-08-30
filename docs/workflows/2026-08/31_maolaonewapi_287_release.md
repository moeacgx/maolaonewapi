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

本页在发版和逐实例更新完成后补充 GitHub Release、Actions、镜像摘要、三个容器状态及健康检查证据。
