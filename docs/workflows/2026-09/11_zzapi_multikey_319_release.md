# zzapi 密钥聚合编辑与定价标签发布

## 目标与范围

用户授权将 PR #196 的 Classic 编辑渠道密钥聚合功能更新到 zzapi，并明确要求合入
zzapi 测试镜像中的 `a3aaa9d7b` 定价标签样式修复，避免更新时回退。
发布版本为 `v1.0.0-rc.10.1.10.319`。

- 主分支：`custom-main`，发布前基线 `0bb5e46e0`。
- 样式修复：档位标签按内容宽度展示，分组价格表复用相同标签；不改变价格计算或结算。
- 密钥聚合接口、安全及模板边界见 [功能记录](09_classic_channel_multikey_conversion.md)。
- 仅部署 CloudSSH“API中转站”项目 `serverId=52`、`hostId=17`，目录 `/home/docker/zzapi`。
- 更新顺序：`zzapi-slave-1`（18098）→ `zzapi-slave-2`（18099）→ `zzapi`（18097）。
- 不重建 PostgreSQL、Redis，不更新 maolaoapi。无数据库结构变更。

## 更新前现场

- 三个应用均为 `running/healthy`，镜像 `ghcr.io/moeacgx/maolaonewapi:test-classic-tier-pill-20260909-a3aaa9d7b`。
- Compose SHA-256：`5ade2cc7608cd13a915acee2a93a19997a02afd505bb58aecef5a43e340423a7`。
- PostgreSQL 容器 ID：`af1f1e228417`，Redis：`07da27a2c193`；部署后核对未变化。

## 发布、验证与回滚

合入样式提交后，通过 PR 检查、前后端 CI，再为合并提交打 `.319` 标签。
等待 GHCR 多架构镜像及 Linux Release 完成；核对标签对应源码后拉取镜像。
备份 Compose，只替换三个应用的镜像，逐个 `docker compose up -d --no-deps --force-recreate`。
每个应用均验证 Docker 健康状态、`/api/status` 版本及重启次数后再继续。
最终核验三个端口、公网状态和数据库/缓存容器身份。

若节点健康或版本不符，停止后续滚动并恢复备份中的旧镜像，仅回滚已更新应用。
保留测试镜像以供回滚，不删除任何业务数据或镜像。

## 验证结果

- 合入后的密钥聚合和定价视觉契约测试：8 项通过。
- CI、镜像摘要和远端滚动结果在发布后补录。
