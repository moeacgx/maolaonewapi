# maolaoapi Gemini OpenAI 协议与 .320 发布

## 目标与范围

发布 `v1.0.0-rc.10.1.10.320`，把已合入 `custom-main` 的 Gemini OpenAI Images/Chat
转换修复滚动到 maolaoapi 生产三应用。当前生产运行 `.318`。

- 主分支：`custom-main`，发布前基线 `bac7a8f19`。
- #199：`gemini-*-image*` 走 `/v1/images/generations` 与 `/v1/images/tasks` 时转为
  `generateContent`，并把 4xx 原文写进异步 `fail_reason`。
- #201：Gemini/Vertex Gemini 走 `/v1/chat/completions` 与 `/pg/chat/completions`
  时，即使开启透传也强制把 `{messages}` 转成 `contents[].parts[].text`。
- #200：TokensPro overview 渠道并发对齐（同基线已合入，随本次镜像一并发布）。
- 仅部署 CloudSSH“API中转站” `serverId=38`、`hostId=11`，目录 `/home/docker/maolaoapi`。
- 更新顺序：`maolaoapi-slave-1`（18100）→ `maolaoapi-slave-2`（18101）→ `maolaoapi`（18095）。
- 不重建 PostgreSQL、Redis，不更新 zzapi。无数据库结构变更。

## 更新前现场

- 三个应用均为 `running/healthy`，镜像
  `ghcr.io/moeacgx/maolaonewapi:v1.0.0-rc.10.1.10.318`。
- 预检作业：`db060dd2-9f6d-4df1-a200-690683e7bd4c`。

## 发布、验证与回滚

通过 PR 提升 `VERSION`，合并后打 `.320` 标签，等待 GHCR 多架构镜像及 Linux
Release。备份 Compose，只替换三个应用镜像，逐个
`docker compose up -d --no-deps --force-recreate`。每个应用均验证 Docker 健康、
`/api/status` 版本及重启次数后再继续。

若节点健康或版本不符，停止后续滚动并恢复备份中的旧镜像，仅回滚已更新应用。
保留 `.318` 镜像以供回滚，不删除业务数据。
