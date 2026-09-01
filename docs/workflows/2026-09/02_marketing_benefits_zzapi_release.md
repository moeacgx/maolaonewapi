# 营销福利重构 zzapi 发布记录

## 目标与范围

- 发布版本：`v1.0.0-rc.10.1.10.294`。
- 源码范围：营销福利额度展示与双模板重构、福利活动/兑换码/优惠码批量管理、终态券回滚保护，以及合并时保留的最新 `custom-main` 修复。
- 部署目标：仅 CloudSSH 项目“API中转站”的 `serverId=52`，Compose 目录 `/home/docker/zzapi`。
- 应用服务：`zzapi`、`zzapi-slave-1`、`zzapi-slave-2`。
- 明确不更新：`zzapi-postgres`、`zzapi-redis` 和受保护的 `maolaoapi`。

## 发布流程

1. 将当前集成提交合并到最新 `origin/custom-main`，解决文档索引和 Classic 中文 locale 冲突，并保留双方变更。
2. 在两套前端构建和 Go 测试通过后推送 `custom-main` 与同名版本标签。
3. 等待 GitHub Actions 完成 Linux Release 和 GHCR 多架构镜像构建，核对标签指向、镜像摘要及架构。
4. 在更新前备份 `/home/docker/zzapi/docker-compose.yml`，把三个应用服务镜像固定到 `.294` 标签。
5. 按 `zzapi-slave-1`、`zzapi-slave-2`、`zzapi` 顺序逐个拉取和重建；每个节点必须先通过健康、版本、重启次数和端口检查，再继续下一个。

## 实际发布证据

- 推送提交：`39862bb884254e6cb5efc7106091d03532523714`。
- GitHub Actions：Release (Linux) `33543084382`、Publish Docker image (Multi-arch) `33543084439`，均成功。
- 多架构镜像：`ghcr.io/moeacgx/maolaonewapi:v1.0.0-rc.10.1.10.294`。
- 镜像清单摘要：`sha256:821a556b2c66ee894a6e43209a81eca4cf088ccd5ca815a8e123828a343068b3`。
- 架构摘要：`linux/amd64` 为 `sha256:9eb7cceec76d965e029316c8db131fbcfb86fac7836cc4fd1c3e5a2b6950f9db`；`linux/arm64` 为 `sha256:d5f3247bc718b4ea5a7cd4ac616e0961fc923d410c98066465c9dfb9e1b31685`。
- 发布前 Compose 备份：`docker-compose.yml.bak-v1.0.0-rc.10.1.10.294`；备份 SHA-256 `f87612d98c6d9f0fdb52be3dc65870b744e664c501c88836ea0d7befd0c5176c`；更新后 Compose SHA-256 `03c36f08355ea0ea23c2917fb6e512edbd636bf402b4cd15c5ef14d24f145209`。
- CloudSSH 作业：Compose 更新 `3e59d85a-2599-44df-b9c0-cacedbdda555`；`zzapi-slave-1` `20111a74-fa80-4a6a-8f99-0c74081ed66c`；`zzapi-slave-2` `4569fc40-c035-48e8-b165-1cfeada01a74`；`zzapi` `7980bfd5-2096-4cf8-be8d-84f15d4f9996`；最终核验 `43797101-7b2b-48a5-a38f-bb62968bf1c2`。

## 验证

- 三个应用容器均为 `running/healthy`，重启次数为 `0`。
- `127.0.0.1:18097`、`18098`、`18099` 的 `/api/status` 均返回 `.294`。
- 公网 `https://zzapi.maolaoapi.com/api/status` 返回 `.294`。
- PostgreSQL、Redis 容器未重建，容器状态和重启次数保持稳定。
- 登录后验证 Classic `/console/benefits`、`/console/redemption`，并验证 Default 对应路由或切换模板后的同等功能入口。

实际结果：三个应用均为 `running/healthy`、`RestartCount=0`，端口 `18097/18098/18099` 的版本均为
`v1.0.0-rc.10.1.10.294`；公网状态同样返回该版本。PostgreSQL 容器短 ID 仍为
`af1f1e228417`，Redis 容器短 ID 仍为 `07da27a2c193`，均为 `running` 且重启次数为 `0`。
`zzapi-slave-1` 首次重建后健康探针短暂为 `starting`，只读等待后转为 `healthy`，未触发回滚。

## 回滚

若任一应用节点更新失败，停止后续节点；使用发布前 Compose 备份恢复原镜像配置，仅重建已修改的应用服务。不得重建数据库或 Redis。回滚后重新核对三个端口、容器健康和业务版本。
