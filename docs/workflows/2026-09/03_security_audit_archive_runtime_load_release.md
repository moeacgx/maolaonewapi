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

## 实际执行证据

- 发布提交：`aaea3199a`；标签：`v1.0.0-rc.10.1.10.301`；基于合并提交
  `978b9d1cc`。
- GitHub Actions：Linux Release `33729992635`、Docker Multi-arch
  `33729992570`，均成功；GHCR manifest digest：
  `sha256:714aef368ddea001eb732f6b27c01897ee40036af7d4b16a15e5953e6b6c681f`，
  同时包含 `linux/amd64` 与 `linux/arm64`。
- Compose 备份作业：`dda3206d-bab4-4d2f-af99-d0e83199e808`；备份文件
  `docker-compose.yml.bak-v1.0.0-rc.10.1.10.301`；备份和发布前 Compose
  SHA-256 均为 `13e93229c00aa83503fa5f2ead3cd5a5afb1cd55d17245d16930915acf524be4`。
- 滚动更新作业：`zzapi-slave-1` 为 `9490d79d-552d-405c-b112-1403b82ddb45`，
  `zzapi-slave-2` 为 `75cabf09-3578-4cfb-a2c6-731b822b2c4c`，主节点 `zzapi` 为
  `9194de6f-f71b-475f-9d3c-45ed346b5dd2`；三者均成功。
- 节点验证：三个应用均为 `running|healthy|0`，镜像均为
  `ghcr.io/moeacgx/maolaonewapi:v1.0.0-rc.10.1.10.301`；端口
  `18097/18098/18099` 的 `/api/status` 均包含 `.301`。安全审计和归档路由外壳
  返回 `200`，未授权的安全审计配置接口返回 `401`，认证边界保持不变。
- 最终核验作业：Compose 状态 `3a4c16ef-c55e-4711-b680-ebb1f3f01217`、容器状态
  `c0cb5b77-10e9-4e19-982d-48dfb38d2bce`、端口和公网状态作业分别为
  `f1b576b7-8162-40d8-be7b-ff7a1693d663`、`c19aa15b-f780-4de9-a5c6-ad25e5471aaa`、
  `8d552345-280b-4d0a-a9c5-94b61c896751`、`c051404c-a963-434e-bc7a-39d51c607bd7`。
  PostgreSQL 与 Redis 均保持 `running|0`，未重建。
- 更新后 Compose SHA-256：`f30482c6c5be2c936cdefb0617627f383c7bf122a7fcd2943625146c85d808cc`。

## 回滚

若任一应用节点更新失败，停止后续节点，恢复发布前 Compose 备份中的应用镜像配置，仅重建已修改的应用服务。不得重建 PostgreSQL 或 Redis；回滚后重新核对三个应用端口、健康状态和版本。
