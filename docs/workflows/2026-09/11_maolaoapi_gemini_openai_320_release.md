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

## 发布结果

- PR #202 已合并，标签指向 `fb36e273f30650ce8e3099ed86669679fe9153f5`。
- Linux Release `34512499775`、Docker Multi-arch `34512499817` 均成功。
- 镜像：`ghcr.io/moeacgx/maolaonewapi:v1.0.0-rc.10.1.10.320`。
- 多架构摘要：`sha256:a5ceec8d7e6ce57dd5f6894bab567b2078f145aeeb9c2f8c84b126e0e08d3b7f`。
- amd64 层：`sha256:ea1a2c8eaaa8cfeed3f4d687a0ca5193ce347e337fd8b697f63235d25c654525`。

## 滚动更新证据

- 备份与拉取作业：`6423d3ae-d033-44ed-a7a3-b03f2723772b`。
- Compose 备份：`/home/docker/maolaoapi/docker-compose.yml.bak-v1.0.0-rc.10.1.10.320`，
  SHA-256 与更新前一致：`b669a392c295f1f22f64aa6c09c4a1c2e370f4817d6fc05cafdb5cd7cb17318f`。
- 更新后 Compose SHA-256：`e0f0eee5f0b6200c73a73a760e53b3fdc4ad6223f85d1721c6d3df7fd22e0ca9`。
- `maolaoapi-slave-1` 作业：`c7ce9fe4-48c9-4ec0-9b3b-255819550f6e`，新容器 `6000f304f99e`。
- `maolaoapi-slave-2` 作业：`2cb572a6-c0bf-406c-8f13-b708f02000a2`，新容器 `2687713ce0f6`。
- `maolaoapi` 作业：`080cb7bb-33e7-4351-8d94-e4c1aff115ff`，新容器 `1cd873687c99`。
- 三个应用均为 `running/healthy`，重启次数 0，镜像 `.320`。
- 本地端口 `18100`、`18101`、`18095` 的 `/api/status` 均返回 `v1.0.0-rc.10.1.10.320`。
- 公网 `https://maolaoapi.com/api/status` 返回同一版本。
- `maolaoapi-postgres`（`bee76d26e45d`）与 `maolaoapi-redis`（`4b5b4ae42b6f`）
  容器 ID 未变，均 `running`、重启次数 0。
- 未操作 zzapi，未重建数据库或缓存。
