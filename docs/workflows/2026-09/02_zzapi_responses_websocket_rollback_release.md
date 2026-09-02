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

- 受影响 Go 测试全部通过：`go test ./model ./service ./relay/... ./controller ./router ./middleware -count=1 -timeout 60s`。
- `git diff --check` 通过；发布提交为 `7eef64713f8203d270c6382a4f6b5963685df5e5`。
- Linux Release 工作流 `33631151025` 成功，GHCR 多架构工作流 `33631150801` 成功。
- GHCR manifest 为
  `sha256:2d1b616aec8bef57599c43d4fc7caef63a9a77c3a8c7894c00c6f6541e78b31b`，
  amd64/arm64 构建和签名均成功。

## 远端更新与验收

更新前读取 Compose SHA-256 并创建带版本号的远端备份。仅替换三个应用服务镜像，
每个容器更新后确认 `running/healthy`、重启次数为 `0`，并核对本地端口和公网
`/api/status` 返回 `.298`。最终确认 PostgreSQL/Redis 状态保持运行，未执行重建。

## 执行证据

- CloudSSH 目标：项目“API中转站” `serverId=52`、`hostId=17`。
- 更新前 Compose SHA-256：
  `b9d4e28f0131cc8a7e9ef533d24ada2ac0821c4d26471e5208093638766eb27f`。
- 备份作业 `b0d689a2-27a5-4529-8887-5452e8660311` 创建
  `docker-compose.yml.bak-v1.0.0-rc.10.1.10.298`，备份 SHA-256 与原文件一致。
- Compose 更新作业 `c8762dba-a3f6-453e-be39-96676885437e` 完成三处应用镜像替换，
  更新后 SHA-256 为
  `8100238ca16f2395003de9f18a601097b668e13cb24d246c3350b7f5da311ab0`。
- 滚动更新顺序及验收作业：
  `zzapi-slave-1`（重建 `e333abda-7025-4dd4-ab28-aec8297018c9`，端口
  `18098` 验收 `3a360604-f7bd-4972-8114-4e5203f37b73`）；
  `zzapi-slave-2`（重建 `97c842bf-3130-4100-9e3b-b8eb9e7ef5a0`，端口
  `18099` 验收 `16120d98-d36b-4a9e-bb84-c2eda2cc4cfa`）；
  `zzapi`（重建 `055e6e06-04cf-468c-bb42-0009194ad772`，端口
  `18097` 验收 `17fb3e7c-993a-4534-83ed-6a3159d6e994`）。
- 最终核验作业 `4ad74c61-1f7a-4961-b0d0-0937d73fa799`：三个端口和公网
  `https://zzapi.maolaoapi.com/api/status` 均返回
  `v1.0.0-rc.10.1.10.298`；三个应用均为 `running healthy`、重启次数 `0`。
- 最终应用容器镜像均为 `.298` manifest；主机记录的 RepoDigest 与上述 manifest 一致。
  PostgreSQL/Redis 保持 `running`、重启次数 `0`，本次未操作其服务。

## 回滚

若任一应用节点更新失败，停止后续节点，恢复发布前 Compose 备份中的应用镜像配置，
仅重建已经修改的应用服务；不得重建 PostgreSQL 或 Redis。回滚后重新核对三个应用
端口、健康状态、重启次数和版本。
