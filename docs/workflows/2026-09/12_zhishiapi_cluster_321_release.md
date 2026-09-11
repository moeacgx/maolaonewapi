# zhishiapi 三节点负载均衡与 .321 发布

## 目标与范围

把同机 `zhishiapi.com` 从单应用扩成三个同等应用容器，入口改 Nginx
upstream 分流，并把应用镜像从 bootstrap `.243` 更新到
`v1.0.0-rc.10.1.10.321`。

- 主机：CloudSSH“API中转站” `serverId=38`、`hostId=11`，目录 `/home/docker/zhishiapi`。
- 应用：`zhishiapi`（18096）、`zhishiapi-slave-1`（18097）、`zhishiapi-slave-2`（18098）。
- 共享原有 `zhishiapi-postgres`、`zhishiapi-redis`，不重建数据卷。
- 入口：`zhishiapi.com` Nginx `upstream zhishiapi_backend`。
- 不更新 maolaoapi 三应用，不更新 zzapi。

## 更新前现场

- 仅 `zhishiapi` 一个应用容器，镜像 `zhishiapi/app:bootstrap-20260828`，
  `/api/status` 为 `v1.0.0-rc.10.1.10.243`，反代 `127.0.0.1:18096`。
- Postgres `2b78970cf1f4`、Redis `304611873f68`，均为 healthy。

## 发布结果

- 应用镜像：`ghcr.io/moeacgx/maolaonewapi:v1.0.0-rc.10.1.10.321`。
- Compose 备份：`/home/docker/zhishiapi/docker-compose.yml.bak-cluster-20260911-211024`。
- Nginx 备份：`/www/server/panel/vhost/nginx/zhishiapi.com.conf.bak-cluster-20260911-211024`。
- `zhishiapi` 作业：`0b9f0815-0e6e-4365-9969-0c4664e7c78d`，容器 `5e585d059993`。
- `zhishiapi-slave-1` 作业：`0831e6d6-28c3-40d3-94a0-11131aeaaa8c`，容器 `fead56fe721a`。
- `zhishiapi-slave-2` 作业：`77a0715c-498a-4583-913b-534664c64142`，容器 `dbd74f6f7111`。
- Nginx 切换作业：`1f45c1c2-8bbd-4e22-aa24-9b8f3523dcf2`。
- 三个应用 `running/healthy`，重启次数 0；Postgres/Redis 容器 ID 未变。
- `18096/18097/18098` 与本机 HTTPS Host `zhishiapi.com` 12 次请求均返回 `.321`。
- 公网 `https://zhishiapi.com/api/status` 返回 `v1.0.0-rc.10.1.10.321` / `ZhiShiAPI`。
- 同机 maolaoapi `18095` 仍为 `.321`。
