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

## 验证

- 三个应用容器均为 `running/healthy`，重启次数为 `0`。
- `127.0.0.1:18097`、`18098`、`18099` 的 `/api/status` 均返回 `.294`。
- 公网 `https://zzapi.maolaoapi.com/api/status` 返回 `.294`。
- PostgreSQL、Redis 容器未重建，容器状态和重启次数保持稳定。
- 登录后验证 Classic `/console/benefits`、`/console/redemption`，并验证 Default 对应路由或切换模板后的同等功能入口。

## 回滚

若任一应用节点更新失败，停止后续节点；使用发布前 Compose 备份恢复原镜像配置，仅重建已修改的应用服务。不得重建数据库或 Redis。回滚后重新核对三个端口、容器健康和业务版本。
