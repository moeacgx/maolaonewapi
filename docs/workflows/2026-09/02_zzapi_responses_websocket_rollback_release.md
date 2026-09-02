# zzapi Responses WebSocket 回滚发布记录

## 目标与范围

- 发布版本：`v1.0.0-rc.10.1.10.298`。
- 回滚提交：`7237644bb9ece571c03fab4cd39a09d1fd840d48`。
- 发布目标：CloudSSH 项目“API中转站”的 `serverId=52`，Compose 目录
  `/home/docker/zzapi`。
- 应用服务：`zzapi-slave-1`、`zzapi-slave-2`、`zzapi`，按此顺序滚动更新。
- 明确不更新：`zzapi-postgres`、`zzapi-redis` 和受保护的 `maolaoapi`。

## 行为契约

- 移除下游 `GET /v1/responses` WebSocket 路由、会话桥接和对应配置入口。
- Codex/CCS Responses 请求恢复使用 HTTP/SSE；既有 `/v1/realtime` WebSocket 不变。
- 保留 `4f467b28e` 操练场分组权限与错误名称显示修复。
- 不涉及数据库迁移，不重建 PostgreSQL 或 Redis。

## 发布前验证

- 运行受影响 Go 测试和 `git diff --check`。
- 推送 `v1.0.0-rc.10.1.10.298` 标签，等待 Linux Release 与 GHCR 多架构镜像成功。
- 核对 GHCR `.298` manifest 及 amd64/arm64 镜像摘要后再更新 Compose。

## 远端更新与验收

更新前读取 Compose SHA-256 并创建带版本号的远端备份。仅替换三个应用服务镜像，
每个容器更新后确认 `running/healthy`、重启次数为 `0`，并核对本地端口和公网
`/api/status` 返回 `.298`。最终确认 PostgreSQL/Redis 状态和容器 ID 未变化。

## 回滚

若任一应用节点更新失败，停止后续节点，恢复发布前 Compose 备份中的应用镜像配置，
仅重建已经修改的应用服务；不得重建 PostgreSQL 或 Redis。回滚后重新核对三个应用
端口、健康状态、重启次数和版本。

## 执行证据

（发布完成后补充 GitHub Actions、镜像摘要、CloudSSH 作业和线上验收结果。）
