# maolaoapi 直登会话寿命与 .321 发布

## 目标与范围

发布 `v1.0.0-rc.10.1.10.321`，把已合入 `custom-main` 的直登会话寿命修复滚动到
maolaoapi 生产三应用。当前生产运行 `.320`。

- 主分支：`custom-main`，发布前基线 `c07eb60b4`（#204 已合并）。
- #204：直登会话票（画布 `/canvas`、扩展 `/api/extensions`）寿命改为
  `DirectLoginSessionTTL`（与登录会话相同，30 天）；`UserSessionAuth` 在
  Authorization 为残留 PAT、过期 JWT 或对应会话已撤销的 JWT 时，回落到仍有效的
  path Cookie。单独 PAT / `sk-` 仍不能直登。面板 15 分钟 Access Token 不变。
- 仅部署 CloudSSH“API中转站” `serverId=38`、`hostId=11`，目录 `/home/docker/maolaoapi`。
- 更新顺序：`maolaoapi-slave-1`（18100）→ `maolaoapi-slave-2`（18101）→ `maolaoapi`（18095）。
- 不重建 PostgreSQL、Redis，不更新 zzapi。无数据库结构变更。

## 更新前现场

- 三个应用均为 `running/healthy`，镜像
  `ghcr.io/moeacgx/maolaonewapi:v1.0.0-rc.10.1.10.320`。
- 预检作业：`f6149345-574f-4866-9dd4-ca0bec30f924`。
- 本地端口 `18100`、`18101`、`18095` 的 `/api/status` 均为 `.320`。

## 发布、验证与回滚

通过 PR 提升 `VERSION`，合并后打 `.321` 标签，等待 GHCR 多架构镜像及 Linux
Release。备份 Compose，只替换三个应用镜像，逐个
`docker compose up -d --no-deps --force-recreate`。每个应用均验证 Docker 健康、
`/api/status` 版本及重启次数后再继续。

若节点健康或版本不符，停止后续滚动并恢复备份中的旧镜像，仅回滚已更新应用。
保留 `.320` 镜像以供回滚，不删除业务数据。

上线后需从面板重新打开一次画布/扩展，才会签发 30 天新票；旧的 8 小时 / 2 小时票不会自动变长。

## 发布结果

- PR #204 已合并，合并提交 `c07eb60b44c07e1ea2012573ca9ec6a27d72d7f4`。
- PR #205 已合并，标签指向 `18669e324a7026a06a69b03200abd5b72db894d5`。
- Linux Release `34635019105`、Docker Multi-arch `34635019015` 均成功。
- 镜像：`ghcr.io/moeacgx/maolaonewapi:v1.0.0-rc.10.1.10.321`。
- 多架构摘要：`sha256:a148de55be70b2f82cad3c2a0079eb04d9020996340023a33c1c95c726e293b4`。
- amd64 层：`sha256:1f57ba28821909ec0bef1d514d25cb6556078ac19986098bb730cc48fe5b1de3`。

## 滚动更新证据

- 备份与拉取作业：`da9793ea-963f-4500-8e28-10b7d633c5a7`。
- Compose 备份：`/home/docker/maolaoapi/docker-compose.yml.bak-v1.0.0-rc.10.1.10.321`，
  SHA-256 与更新前一致：`e0f0eee5f0b6200c73a73a760e53b3fdc4ad6223f85d1721c6d3df7fd22e0ca9`。
- 更新后 Compose SHA-256：`12abe25424315128ca16a4ff7266a269b503f48e918e2c27a2f3694762056d7f`。
- `maolaoapi-slave-1` 作业：`22f779bb-2fea-459c-b7a5-e33d8eb6c9a4`，新容器 `900707a01d94`。
- `maolaoapi-slave-2` 作业：`f8f1565a-ae5b-437c-b42b-ab64711e647f`，新容器 `4686cb125b49`。
- `maolaoapi` 作业：`c53f95b2-482f-4402-a6e8-422fbdc6a31b`，新容器 `0be6b23b9336`。
- 三个应用均为 `running/healthy`，重启次数 0，镜像 `.321`。
- 本地端口 `18100`、`18101`、`18095` 的 `/api/status` 均返回 `v1.0.0-rc.10.1.10.321`。
- 公网 `https://maolaoapi.com/api/status` 返回同一版本。
- `maolaoapi-postgres`（`bee76d26e45d`）与 `maolaoapi-redis`（`4b5b4ae42b6f`）
  容器 ID 未变，均 `running`、重启次数 0。
- 未操作 zzapi，未重建数据库或缓存。

