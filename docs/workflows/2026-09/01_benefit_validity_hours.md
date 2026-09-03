# 福利活动个人券有效期改用小时

## 目标

个人券有效期从管理端的秒单位改为小时单位，降低活动配置门槛，同时保持已有活动的
失效时间和数据库数据不变。

## 变更范围

- 管理端创建/编辑请求使用 `personal_valid_hours`；旧的 `personal_valid_seconds` 仅作为
  兼容输入。
- 管理端和用户端活动响应增加按小时返回的 `personal_valid_hours`；模型数据库字段
  `personal_valid_seconds` 保留，计费与券领取仍以秒计算。
- Default 和 Classic 活动表单使用小时输入，默认 24 小时；提交时不再发送秒字段。
- 小时值使用 decimal 解析，换算后必须是完整秒，并限制在 `int64` 可表示范围内。

## 兼容性与安全边界

- 已有数据库记录无需迁移，读取时按 `personal_valid_seconds / 3600` 回显。
- 新请求优先使用 `personal_valid_hours`；仅在新字段缺省时读取旧秒字段。
- 小于等于 0、无法换算为完整秒或换算溢出的有效期请求会被拒绝。
- 活动结束时间仍是硬失效点，实际券失效时间继续取个人有效期与活动结束时间的较小值。

## 验证计划

- `go test ./controller ./model`
- Default 福利表单相关测试、类型检查和受影响文件 lint。
- Classic 福利契约测试、格式检查和生产构建。
- `git diff --check`

## 实际验证

- `go test ./controller ./model`：通过。
- Classic 福利契约测试 8/8：通过；受影响文件 Prettier 检查：通过；Classic 生产构建：通过。
- Default 受影响文件使用 `oxfmt --check`：通过；本机未安装 Bun 且没有
  `web/node_modules`，因此 Default Vitest 和 typecheck 未执行。
- `git diff --check`：通过。

## zzapi 部署

- 目标：CloudSSH 项目“API中转站”的 `serverId=52`，Compose 目录
  `/home/docker/zzapi`；未操作 `maolaoapi`。
- 远端构建镜像：
  `ghcr.io/moeacgx/maolaonewapi:zzapi-benefit-validity-hours-20260901`，镜像摘要
  `sha256:821938da9c731d385c5511b9a34876e49ae05760bdee381a4504dbc90206e501`。
- 更新前备份 Compose：`docker-compose.yml.bak-benefit-validity-hours-20260901`。
- 按 `zzapi-slave-1`、`zzapi-slave-2`、`zzapi` 顺序逐个重建应用容器；PostgreSQL 和
  Redis 未重建，三个应用容器最终均为 `running/healthy`，重启计数为 `0`。
- 本地端口 `18097`、`18098`、`18099` 和公网
  `https://zzapi.maolaoapi.com/api/status` 均返回 `"success":true`。
- `/api/status` 业务版本为仓库 `VERSION` 的 `v1.0.0-rc.10.1.10.287`；容器实际运行
  镜像均为上述新镜像摘要。
