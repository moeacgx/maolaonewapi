# maolaoapi Classic 模型详情动态计费发布记录

日期：2026-09-09

## 目标与范围

- 发布版本：`v1.0.0-rc.10.1.10.318`。
- 变更范围：Classic 模型详情按分组倍率展开动态计费单价，并提高定价卡片对比度。
- 部署目标：仅 CloudSSH 项目“API中转站”的 `serverId=38`、`hostId=11`，Compose
  目录 `/home/docker/maolaoapi`。
- 应用服务：`maolaoapi-slave-1`、`maolaoapi-slave-2`、`maolaoapi`，按此顺序逐个更新。
- 明确不更新：`maolaoapi-postgres`、`maolaoapi-redis`，也未操作 zzapi 或其他主机。

## 发布前核验

- 源码发布提交：`c05667bde`；版本标签：`v1.0.0-rc.10.1.10.318`。
- GHCR 多架构 manifest：
  `sha256:6bfa91b376f9e75c791d4e55412de669dda015081b94958e016d04c265fcc873`。
- 更新前三个应用均运行 `v1.0.0-rc.10.1.10.317`，健康状态正常；Compose 原文件
  SHA-256 `c6694fba567de6226e530d466dcecfd1282ba2cac49fc6e7a6d1d3ae2c3dc30e`。

## 更新与回滚保护

- CloudSSH 备份及 `maolaoapi-slave-1` 滚动作业：`6479ae04-bd6e-4a08-a2df-f264d62c07ad`。
- 备份文件：`docker-compose.yml.bak-v1.0.0-rc.10.1.10.318`，备份 SHA-256 与原文件
  一致；更新后 Compose SHA-256：
  `b669a392c295f1f22f64aa6c09c4a1c2e370f4817d6fc05cafdb5cd7cb17318f`。
- 滚动更新作业：`maolaoapi-slave-2` 为 `d689d3b2-0ead-4ffb-b2f9-c5a3b5ba8ffe`，
  主节点 `maolaoapi` 为 `40670dde-6ba1-40f2-a5fb-8b734f86071c`。
  每个节点均在继续前通过健康、版本和重启次数检查。
- 任一节点失败时停止后续更新，恢复上述 Compose 备份中的应用镜像配置，仅重建已修改的
  应用服务，不重建 PostgreSQL/Redis。

## 最终验证

- 本地端口 `18100`、`18101`、`18095` 均返回 `v1.0.0-rc.10.1.10.318`。
- 公网 `https://maolaoapi.com/api/status` 返回同一版本（curl 作业
  `5b40760a-07e9-4c19-96aa-08ea6812134b`）。
- 三个应用均为 `running/healthy`，重启次数均为 `0`。
- `maolaoapi-postgres` 与 `maolaoapi-redis` 均保持 `running`、重启次数 `0`，容器 ID
  未变。
