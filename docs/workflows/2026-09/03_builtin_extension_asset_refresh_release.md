# 内置扩展资源刷新修复 `.302` 发布记录

## 目标与范围

- 发布版本：`v1.0.0-rc.10.1.10.302`。
- 修复内容：内置扩展同版本时校验并刷新嵌入资源，解决持久化旧原生入口导致的宿主 SDK
  加载失败；覆盖 Default/Classic 对话归档入口和其他内置静态扩展资源。
- 部署目标：仅 CloudSSH 项目“API中转站”的 `serverId=52`，Compose 目录
  `/home/docker/zzapi`。
- 应用服务：`zzapi-slave-1`、`zzapi-slave-2`、`zzapi`，按此顺序滚动更新。
- 明确不更新：`zzapi-postgres`、`zzapi-redis` 和受保护的 `maolaoapi`。

## 发布与验收

1. 推送 `custom-main` 的版本提交和 `v1.0.0-rc.10.1.10.302` 标签。
2. 等待 Linux Release 与 GHCR 多架构镜像工作流成功，核对镜像 manifest 摘要。
3. 更新前读取并备份 `/home/docker/zzapi/docker-compose.yml`，只替换三个应用服务镜像。
4. 每个应用更新后核对 `running/healthy`、重启次数、容器内版本和本地端口状态，再继续下一个。
5. 发布后使用 Root 登录态分别打开 Default 与 Classic 对话归档页面，确认原生入口加载成功，
   并检查 `/api/extensions/conversation-archive/config` 返回成功；同时确认安全审计页面主体
   可打开。

## 回滚

任一应用更新失败时停止后续节点，使用发布前 Compose 备份恢复上一已验证镜像，仅回滚已修改的
应用服务；不重建 PostgreSQL/Redis，不操作 `maolaoapi`。
