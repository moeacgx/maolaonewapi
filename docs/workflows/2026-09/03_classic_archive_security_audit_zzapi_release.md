# Classic 对话归档与安全审计 zzapi 发版记录

## 目标与范围

- 发布版本：`v1.0.0-rc.10.1.10.300`。
- 源码基线：`origin/custom-main` 的合并提交 `b5cab45d0`，即 PR #154。
- 修复内容：Classic 对话归档入口改用 Classic 宿主 SDK；Classic 安全审计修复
  `policy_action_sources` 归一化异常，并兼容新旧窗口累计字段。
- 部署目标：CloudSSH 项目“API中转站”的 `serverId=52`，Compose 目录
  `/home/docker/zzapi`。
- 应用服务：`zzapi-slave-1`、`zzapi-slave-2`、`zzapi`，按此顺序逐个更新。
- 排除范围：不修改或重启 `zzapi-postgres`、`zzapi-redis`；不触及
  `maolaoapi` 生产集群。

## 发布前状态

- zzapi 三个应用服务当前镜像均为
  `ghcr.io/moeacgx/maolaonewapi:v1.0.0-rc.10.1.10.298`。
- 三个应用服务均为 `running/healthy`；两个数据容器均为 `running`。
- 发布前 Compose SHA-256：
  `8100238ca16f2395003de9f18a601097b668e13cb24d246c3350b7f5da311ab0`。
- 磁盘使用率为 24%，具备拉取和重建镜像的空间。

## 发布与部署流程

1. 从最新 `origin/custom-main` 创建独立发版分支，更新 `VERSION` 并执行受影响测试和
   `git diff --check`。
2. 推送发版提交和同名标签，等待 Linux Release 与 GHCR 多架构镜像工作流完成；
   核对 Release 资产及 amd64/arm64 manifest。
3. 在远端创建带版本号和时间戳的 Compose 备份，只把三个应用服务镜像替换为 `.300`。
4. 按 `zzapi-slave-1`、`zzapi-slave-2`、`zzapi` 顺序执行 pull 和强制重建。每个服务
   必须在继续前确认 `running/healthy`、对应端口返回 `.300`，且没有新增重启。
5. 最终核对三个应用服务、PostgreSQL、Redis、Compose 镜像和三个本地状态端点。

## 验证基线

- PR #154 的前端类型检查、前端测试、Classic 构建和 PR 后端全量检查通过。
- 本地 `npm run test:native`、Classic 安全审计 33 项测试、`go test ./extension`、
  `go test ./router -run TestSecurityAudit -count=1` 和 `git diff --check` 通过。
- 合并后后端 CI 首次因测试临时目录清理竞态失败；失败任务重跑后全量通过，未出现
  代码断言失败。

## 回滚

若任一应用节点更新失败，立即停止后续节点，使用发布前 Compose 备份恢复 `.298`
镜像，仅重建已经修改的应用服务。回滚后重新核对健康状态、重启次数和本地状态端点；
不得重建 PostgreSQL 或 Redis。
