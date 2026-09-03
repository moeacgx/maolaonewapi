# MaoLaoNewAPI 最新基线逐一对比清单

## 锚点

- 旧版本锚点：`.243 = 2b538c8e`，`.244 = d182efad`
- 当前最新基线：`origin/custom-main@4d65dd4bdda4c3529b92c0af44890963d41dbe1d`（PR #140 合并后）
- 近期合并参考：`#140` `#139` `#138` `#137` `#133` `#132` `#131` `#130` `#129` `#128` `#127` `#126` `#124` `#123` `#122` `#121` `#120`
- 本轮运行验证：`2026-08-31`
- 配套文件：
  - `30_maolaonewapi_latest_baseline_review_dispatch_guide.md`
  - `30_maolaonewapi_latest_baseline_compare_plan.md`
  - `30_maolaonewapi_v243_v244_merge_conflict_audit.md`

## Issue 进度

- [x] 文档 PR：`#137`
- [x] `#134` `文档：最新基线复核分发指引`
- [x] `#136` `文档：最新基线逐一对比计划`
- [x] `#135` `文档：最新基线逐一对比清单`

## 1. 认证会话

主题状态：`已覆盖`

- [x] 原子准入 / 并发上限
  - 结论：`已覆盖`
  - 证据：`model/user_session.go`、`service/auth_session.go`、`service/auth_session_test.go`
  - 说明：主路径已切到后端自动准入与最旧会话淘汰，不再只靠前端恢复提示。
- [x] 撤销缓存失败兜底
  - 结论：`已覆盖`
  - 证据：`model/user_session.go`、`controller/auth_session.go`、`controller/auth_session_test.go`
  - 说明：缓存失败不应阻断数据库撤销，当前基线已保留该兜底。
- [x] Classic 登录上限恢复
  - 结论：`已覆盖`
  - 证据：`web/classic/src/classic-auth-session-compat.test.mjs`、Playwright Chrome 409/429 运行验证
  - 说明：409 和 429 均只发送一次登录 POST、只显示一个稳定错误码提示；未出现 Axios 通用错误或重复的“登录失败，请重试”。普通活跃会话满员仍以后端自动淘汰为主路径。
- [x] 旧 Classic 兼容对照
  - 结论：`已覆盖`
  - 证据：`18_classic_auth_session_compat.md`
  - 说明：它更像历史对照，不是当前主修法。

## 2. 支付 / 充值 / 返佣 / 发布

主题状态：`部分覆盖`

- [x] 充值审计快照
  - 结论：`已覆盖`
  - 证据：`model/topup.go`、`model/topup_audit_test.go`
  - 说明：余额前后快照、已充额度、审计字段已经回到同一事务内。
- [x] 返佣幂等键
  - 结论：`已覆盖`
  - 证据：`model/affiliate.go`、`model/affiliate_migration_test.go`、`model/affiliate_transaction_test.go`
  - 说明：`ON CONFLICT` 和唯一索引已收敛到同一套四字段契约。
- [x] Model Plaza 卡片计费展示
  - 结论：`已覆盖`
  - 证据：`web/classic/src/components/table/model-pricing/view/card/PricingCardView.jsx`、`web/classic/src/components/table/model-pricing/view/card/__tests__/card-layout-contract.test.mjs`
  - 说明：Playwright 已验证动态、按次、按秒、按量四种标签；桌面、459px、460px 的计费标签与性能摘要底边差为 0，亮暗主题均无页面级横向溢出。
- [ ] 订阅 / 发票链路
  - 结论：`部分覆盖`
  - 证据：`controller/subscription.go`、`model/subscription.go`、`model/invoice.go` 及对应 controller/model 运行测试
  - [x] SQLite 真事务与 Gin `httptest` 已覆盖支付预览、余额购买、重复回调、provider guard、发票创建/状态同步、回滚、优惠码与缓存刷新失败。
  - [ ] MySQL/PostgreSQL 真实行锁、外部支付网关网络回调、完整 Router/反代、多实例 Redis 和浏览器支付回跳尚未验证。
  - 说明：这条链路和充值审计不是同一症状；本地运行验证通过的部分已标记，但不足以把完整链路判为已覆盖。
- [x] Linux-only release
  - 结论：`已覆盖`
  - 证据：`.github/workflows/release.yml`、`29_linux_only_release_workflow.md`
  - 说明：只作为 release 侧兜底证据，不拿它直接当产品 bug。

## 3. Channel / 用户 / 权限

主题状态：`已覆盖`

- [x] 分组部分更新
  - 结论：`已覆盖`
  - 证据：`controller/channel.go`、`model/channel.go`、`controller/channel_concurrency_test.go`
  - 说明：`group` / `group_ids` 省略时会保留原绑定，不再误清空。
- [x] 并发字段恢复
  - 结论：`已覆盖`
  - 证据：`controller/channel.go`、`model/channel.go`、`model/channel_concurrency.go`
  - 说明：`ConcurrencyLimit` 已恢复持久化、回读和选择器参与。
- [x] 管理侧限流隔离
  - 结论：`已覆盖`
  - 证据：`middleware/rate-limit.go`、`middleware/rate_limit_test.go`、`router/channel-router.go`
  - 说明：`isChannelManagementWrite` 已与已注册管理路由对齐；Admin/Root 管理写操作绕过 GA，`POST /api/channel/:id/key`、读接口、未知路径和普通/匿名/无效凭证继续受 GA 保护。矩阵测试和 Redis/IP 限流测试均通过。
- [x] 审计日志过滤
  - 结论：`已覆盖`
  - 证据：`model/log.go`
  - 说明：`TestRecordOperationAuditLogSuppressesChannelUpdates` 已验证 `channel.update` 不落库、`channel.create` 仍落库；消费统计路径未改。
- [x] 用户搜索契约
  - 结论：`已覆盖`
  - 证据：`controller/user.go`、`model/user.go`、`controller/user_search_test.go`
  - 说明：`search_type=id` 已恢复为独立 ID 精确匹配。

## 4. Classic / UI / 路由

主题状态：`已覆盖`

- [x] 控制台页面平铺
  - 结论：`已覆盖`
  - 证据：`web/classic/src/pages/Dashboard/__tests__/flat-layout-contract.test.mjs`、`web/classic/src/pages/Dashboard/__tests__/console-shell-contract.test.mjs`
  - 说明：这是纯布局收敛，已形成稳定契约。
- [x] 页面级视觉收敛
  - 结论：`已覆盖`
  - 证据：`web/classic/src/index.css`、`web/classic/src/pages/Dashboard/__tests__/flat-layout-contract.test.mjs`
  - 说明：页面级 Card 和外壳已分层处理，不再混在一起。
- [x] 通知筛选恢复
  - 结论：`已覆盖`
  - 证据：`web/classic/src/pages/NotificationCenter/index.jsx`、`web/classic/src/pages/NotificationCenter/__tests__/filter-config.test.mjs`
  - 说明：Playwright 已验证已有筛选回显；`channel_disabled` 保存 PUT 会归一化状态码和关键词，非 `channel_disabled` 的保存 payload 不含 `filter_config`。
- [x] 移动端弹窗可用性
  - 结论：`已覆盖`
  - 证据：`web/classic/src/pages/NotificationCenter/index.jsx`、`web/classic/src/pages/NotificationCenter/__tests__/modal-viewport.test.mjs`
  - 说明：修复后真实本地模拟登录态页面在 1280/414/375/320px 下 Modal 均保持视口内，长 Chat ID、名称、模板和事件变量可断行，正文层可滚动，footer 确认按钮可达且页面无横向溢出。
- [x] 模型详情分组 / 标签收敛
  - 结论：`已覆盖`
  - 证据：`web/classic/src/components/table/model-pricing/modal/components/ModelBasicInfo.jsx`、`web/classic/src/index.css`
  - 说明：Playwright 已确认 Classic 详情只显示灰色分组 pill 且不显示模型 tags；Default 与 Classic 仍是两条独立实现路径。
- [x] Default 计费折扣 / 供应商收敛
  - 结论：`已覆盖`
  - 证据：`29_default_model_details_discount_supplier_cleanup.md`
  - 说明：这是同一语义在 Default 侧的收敛，不要和 Classic 详情展示混成一条。

## 5. 运行验证结果

- 总计：`已覆盖 19 / 部分覆盖 1 / 未覆盖 0 / 需运行验证 0`
- [x] `Classic 登录上限恢复`
- [x] `Model Plaza 卡片计费展示`
- [x] `审计日志过滤`
- [x] `通知筛选恢复`
- [x] `模型详情分组 / 标签收敛`
- [x] `管理侧限流隔离`
- [x] `移动端弹窗可用性`
- [x] `订阅 / 发票` 本地 SQLite/Gin 运行验证
- [ ] `订阅 / 发票` 多数据库、外部网关、完整路由和浏览器回跳

确定性命令：

```text
go test ./controller -run 'Test.*(Subscription|Invoice)' -count=1 -timeout=60s
go test ./model -run 'Test.*(Subscription|Invoice)' -count=1 -timeout=60s
go test ./middleware -run 'TestChannelAdminBypassCoversManagementWritesButProtectsKeyRead|TestChannelAdminBypassRejectsUnprivilegedCredentials|TestChannelAdminBypassIsScopedToAuthenticatedWrites|TestRedis.*RateLimiter|TestGlobalWebRateLimit' -count=1 -timeout=60s
node --test web/classic/src/classic-auth-session-compat.test.mjs web/classic/src/pages/__tests__/page-card-flat-contract.test.mjs web/classic/src/pages/Dashboard/__tests__/flat-layout-contract.test.mjs web/classic/src/pages/Dashboard/__tests__/console-shell-contract.test.mjs web/classic/src/pages/NotificationCenter/__tests__/filter-config.test.mjs web/classic/src/pages/NotificationCenter/__tests__/modal-viewport.test.mjs web/classic/src/components/table/model-pricing/view/card/__tests__/card-layout-contract.test.mjs web/classic/src/components/table/model-pricing/model-pricing-visual-contract.test.mjs web/classic/src/components/table/model-pricing/groupVisuals.test.mjs
npm run build  # web/classic，16972 modules transformed
Playwright Chrome  # 登录、模型广场、通知筛选、通知 Modal 视口
```

## 6. 直接对比顺序

1. 先看 `30_maolaonewapi_latest_baseline_compare_plan.md`
2. 再按本清单逐项勾选
3. 对 `部分覆盖` 和 `未覆盖` 项按上节剩余边界继续验证或修复
