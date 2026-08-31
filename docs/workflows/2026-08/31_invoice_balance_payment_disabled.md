# 发票中心关闭余额支付

## 目标

关闭发票中心申请发票时的账户余额支付，仅保留管理员配置且当前可用的外部支付方式；
系统充值、订阅和其他余额消费流程不受影响。

## 行为

- `GET /api/user/invoice/config` 不再返回 `balance` 支付方式。
- Default 与 Classic 发票中心不展示余额支付选项，正服务费默认/必须选择外部支付。
- `POST /api/user/invoice/request` 仅允许零服务费申请；当服务费大于 0 时返回
  “发票服务费不支持余额支付，请选择其他支付方式”。
- `POST /api/user/invoice/payment` 拒绝 `payment_method=balance`，避免绕过页面时得到
  过时的余额支付指引。
- `POST /api/user/invoice/payment` 继续处理易支付微信/支付宝等外部支付。
- 零服务费仍可提交申请，因为没有实际余额扣款。

## 范围边界

本次仅影响发票中心服务费支付，不修改普通充值、订阅购买、用户余额结算或其他支付入口。
Default 和 Classic 两套模板分别保留同一后端边界。

## 验证

- `go test ./controller ./model ./service -run "Invoice|Notification" -count=1 -timeout 60s`
- `node --test web/classic/src/pages/Invoice/__tests__/epay-payment.test.mjs web/classic/src/invoice-batch-request.test.mjs`
- Default Vitest、typecheck、lint 和构建需要 Bun；本机未安装 Bun，无法执行。
- `git diff --check`

## zzapi 验证结果（2026-08-31）

- 目标为 CloudSSH 项目“API中转站”的测试服务器 `serverId=52`，Compose 目录为
  `/home/docker/zzapi`；未操作 `maolaoapi`。
- 从提交 `b35c99922` 在测试机本地构建镜像
  `ghcr.io/moeacgx/maolaonewapi:zzapi-pr141-b35c99922-local`，避免改变正式发布流程。
- 更新前创建备份 `docker-compose.yml.bak-invoice-balance-20260831-b35c99922`，随后按
  `zzapi-slave-1`、`zzapi-slave-2`、`zzapi` 顺序执行
  `docker compose up -d --no-deps --force-recreate`。
- 三个应用容器均使用新镜像并为 `running/healthy`，重启计数为 0；PostgreSQL 与 Redis
  未重建，仍为 `running`。
- 本机端口 `18097`、`18098`、`18099` 的 `/api/status` 均返回版本
  `zzapi-pr141-b35c99922`。三笔演示订单仍存在，其中两笔为 `success|invoice_required=false`，
  另一笔已被现场测试创建为 `payment_pending` 发票申请。
- Docker Hub 分支镜像工作流因未配置 Docker Hub 凭据失败，未产生镜像；不影响上述本地构建
  和 zzapi 部署结果。
