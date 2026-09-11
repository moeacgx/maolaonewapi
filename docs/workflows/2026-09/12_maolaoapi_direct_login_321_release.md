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

待预检后填写。

## 发布、验证与回滚

通过 PR 提升 `VERSION`，合并后打 `.321` 标签，等待 GHCR 多架构镜像及 Linux
Release。备份 Compose，只替换三个应用镜像，逐个
`docker compose up -d --no-deps --force-recreate`。每个应用均验证 Docker 健康、
`/api/status` 版本及重启次数后再继续。

若节点健康或版本不符，停止后续滚动并恢复备份中的旧镜像，仅回滚已更新应用。
保留 `.320` 镜像以供回滚，不删除业务数据。

上线后需从面板重新打开一次画布/扩展，才会签发 30 天新票；旧的 8 小时 / 2 小时票不会自动变长。
