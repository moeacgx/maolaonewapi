# NewAPI 负载均衡部署流程记录

## 目标

记录 zzapi 多节点负载均衡部署后可复用的检查流程，后续给 maolaoapi 做负载均衡时按
同一顺序执行。本文只记录流程和配置项名称，不记录主机密码、token、数据库连接串或真实
生产端点。

## 安全边界

- zzapi 是测试目标；maolaoapi 是生产目标，变更前必须再次确认目标主机、仓库、分支、
  Compose 文件、容器名和回滚镜像。
- 不在未确认的情况下重启、替换或迁移 maolaoapi 生产容器。
- 仓库 Actions 因私库限制出现 `steps: []` 时，可以在用户确认后临时公开仓库，等
  Release 和 Docker workflow 成功后立即改回 private。
- 所有生产凭据只从受控来源读取，不能写入文档、PR、Issue 或日志。

## 发布前准备

1. 确认目标版本已经合入 `custom-main`，并记录 merge commit。
2. 创建 release 分支，只修改 `VERSION`，例如：
   `release/v1.0.0-rc.10.1.10.xxx`。
3. 提交 release PR，合并后把版本 tag 打到 release PR 的 merge commit。
4. 推送 tag 后检查两条发布工作流：
   - `Release (Linux, macOS, Windows)`
   - `Publish Docker image (Multi-arch)`
5. 如果 workflow 立刻失败且 job 为 `steps: []`：
   - 确认仓库当前是 private。
   - 经用户确认后临时改 public。
   - 重跑失败的 tag workflow。
   - 等二进制 Release 和 GHCR 多架构镜像都成功。
   - 立即改回 private，并再次确认仓库可见性。

## 应用节点配置

所有应用节点必须共享同一套控制面状态：

- `SQL_DSN`：所有节点连接同一个主数据库。
- `SESSION_SECRET`：所有节点必须一致，否则登录态、刷新会话和临时鉴权无法一致校验。
- `REDIS_CONN_STRING`：建议所有节点共享同一个 Redis，避免限流、缓存和鉴权状态分裂。
- `CRYPTO_SECRET`：共享 Redis 时必须一致；未显式设置时要确认实际回退策略符合预期。
- `FRONTEND_BASE_URL`：指向负载均衡后的统一外部访问地址。
- `NODE_NAME`：每个节点必须唯一且稳定，例如按区域、机器和项目命名。
- `NODE_TYPE`：只保留一个 master 节点；工作节点设置为 `slave`。

注意：代码中 `NODE_TYPE != slave` 会被视为 master。因此新增 worker 时必须显式写
`NODE_TYPE=slave`，不能依赖默认值。

## 负载均衡接入顺序

1. 先选定当前单节点作为 master，不改数据库和业务配置。
2. 新增第一个 worker 节点，使用同版本镜像、同数据库、同 Redis、同密钥。
3. 给 worker 配置独立 `NODE_NAME` 和 `NODE_TYPE=slave`。
4. 先只启动 worker，不立刻加入公网流量。
5. 在 worker 本机或内网访问 `/api/status`，确认进程健康、版本正确。
6. 在 Classic 后台打开 `/console/setting?tab=performance`，查看“多节点实例”：
   - master 和 worker 均应显示在线。
   - 版本应一致。
   - 角色应分别为 master / worker。
   - 最近上报时间应在 90 秒内刷新。
7. 将 worker 加入负载均衡 upstream，先小流量观察。
8. 验证登录、渠道请求、计费日志、任务状态、后台设置页和静态资源均正常。
9. 按同样方式逐个加入后续 worker，不并行替换所有节点。

## 负载均衡器检查点

- 健康检查建议使用轻量 HTTP 路径，例如 `/api/status`。
- 代理必须保留真实客户端 IP 相关头部，避免日志和风控只看到负载均衡器地址。
- WebSocket、SSE 或流式响应必须关闭不合理的缓冲，保持长连接可用。
- 超时时间要覆盖长耗时流式请求，不能只按普通短请求设置。
- 只暴露负载均衡入口到公网；单个应用节点端口优先限制在内网或本机监听。

## 数据与后台任务边界

- 数据库迁移、日志清理、订阅重置、鉴权同步等后台任务只能由 master 执行。
- worker 只承接 Web/API 流量，不运行 master-only 后台任务。
- `system_instances` 表会保留历史节点心跳。失联节点超过 90 秒会显示 stale，
  需要在后台确认后删除。
- 如果从其他项目克隆数据库，旧项目节点也会出现在多节点实例列表中，删除 stale 记录即可。

## 发版产物确认

发布成功后至少确认：

- GitHub Release 存在目标 tag，且 Linux、macOS、Windows 资产已上传。
- GHCR 目标镜像存在，例如：
  `ghcr.io/moeacgx/maolaonewapi:v1.0.0-rc.10.1.10.xxx`
- 多架构 manifest 包含 `linux/amd64` 和 `linux/arm64`。
- `latest` manifest 如有更新，确认 digest 与目标版本来源一致。

## 线上更新步骤

1. 记录当前运行镜像、容器 ID、版本和 compose 文件路径。
2. 备份当前 compose 文件和关键环境变量文件。
3. 拉取目标版本镜像。
4. 先更新一个非入口或低风险 worker 节点。
5. 健康检查通过后，将流量切到该节点或恢复其 upstream 权重。
6. 逐个更新其他 worker。
7. 最后更新 master，确保更新期间至少有一个健康节点承接流量。
8. 更新完成后验证：
   - `/api/status` 版本。
   - Classic `/console/setting?tab=performance` 多节点状态。
   - 业务请求、计费日志和后台页面。
   - 容器最近日志无启动错误和循环重启。

## 回滚步骤

1. 负载均衡器先摘除异常节点。
2. 使用记录的上一版镜像恢复该节点。
3. 验证 `/api/status`、后台页面和业务请求。
4. 节点恢复后再决定是否加入 upstream。
5. 如果是 master 异常，先确认没有数据库迁移或后台任务进行中，再回滚 master。

## maolaoapi 执行前清单

- 再次确认 maolaoapi 的目标主机、当前容器名、当前镜像 tag、compose 路径和监听端口。
- 确认不会误操作 zzapi 或其他测试容器。
- 确认数据库、Redis、`SESSION_SECRET`、`CRYPTO_SECRET` 的共享方式。
- 规划唯一 `NODE_NAME`，并明确哪个节点是 master。
- 准备上一版镜像和 compose 备份作为回滚点。
- 先用一个 worker 节点验证完整链路，再扩容到多节点。
