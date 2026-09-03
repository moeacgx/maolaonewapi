# zzapi 福利与发票 UI 修复发布记录

## 目标与范围

- 发布版本：`v1.0.0-rc.10.1.10.295`。
- 变更范围：Default/Classic 福利页面移除“查看流水”入口；Classic 福利活动和券列表
  操作列固定到右侧并铺满业务面板；Classic 发票中心恢复透明玻璃卡片和全宽表格。
- 部署目标：仅 CloudSSH 项目“API中转站”的 `serverId=52`、`hostId=17`，Compose
  目录 `/home/docker/zzapi`。
- 应用服务：`zzapi-slave-1`、`zzapi-slave-2`、`zzapi`，按此顺序滚动更新。
- 明确不更新：`zzapi-postgres`、`zzapi-redis` 和受保护的 `maolaoapi`。

## 发布前验证

- 确认 `origin/custom-main` 未出现未预期的新提交，集成分支仅包含本次页面与文档变更。
- 执行后端定向 Go 测试，以及 Default/Classic 受影响的前端测试、类型检查和生产构建。
- 推送 `custom-main` 与同名版本标签，等待 GitHub Actions 完成 Linux Release 和 GHCR
  多架构镜像后，核对镜像清单摘要与 `linux/amd64`、`linux/arm64` 架构摘要。

## 远端更新与验证

更新前读取并记录 Compose 文件 SHA-256，创建带版本号的远端备份。每个应用容器更新后，
必须确认容器 `running/healthy`、重启次数为 `0`、对应本地端口 `/api/status` 返回
`v1.0.0-rc.10.1.10.295`，再继续下一个节点。全部节点完成后再次核对 Compose、数据库和
Redis 容器状态，并通过公网状态接口确认版本。

本次执行证据：

- 发布提交：`54c80f8337e53482b1678a218203ca184f719a43`；标签：
  `v1.0.0-rc.10.1.10.295`。
- GitHub Actions：Linux Release `33569157558`、Docker Multi-arch `33569157546`，均成功。
- GHCR manifest：`sha256:8263499586bd89f5b0c63150bf9313edc569b84460b83f3d3630cfa4a07c21e0`，
  主机按该 digest 拉取，amd64/arm64 构建均成功。
- CloudSSH Compose 备份作业：`834cbf68-e802-41de-a446-f9ba8a09f09b`；备份文件
  `docker-compose.yml.bak-v1.0.0-rc.10.1.10.295`，备份 SHA-256
  `03c36f08355ea0ea23c2917fb6e512edbd636bf402b4cd15c5ef14d24f145209`；更新后 Compose
  SHA-256 `80230fe49ae891cf357d35b1b3de3fa72b5b2bb4464cebf6ff23339b1cf24f30`。
- 滚动更新作业：`zzapi-slave-1` 为 `3623e9f6-0d82-4f6b-ace6-4dfd4a1a18e6`，
  `zzapi-slave-2` 为 `673b4e7c-0f68-4b6c-ace6-4dfd4a1a18e6`，主节点 `zzapi` 为
  `ab4b4851-8533-4027-ac97-9f3456b620b0`；三者均成功，版本和健康检查通过。
- 最终核验作业：`63ea0477-581e-40de-b169-a4480a0ac0bb`；端口 `18097/18098/18099`
  与公网 `/api/status` 均返回 `.295`，三个应用 `running/healthy`、重启次数 `0`；
  PostgreSQL 与 Redis 均保持 `running`、重启次数 `0`。

## 回滚

若任一应用节点更新失败，停止后续节点，恢复发布前 Compose 备份中的应用镜像配置，仅
重建已修改的应用服务。不得重建 PostgreSQL 或 Redis；回滚后重新核对三个应用端口、健康
状态、重启次数和版本。
