# 可信代理 IP 多实例登记与真实 IP 排障

## 目标

记录 v1.0.0-rc.10.1.10.244 以后真实客户端 IP 解析的配置契约，并建立新增反代入口和网关实例时的登记、同步与验证流程。

## 实时核验结果

核验日期：2026-08-30。

- CloudSSH server `43`（入口 IP `152.53.239.32`）运行 Caddy，为 `api.maolaoapi.com` 设置 `X-Real-IP` 和 `X-Forwarded-For`，并转发至 `159.195.12.233`。
- CloudSSH server `38` 的生产 Compose 项目位于 `/home/docker/maolaoapi`，包含 `maolaoapi`、`maolaoapi-slave-1` 和 `maolaoapi-slave-2` 三个网关实例。
- 主容器配置了包含 `152.53.239.32` 的 `TRUSTED_PROXIES`。
- 两个 slave 没有 `TRUSTED_PROXIES` 环境变量，并且实时日志显示它们正在承载请求且把客户端记录成 `152.53.239.32`。
- Compose 渲染配置中的 `TRUSTED_PROXIES` 只有一处，说明环境配置没有同步到所有网关服务。

## 根因

244 版本在启动时显式调用 `ConfigureTrustedProxies`。空配置只信任回环、RFC1918 和 IPv6 ULA，公网 Caddy 地址不会被信任。243 版本没有这层显式配置，Gin 默认转发头行为使问题没有暴露。

本次现象不是客户端没有发送真实 IP，也不是 `ClientIP()` 在所有实例中失效，而是部分实际承载请求的实例缺少同一组可信代理配置。

## 文档交付

- 新增 [反代 IP 登记与多实例接入手册](../../developer/trusted-proxy-instance-registration.md)，登记当前 `152.53.239.32` 并提供新增入口模板。
- 更新 [authentication.md](../../authentication.md)，明确所有主实例和 slave/worker 必须保持配置一致。
- 更新 [开发文档入口](../../developer/README.md)，让后续开发和部署工作能找到该手册。

## 生产变更执行结果

- 在 `/home/docker/maolaoapi/docker-compose.yml` 中为两个 slave 补齐与主容器一致的 `TRUSTED_PROXIES`。
- 在修改前创建备份：`docker-compose.yml.bak-real-ip-20260830`。
- 按顺序分别执行 `docker compose up -d --no-deps --force-recreate`，先重建 `maolaoapi-slave-1`，验收通过后再重建 `maolaoapi-slave-2`。
- 未执行 `pull` 或 `build`，未重建主容器、数据库或 Redis，未修改反代配置。
- 两个 slave 和主容器最终均为 `running/healthy`；Compose 渲染配置中共有三项 `TRUSTED_PROXIES`。
- Compose 与备份相比只新增两个 slave 的 `TRUSTED_PROXIES` 配置行。

## 发布目标

- 云端最新基线：`origin/custom-main`，已在本地 `custom-main` 合并本次文档提交。
- 发布版本：`v1.0.0-rc.10.1.10.281`。
- 发布方式：推送 `custom-main` 后推送同名 Git 标签，由 `.github/workflows/release.yml` 触发 Linux Release。
- 发布范围：本次可信代理登记手册、排障记录和认证配置说明；不包含生产服务器 Compose 文件。

## 后续验证

- `go test ./middleware -run TrustedProxies`：已通过，覆盖默认、严格直连、显式 IP/CIDR 和非法配置行为。
- 文档变更后执行 `git diff --check`，并检查新增链接和示例命令。
- 生产修复后已逐容器检查 `TRUSTED_PROXIES`；两个 slave 重建后的 10 分钟日志中，`152.53.239.32` 出现次数均为 `0`，并出现正常公网客户端地址。
- 未主动构造生产 API 测试请求，使用重建后实际到达两个 slave 的真实流量日志完成验证。
