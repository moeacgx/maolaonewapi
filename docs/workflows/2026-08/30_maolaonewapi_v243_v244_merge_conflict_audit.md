# MaoLaoNewAPI 243/244 合流冲突审计

## 概要

- 对比范围：`v1.0.0-rc.10.1.10.243` (`2b538c8e98a14004b8948b6e5fb7482cfeb32941`) 与 `v1.0.0-rc.10.1.10.244` (`d182efadc57279807eeb8807bbc66b886e2458d5`)。
- 语义锚点：`.244` 与 `PR #43` 的 merge commit 一致，且 `d182efadc` 的树与 `739ac468ec7ee1bfc6695b1c0719462eecde960c` 完全一致；因此主语义来源按 `PR 轴` 记录，`merge 轴` 仅作结果确认。
- 产物目标：把每条变化标成 `NORMAL / MERGE_RESOLUTION / REGRESSION / NEEDS_RUNTIME`，方便后续逐条勾选排除。

## 统计

- 总变化：`A 646 / D 655 / M 628 / R080+ 393`。
- 顶层目录集中度：`web 1546`，其次为 `relay 242`、`model 145`、`service 144`、`controller 142`、`docs 106`。
- 结构性结论：`web/default` 被整体移除，`web/src` 作为新的 Default 前端根目录接收，`web/classic` 保持存在并继续修改。
- 后端主线变化集中在认证会话、支付/发票/订阅、Canvas、relay、channel authz、扩展代理与主题兼容。

## 高风险合流点

### 1. 前端拓扑迁移

- `web/default`：1072 项删除。
- `web/src`：1193 项新增。
- `web/classic`：520 项保留修改。
- 结论：这是完整布局迁移，不是单点文件重命名；检查重点应放在旧路径是否还有外部引用、旧 console 路由是否被正确映射、深链是否被 SPA fallback 吞掉。

### 2. 认证与会话

- 关键文件：`service/auth_session.go`、`middleware/auth.go`、`middleware/auth_origin.go`、`common/session_cookie.go`。
- 关注点：refresh 轮换、并发刷新、撤销、OriginGuard、Secure cookie、旧会话失效。

### 3. 支付 / 发票 / 充值 / 订阅

- 关键文件：`controller/topup.go`、`controller/invoice.go`、`controller/subscription.go`。
- 关注点：回跳路径、签名、幂等、`lockForUpdate`、余额/订阅/发票状态同步。

### 4. Relay / Canvas / 扩展代理

- 关键文件：`controller/relay.go`、`relay/common/relay_utils.go`、`controller/canvas_image_task.go`、`controller/canvas_proxy.go`、`extension/proxy.go`。
- 关注点：retry、selected route、stream started、task duration、SSRF、路径穿越、双计费。

## Checklist

### 版本轴
- [ ] V-01 | PR 轴 | `0f8d391...739ac468` | 证明 `.244` 的语义来自 PR head，而非 merge resolution | `git diff 0f8d391..739ac468`
- [ ] V-02 | release 轴 | `2b538c8e..d182efadc` | 证明 `.243 -> .244` 的发布差异规模与实际树变化一致 | `git diff 2b538c8e..d182efadc`
- [ ] V-03 | merge 轴 | `d182efadc^1 / d182efadc^2` | 证明 merge commit 没有额外独立 resolution 树差异 | `git diff d182efadc^1 d182efadc` / `git diff d182efadc^2 d182efadc`

### 前端拓扑
- [ ] F-01 | `web/default` -> `web/src` | Default 前端整体上移后，旧入口仍有对应新路由 | `web/src/routes/*`, `web/src/lib/legacy-route.ts`
- [ ] F-02 | `web/classic` | Classic 路由与支付页仍保留 | `web/classic/src/App.jsx`, `web/classic/src/pages/Canvas/index.jsx`
- [ ] F-03 | SPA fallback | `/api/*`、`/v1/*`、`/assets` 不被前端 HTML 吞掉 | `router/web-router.go`
- [ ] F-04 | 旧 console 映射 | `/console/topup`、`/console/invoice`、`/console/log`、`/console/personal` 映射一致 | `common/constants.go`, `controller/return_path.go`

### 认证 / 会话
- [ ] A-01 | session refresh | refresh 轮换不产生重放窗口 | `service/auth_session.go`
- [ ] A-02 | auth middleware | dashboard JWT 不应误落到 PAT/relay token | `middleware/auth.go`
- [ ] A-03 | origin guard | 生产反代下 `OriginGuard` 和 secure cookie 配置一致 | `middleware/auth_origin.go`, `common/session_cookie.go`

### 支付 / 发票 / 订阅
- [ ] P-01 | topup flow | 充值回调、幂等、余额上限一致 | `controller/topup.go`
- [ ] P-02 | invoice flow | 发票支付回跳与状态落库一致 | `controller/invoice.go`
- [ ] P-03 | subscription flow | 订阅购买 / 扣费 / 回跳一致 | `controller/subscription.go`

### Relay / Canvas / 扩展
- [ ] R-01 | relay retry | retry 不应导致双写或双计费 | `controller/relay.go`
- [ ] R-02 | relay task | 任务型 relay 的 duration / seconds 上限受控 | `relay/common/relay_utils.go`
- [ ] R-03 | canvas | 异步图片任务与回退路径的计费边界正确 | `controller/canvas_image_task.go`, `controller/canvas_proxy.go`
- [ ] R-04 | extension proxy | SSRF / header 隔离 / 路径穿越受控 | `extension/proxy.go`

### 文档 / 构建 / 删除项
- [ ] D-01 | docs/workflows | 删除的工作流文档都能找到后继代码或测试锚点 | `docs/workflows/*`
- [ ] D-02 | .github/workflows | 发布链路、版本注入、Docker 产物一致 | `.github/workflows/*`, `VERSION`
- [ ] D-03 | 旧 skill / test 删除 | 被删测试或 skill 是否已有 successor | `.agents/skills/*`, `*_test.go`

## 证据锚点

- `.243`：`2b538c8e98a14004b8948b6e5fb7482cfeb32941`
- `.244`：`d182efadc57279807eeb8807bbc66b886e2458d5`
- PR #43 head：`739ac468ec7ee1bfc6695b1c0719462eecde960c`
- PR #43 base：`0f8d3911786f5f9787cef702740768c0a1f3ec75`
- `web/default` 删除量：1072
- `web/src` 新增量：1193
- `web/classic` 保留修改：520

## 结论

- `.244` 不是简单补丁集，而是一次大规模重组合并。
- 最需要逐条勾选排除的是前端拓扑、认证会话、支付回跳、relay/Canvas 计费边界，以及被删除的 docs/skills/tests 是否有 successor。
- 静态证据不足的项统一标 `NEEDS_RUNTIME`，不要提前判无问题。
