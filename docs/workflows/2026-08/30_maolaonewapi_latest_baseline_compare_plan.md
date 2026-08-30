# MaoLaoNewAPI 最新基线逐一对比计划

## 目标

- 用同一套顺序，把 `.244` 旧问题与当前 `origin/custom-main@90020e4c02c10ad187796015d132262a085536f4` 逐项对齐。
- 先分主题，再分子项，最后合并成可勾选结论。
- 任何“同症状但修法不同”的项，都单独标出来，不混写。

## 关联 Issue

- `#136` `文档：最新基线逐一对比计划`
- `#134` `文档：最新基线复核分发指引`
- `#135` `文档：最新基线逐一对比清单`

## 对比顺序

1. 认证会话
2. 支付 / 充值 / 返佣 / 发布
3. Channel / 用户 / 权限
4. Classic / UI / 路由

## 逐项计划

### 1) 认证会话

- 主题文档：
  - `29_auth_session_atomic_admission.md`
  - `29_auth_session_revoke_cache_failure.md`
  - `29_classic_login_session_limit_recovery.md`
  - `18_classic_auth_session_compat.md`
- 当前判断：
  - 主体：`部分覆盖`
  - 原子准入、撤销缓存失败：已被后端新修法接住
  - Classic 登录上限恢复：仍要保留运行验证
- 逐一核对：
  - 并发登录是否还会突破会话上限
  - revoke 缓存失败是否会阻断数据库撤销
  - `AUTH_SESSION_LIMIT` 的恢复路径是否仍会触发
  - Classic 兼容是否只是旧对照，不再是主修法

### 2) 支付 / 充值 / 返佣 / 发布

- 主题文档：
  - `29_topup_balance_audit.md`
  - `29_affiliate_record_conflict_target.md`
  - `29_model_plaza_card_billing_layout.md`
  - `29_linux_only_release_workflow.md`
- 当前判断：
  - 充值 / 返佣：主体已覆盖
  - Classic 模型广场卡片：`需运行验证`
  - release 侧：只作为兜底证据，不单独当产品 bug
- 逐一核对：
  - 充值审计快照是否完整
  - 返佣 `ON CONFLICT` 和唯一索引是否完全一致
  - Classic 卡片仅展示层改动，还是计费语义也变了
  - Linux-only release 是否只影响构建链路

### 3) Channel / 用户 / 权限

- 主题文档：
  - `29_channel_group_binding_partial_update.md`
  - `29_channel_concurrency_backend_restore.md`
  - `29_channel_admin_rate_limit_isolation.md`
  - `29_channel_update_audit_log_filter.md`
  - `29_user_search_type_contract.md`
- 当前判断：
  - 分组部分更新、并发字段恢复：`已覆盖`
  - 管理侧限流隔离、审计日志过滤：`需运行验证`
  - 用户搜索契约：独立包，已覆盖
- 逐一核对：
  - `group` / `group_ids` 部分更新是否保留原绑定
  - `ConcurrencyLimit` 是否持久化、回读、参与选择
  - 管理写接口是否仍被普通限流桶误伤
  - `channel.update` 是否仍被精确过滤
  - `search_type=id` 是否只做 ID 精确匹配

### 4) Classic / UI / 路由

- 主题文档：
  - `29_classic_console_flat_layout.md`
  - `30_classic_console_flat_pages_visual.md`
  - `29_classic_channel_notification_filter_restore.md`
  - `29_classic_notification_mobile_filters.md`
  - `30_classic_model_details_group_tags_cleanup.md`
  - `29_default_model_details_discount_supplier_cleanup.md`
- 当前判断：
  - Classic 控制台页面平铺：`已覆盖`
  - 通知筛选恢复与移动端弹窗：`需运行验证`
  - 模型详情分组 / 标签收敛：`部分覆盖`
- 逐一核对：
  - 控制台外壳是否只改布局，不误伤业务层
  - 通知任务的 `filter_config` 是否只在 `channel_disabled` 保存
  - 移动端弹窗是否真可用
  - 模型详情与卡片页脚是否是两条独立收口线
  - Default 和 Classic 是否只是同一语义的不同前端面

## 统一输出模板

每个子项都按同一格式写：

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

## 结论用法

- `已覆盖`：旧问题有后继，当前代码一致。
- `部分覆盖`：同症状但修法不同，或只有一部分被新基线接住。
- `未覆盖`：当前基线没有对应后继。
- `需运行验证`：静态证据不够，必须跑测试或浏览器验证。
