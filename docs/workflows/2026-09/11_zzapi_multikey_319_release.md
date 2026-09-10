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
- PR #198 已合并，发布标签指向 `304dd0dd1e72daeed549220807a31b87de7304c3`。
- PR CI `34500454988`、合并后 CI `34501037387` 均成功。
- Linux Release `34501049652`、Docker Multi-arch `34501049635` 均成功。
- 镜像：`ghcr.io/moeacgx/maolaonewapi:v1.0.0-rc.10.1.10.319`。
- 多架构摘要：`sha256:8f6e34ce50495ba8e66cfa2303ffe4680149e35c0bd98e1a9ad687a9eaae155c`。
- 实际 amd64 镜像 ID：`sha256:78951489ca125cd8d5807415f60c65dbe3600cf9adf102f87d1d8430c0625984`；OCI revision 与发布提交一致。

## 滚动更新证据

- 备份与配置作业：`c8a812b0-8e4f-4247-a5fe-3607bb467ea5`。
- Compose 备份：`/home/docker/zzapi/docker-compose.yml.bak-v1.0.0-rc.10.1.10.319`，
  SHA-256 与更新前一致；仅三个应用的 image 值发生变化，已做解析后完整配置对比。
- 更新后 Compose SHA-256：`3f8444d2fdf823b705f2387e01031b5432bb60d38f55703d7481e4a671f94189`。
- `zzapi-slave-1` 作业：`7d6994aa-6754-43b1-b6b2-57b89a7c9939`，新容器 `6c55a7724ab4`。
- `zzapi-slave-2` 作业：`dffddf2c-50ff-4efb-b88e-f3d14f99554b`，新容器 `9ecccea76e26`。
- `zzapi` 作业：`789d7b65-08e6-496f-8ecd-0db39130cc62`，新容器 `b68c3c5dba21`。
- 三个作业均先确认当前节点健康、版本 `.319`、重启次数 0 和其余应用容器 ID 不变，再继续。
- 终验作业：`7425f180-ba87-4883-b0b1-72c051e40564`；北京时间 2026-09-11 00:31 完成。
- `18097/18098/18099` 与 `https://zzapi.maolaoapi.com/api/status` 均返回 `.319`，三个应用健康且重启次数为 0。
- PostgreSQL 与 Redis 的容器 ID 与更新前一致，均运行正常、重启次数 0；未操作 maolaoapi。
- 线上验证覆盖镜像来源、版本、健康与容器边界；未创建或转换真实渠道，也未发起付费推理请求。
