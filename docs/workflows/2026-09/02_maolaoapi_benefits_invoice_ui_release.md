# maolaoapi 福利与发票 UI 修复发布记录

## 目标与范围

- 发布版本：`v1.0.0-rc.10.1.10.295`。
- 变更范围：Default/Classic 福利页面移除“查看流水”入口；Classic 福利活动和券列表
  操作列固定到右侧并铺满业务面板；Classic 发票中心恢复透明玻璃卡片和全宽表格。
- 部署目标：仅 CloudSSH 项目“API中转站”的 `serverId=38`、`hostId=11`，Compose
  目录 `/home/docker/maolaoapi`。
- 应用服务：`maolaoapi-slave-1`、`maolaoapi-slave-2`、`maolaoapi`，按此顺序逐个更新。
- 明确不更新：`maolaoapi-postgres`、`maolaoapi-redis`，也未操作其他项目或主机。

## 发布前核验

- 源码发布提交：`54c80f8337e53482b1678a218203ca184f719a43`；版本标签：
  `v1.0.0-rc.10.1.10.295`。
- GHCR 多架构 manifest：
  `sha256:8263499586bd89f5b0c63150bf9313edc569b84460b83f3d3630cfa4a07c21e0`；Linux Release
  和 Docker Multi-arch 工作流均成功。
- 更新前三个应用均运行 `.latest`/`.288`，健康状态正常；Compose 原文件 SHA-256：
  `c4627a280c88b4a524866238afb77159e39ddfd7306ca45ef910808a4a2d4030`。

## 更新与回滚保护

- CloudSSH 预检、备份及 Compose 更新作业：`bf75a06b-3027-40e9-8cbc-caa343a98bf8`。
- 备份文件：`docker-compose.yml.bak-v1.0.0-rc.10.1.10.295`，备份 SHA-256 与原文件
  一致；更新后 Compose SHA-256：
  `8a536de9dbd1d8f6603e38f25bb3631ca3639983ee1f3cbed8597b1217d628b3`。
- 滚动更新作业：`maolaoapi-slave-1` 为 `b4bcc6bd-468b-4cd9-873d-668b384d2d02`，
  `maolaoapi-slave-2` 为 `c6701755-bd01-4460-8add-cc169a025312`，主节点 `maolaoapi`
  为 `553e7cf8-0c41-49e6-ac9b-16016a01d3db`。每个节点均在继续前通过健康、版本和重启
  次数检查。
- 任一节点失败时停止后续更新，恢复上述 Compose 备份中的应用镜像配置，仅重建已修改的
  应用服务，不重建 PostgreSQL/Redis。

## 最终验证

- CloudSSH 最终核验作业：`6c9d6ff2-b64a-4753-85ed-0fcff771fce3`。
- `maolaoapi-slave-1`、`maolaoapi-slave-2`、`maolaoapi` 均为 `running/healthy`，重启次数
  均为 `0`，统一使用 `.295` 镜像及上述 digest。
- 本地端口 `18100`、`18101`、`18095` 和公网 `https://maolaoapi.com/api/status` 均返回
  `v1.0.0-rc.10.1.10.295`。
- `maolaoapi-postgres` 与 `maolaoapi-redis` 均保持 `running`、重启次数 `0`，容器 ID 未变。
