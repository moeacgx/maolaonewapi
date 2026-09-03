# 安全审计与对话归档 `.301` 发布记录

## 目标与范围

- 发布版本：`v1.0.0-rc.10.1.10.301`。
- 源码基线：`custom-main` 合并提交 `978b9d1cc`，对应 PR #155。
- 修复内容：Classic 原生扩展使用登录刷新后的 API 实例；对话归档与安全审计的非核心加载请求不再阻塞页面主体；补充请求序号、卸载保护和保存/刷新竞态保护。
- 部署目标：仅 CloudSSH 项目“API中转站”的 `serverId=52`，Compose 目录 `/home/docker/zzapi`。
- 应用服务：`zzapi-slave-1`、`zzapi-slave-2`、`zzapi`，按此顺序逐个更新。
- 明确不更新：`zzapi-postgres`、`zzapi-redis` 和受保护的 `maolaoapi`。

## 发布前验证

- Classic Node 回归测试：40/40 通过。
- 相关 Go 定向测试：`controller`、`router`、`extension`、`service` 通过。
- 合并后 `custom-main` CI：前端 typecheck/test/build 与后端 vet/build/test 均通过（Run `33729452158`）。
- 当前本机未安装 Bun；Classic 生产构建由 GitHub Actions 完成。

## 发布与远端更新

1. 在合并后的 `custom-main` 上将根目录 `VERSION` 更新为 `.301`，提交并推送同名标签。
2. 等待 Linux Release 与 GHCR 多架构工作流完成，核对 Release 资产、镜像 manifest 和 amd64/arm64 架构摘要。
3. 更新前读取 `/home/docker/zzapi/docker-compose.yml` SHA-256，并创建带 `.301` 的远端备份。
4. 仅替换三个应用服务镜像，按 `zzapi-slave-1`、`zzapi-slave-2`、`zzapi` 顺序滚动更新。
5. 每个服务更新后确认 `running/healthy`、重启次数、对应本地端口 `/api/status` 版本，再继续下一个；数据库和 Redis 不重建。

## 回滚

若任一应用节点更新失败，停止后续节点，恢复发布前 Compose 备份中的应用镜像配置，仅重建已修改的应用服务。不得重建 PostgreSQL 或 Redis；回滚后重新核对三个应用端口、健康状态和版本。
