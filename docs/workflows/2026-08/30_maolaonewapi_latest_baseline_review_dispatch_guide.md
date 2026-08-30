# MaoLaoNewAPI 最新基线复核分发指引

## 目标

- 把 `.244` 时代的旧差异，和当前 `custom-main` 最新基线、最近 merged PR、对应
  `docs/workflows` 文档三方对齐。
- 识别“同一症状但修法不同”的项，避免把旧结论直接套到新基线。
- 产出可勾选的审计清单，后续可以直接排除、复核或补运行验证。
- 本文只规定**分发与复核方法**，不写实现修复。

## 当前基线

- 当前已校准的最新基线：
  `origin/custom-main@90020e4c02c10ad187796015d132262a085536f4`
- 旧摘要里记录的 `62fe0247` 已不是最新头，分发前必须刷新。
- 复核时要同时看：
  - 最新 `custom-main` 头
  - 最近 merged PR
  - 相关 workflow 文档
  - 对应代码路径

## 关联 Issue

- 文档 PR：`#137`
- `#134` `文档：最新基线复核分发指引`
- `#136` `文档：最新基线逐一对比计划`
- `#135` `文档：最新基线逐一对比清单`

## 核心原则

- `docs/workflows` 是入口，不是唯一来源。
- 不允许只扫全目录后拼结论。
- 每个主题包只读 2-5 篇同主题 md，必要时再补一个兜底包。
- 同一症状若当前基线换了修法，状态要标 `部分覆盖`，不能沿用旧结论。
- 静态证据不够的，一律标 `需运行验证`。

## 最新基线参考

优先把最近 merged PR 当成“当前修法参考”，再回看 workflow 文档里的旧修法。

- #133 `feat: 展示多节点实例 RPM 和当前并发`
- #131 `fix: unify channel concurrency across instances`
- #130 `fix: localize async image task rejection errors`
- #129 `fix: localize channel concurrency limit error`
- #128 `fix: preserve billing reservation across empty retries`
- #127 `fix(classic): unify console header and repair promo code 404`
- #126 `[codex] fix Classic login audit log display`
- #124 `fix(auth): auto-evict oldest active session on login`
- #123 `fix: restore OKX Alipay rate contracts`
- #122 `修复 Classic 控制台页面级卡片平铺`
- #121 `fix classic model detail group tags`
- #120 `align default pricing discount badges`

## 阅读顺序

1. 先读旧审计：
   `docs/workflows/2026-08/30_maolaonewapi_v243_v244_merge_conflict_audit.md`
2. 再读本主题包里的 workflow md，每个 agent 只读自己的 2-5 篇。
3. 再读最新基线对应的 merged PR / commit。
4. 最后只看对应代码路径，用 `git show` / `git diff` / `rg` 验证。

## 分发包

### 1) 认证会话组

建议读取：

- `docs/workflows/2026-08/29_auth_session_atomic_admission.md`
- `docs/workflows/2026-08/29_auth_session_revoke_cache_failure.md`
- `docs/workflows/2026-08/29_classic_login_session_limit_recovery.md`
- `docs/workflows/2026-08/18_classic_auth_session_compat.md`

重点看：

- 登录态是否会重复入账或重复写入
- 撤销、刷新、并发登录是否仍有竞态
- Classic 登录限制与新基线是否同症状不同实现

### 2) 支付 / 充值 / 订阅组

建议读取：

- `docs/workflows/2026-08/29_topup_balance_audit.md`
- `docs/workflows/2026-08/29_model_plaza_card_billing_layout.md`
- `docs/workflows/2026-08/29_affiliate_record_conflict_target.md`
- `docs/workflows/2026-08/29_linux_only_release_workflow.md`

重点看：

- 充值、结算、回跳、退款、余额预扣
- 发票 / 订阅 / 优惠 / 返佣是否和最新基线一致
- `29_linux_only_release_workflow.md` 作为 release 侧交叉证据，不单独当产品问题结论

### 3) Channel / 用户 / 权限组

建议读取：

- `docs/workflows/2026-08/29_channel_group_binding_partial_update.md`
- `docs/workflows/2026-08/29_channel_admin_rate_limit_isolation.md`
- `docs/workflows/2026-08/29_channel_concurrency_backend_restore.md`
- `docs/workflows/2026-08/29_channel_update_audit_log_filter.md`
- `docs/workflows/2026-08/29_user_search_type_contract.md`

重点看：

- Channel 绑定、并发、限流、审计日志过滤
- 用户搜索类型约束是否被最新基线改写
- 同一症状是否已经在新基线里用不同路径修过

### 4) Classic / UI / 路由组

建议读取：

- `docs/workflows/2026-08/29_classic_console_flat_layout.md`
- `docs/workflows/2026-08/30_classic_console_flat_pages_visual.md`
- `docs/workflows/2026-08/29_classic_channel_notification_filter_restore.md`
- `docs/workflows/2026-08/29_classic_notification_mobile_filters.md`
- `docs/workflows/2026-08/30_classic_model_details_group_tags_cleanup.md`
- `docs/workflows/2026-08/29_default_model_details_discount_supplier_cleanup.md`

重点看：

- Classic 视觉与布局是否只是样式迁移，还是路由 / 行为也变了
- 通知筛选、移动端过滤是否和当前基线保持一致
- 默认模型详情、折扣、供应商展示是否被新修法覆盖

### 4.1) 已回收的细分结论

根据已回来的主题审查，这几个大包还能再拆一层，后续派发时优先按下面的小包走：

- 认证会话包再拆成：
  - 会话原子准入 / 并发上限
  - 撤销缓存失败兜底
  - Classic 登录上限恢复
  - 旧 Classic 兼容对照
- 支付 / 充值包再拆成：
  - 充值事务完整性
  - 返佣幂等键一致性
  - Classic 模型广场卡片计费展示
  - Linux-only release 兜底证据
- Channel / 用户包再拆成：
  - 分组绑定部分更新
  - 并发字段恢复
  - 管理侧限流隔离
  - 审计日志噪音过滤
  - 用户搜索契约独立包
- Classic / UI 包再拆成：
  - 控制台页面平铺与视觉收敛
  - 通知筛选恢复与移动端可用性
  - 模型详情分组 / 标签收敛

### 4.2) 当前回收状态

- Classic 控制台页面平铺与视觉收敛：`已覆盖`
- Classic 通知筛选恢复与移动端可用性：`需运行验证`
- 模型详情分组 / 标签收敛：`部分覆盖`
- 支付 / 充值 / 返佣：主体已覆盖，但展示层和 release 兜底仍要单独看
- Channel / 用户 / 权限：写路径主体要单独勾选，管理侧边界与审计噪音适合放辅助证据

### 4.3) 代码级锚点

后续分发时，优先看这些文件，而不是只看 workflow 文档：

- Channel 写路径
  - `controller/channel.go`
  - `model/channel.go`
  - `controller/channel_concurrency_test.go`
  - `model/channel_concurrency.go`
  - `model/channel_migration_test.go`
- 认证会话
  - `model/user_session.go`
  - `service/auth_session.go`
  - `controller/auth_session.go`
  - `middleware/auth.go`
  - `middleware/auth_origin.go`
  - `common/session_cookie.go`
  - `service/auth_session_test.go`
  - `controller/auth_session_test.go`
  - `web/classic/src/classic-auth-session-compat.test.mjs`
- 支付 / 充值 / 返佣 / 发布
  - `model/topup.go`
  - `model/topup_audit_test.go`
  - `model/affiliate.go`
  - `model/affiliate_migration_test.go`
  - `model/affiliate_transaction_test.go`
  - `controller/subscription.go`
  - `web/classic/src/components/topup/RechargeCard.jsx`
  - `web/classic/src/components/topup/SubscriptionPlansCard.jsx`
  - `web/classic/src/components/topup/modals/SubscriptionPurchaseModal.jsx`
  - `web/classic/src/components/topup/card-layout-contract.test.mjs`
  - `.github/workflows/release.yml`
- 用户搜索契约
  - `controller/user.go`
  - `model/user.go`
  - `controller/user_search_test.go`
- Classic 通知任务
  - `web/classic/src/pages/NotificationCenter/index.jsx`
  - `web/classic/src/pages/NotificationCenter/__tests__/filter-config.test.mjs`
  - `web/classic/src/pages/NotificationCenter/__tests__/modal-viewport.test.mjs`
  - `web/classic/src/index.css`
- Classic 模型广场卡片与详情
  - `web/classic/src/components/table/model-pricing/view/card/PricingCardView.jsx`
  - `web/classic/src/components/table/model-pricing/view/card/__tests__/card-layout-contract.test.mjs`
  - `web/classic/src/components/table/model-pricing/modal/components/ModelBasicInfo.jsx`
  - `web/classic/src/index.css`
- Classic 控制台外壳
  - `web/classic/src/pages/Dashboard/__tests__/flat-layout-contract.test.mjs`
  - `web/classic/src/pages/Dashboard/__tests__/console-shell-contract.test.mjs`

### 4.4) 读代码时的判断点

- `controller/channel.go` 里的 `groupProvided` / `groupIDsProvided` 和 `model/channel.go` 里的 `groupSelectionProvided`，共同决定“部分更新是否保留原绑定”。
- `controller/channel_concurrency_test.go` 和 `model/channel_concurrency.go` 说明并发上限是独立能力，不要和分组绑定混成一个问题。
- `controller/user.go` + `model/user.go` + `controller/user_search_test.go` 说明 `search_type=id` 是独立查询契约，不属于 Channel 主题。
- `NotificationCenter` 的 `filter_config` 只在 `channel_disabled` 事件里保存，移动端弹窗问题主要是 CSS 作用域和尺寸。
- `PricingCardView.jsx` 和 `ModelBasicInfo.jsx` 分别管卡片页脚和详情页，不能把它们当同一个视觉问题。
- `web/classic/src/index.css` 里 `classic-pricing-model-card-footer-info`、`classic-pricing-detail-pill`、`classic-notification-task-modal` 是三条独立收口线。

### 5) 兜底组

如果 29/30 没有找到后继，再回看：

- `docs/workflows/2026-08/28_*`
- `docs/workflows/2026-08/18_*`

只按同关键词回看，不做全目录扫。

## 每个 agent 的固定输出

请统一返回这些字段：

- `topic`
- `workflow_docs`
- `latest_pr_or_commit`
- `old_fix`
- `current_fix`
- `same_issue_diff_fix`
- `status`
- `evidence`
- `checklist_items`
- `notes`

### 状态枚举

- `已覆盖`
- `部分覆盖`
- `未覆盖`
- `需运行验证`

## 判定规则

- 只有“workflow 有旧问题 + 最新 PR / commit 有明确后继 + 当前代码一致”，才可标 `已覆盖`。
- 同一症状但修法不同，标 `部分覆盖`，不要沿用旧结论。
- 没有后继或代码不一致的，标 `未覆盖`。
- 静态证据不够的，统一标 `需运行验证`。

## checklist 建议字段

每条 checklist 统一带这些信息，方便后续勾选：

`compare_axis / base_ref / head_ref / merge_commit / tag_sha / file / symbol / base_line / head_line / diff_status / invariant / call_chain / evidence_command / verdict`

### 建议 verdict

- `NORMAL`
- `MERGE_RESOLUTION`
- `REGRESSION`
- `NEEDS_RUNTIME`

## 证据命令

建议优先使用这些命令：

```bash
git fetch origin
git rev-parse origin/custom-main
git log --merges --oneline -n 12 origin/custom-main
git show --stat <sha>
git diff <base>..<head> -- <path>
rg -n "<symbol-or-keyword>" <path>
```

## 派发策略

- 每个 agent 只处理一个主题包。
- 如果同一主题包超过 5 篇文档，拆成两个 agent。
- agent 只输出路径、符号、行为变化、证据命令和结论，不写实现。
- 先做静态归因，再做 runtime 复核候选列表。

## 交付要求

- 先给总统计，再给全量路径索引，再给高风险清单。
- 高风险清单优先列：
  - `web/default` 残留
  - 主题切换
  - 支付回跳
  - Relay / 计费
  - Canvas
  - 权限边界
  - CI / 文档 / 删除项
- 每一项都要能落到文件、符号或测试名。

## 结论口径

- 本次复核不是“再做一次旧审计”，而是确认旧修法在最新基线里是否还成立。
- 如果最新修法和旧修法不同，以当前基线为准。
- 如果静态材料不够，宁可标 `需运行验证`，不要硬判。
