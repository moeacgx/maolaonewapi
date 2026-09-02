# zzapi WebSocket 徽标发布记录

日期：2026-09-02

## 目标与基线

- 发布版本：`v1.0.0-rc.10.1.10.297`。
- 发布基线：已在 zzapi 运行的 `v1.0.0-rc.10.1.10.296`，保留 Responses
  WebSocket 后端能力；其上叠加操练场分组权限/错误名称修复和使用日志 `WS` 徽标。
- 部署目标：仅 CloudSSH `serverId=52`、项目“API中转站”、Compose 目录
  `/home/docker/zzapi`。
- 应用服务：`zzapi-slave-1`、`zzapi-slave-2`、`zzapi`，按此顺序滚动更新。
- 明确不更新：`zzapi-postgres`、`zzapi-redis` 和受保护的 `maolaoapi`。

## 行为契约

- Default 和 Classic 使用日志仅在 `other.transport = websocket` 或历史兼容字段
  `other.ws = true` 时显示 `WS` 徽标。
- 普通 HTTP/SSE 流式请求不显示 WS 徽标。
- 使用日志、操练场权限、错误分组名称和 Responses WebSocket 后端行为一起发布，
  防止从 `.296` 发布线切换到 `custom-main` 时丢失已上线能力。

## 发布流程

1. 在本地以 `.296` 标签为基线，逐段合并 `custom-main` 的已合入修复和 WS 徽标提交。
2. 运行受影响 Go/Classic 测试、`git diff --check` 和 locale JSON 校验。
3. 推送 `v1.0.0-rc.10.1.10.297`，等待 Linux Release 与 GHCR 多架构镜像成功，核对
   manifest digest 和两个架构 digest。
4. 更新前读取 Compose SHA-256 并创建带版本号的远端备份；仅替换三个应用服务镜像。
5. 按 slave-1、slave-2、主节点顺序执行 `docker compose pull` 与
   `docker compose up -d --no-deps --force-recreate`，每步确认 healthy、版本和重启次数。

## 验证与回滚

- 全部应用节点完成后核对本地端口 `18097/18098/18099` 和公网 zzapi 状态接口版本，
  并确认 PostgreSQL/Redis 状态与重启次数未变化。
- 任一应用节点失败时停止后续节点，使用发布前 Compose 备份恢复应用镜像；不得重建
  PostgreSQL 或 Redis。当前发布不涉及数据库迁移。

## 当前边界

本记录只覆盖 zzapi 发布，不包含 maolaoapi 生产部署。所有实时容器、镜像和健康状态均以
CloudSSH 发布前后核验结果为准。
