# MaoLaoNewAPI 最新基线逐一对比清单

## 锚点

- 旧版本锚点：`.243 = 2b538c8e`，`.244 = d182efad`
- 当前最新基线：`origin/custom-main@90020e4c02c10ad187796015d132262a085536f4`
- 近期合并参考：`#133` `#131` `#130` `#129` `#128` `#127` `#126` `#124` `#123` `#122` `#121` `#120`
- 配套文件：
  - `30_maolaonewapi_latest_baseline_review_dispatch_guide.md`
  - `30_maolaonewapi_latest_baseline_compare_plan.md`
  - `30_maolaonewapi_v243_v244_merge_conflict_audit.md`

## Issue 进度

- [x] `#134` `文档：最新基线复核分发指引`
- [x] `#136` `文档：最新基线逐一对比计划`
- [x] `#135` `文档：最新基线逐一对比清单`

## 1. 认证会话

主题状态：`部分覆盖`

- [x] 原子准入 / 并发上限
  - 结论：`已覆盖`
  - 证据：`model/user_session.go`、`service/auth_session.go`、`service/auth_session_test.go`
  - 说明：主路径已切到后端自动准入与最旧会话淘汰，不再只靠前端恢复提示。
- [x] 撤销缓存失败兜底
  - 结论：`已覆盖`
  - 证据：`model/user_session.go`、`controller/auth_session.go`、`controller/auth_session_test.go`
  - 说明：缓存失败不应阻断数据库撤销，当前基线已保留该兜底。
- [ ] Classic 登录上限恢复
  - 结论：`需运行验证`
  - 证据：`web/classic/src/classic-auth-session-compat.test.mjs`
  - 说明：静态上能看到恢复链路，但真实 409 体验仍要跑一次。
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
- [ ] Model Plaza 卡片计费展示
  - 结论：`需运行验证`
  - 证据：`web/classic/src/components/table/model-pricing/view/card/PricingCardView.jsx`、`web/classic/src/components/table/model-pricing/view/card/__tests__/card-layout-contract.test.mjs`
  - 说明：目前更像展示层收敛，像素和交互还要跑验证。
- [ ] 订阅 / 发票链路
  - 结论：`需运行验证`
  - 证据：`controller/subscription.go`
  - 说明：这条链路和充值审计不是同一症状，不能和前两项混成一条。
- [x] Linux-only release
  - 结论：`已覆盖`
  - 证据：`.github/workflows/release.yml`、`29_linux_only_release_workflow.md`
  - 说明：只作为 release 侧兜底证据，不拿它直接当产品 bug。

## 3. Channel / 用户 / 权限

主题状态：`部分覆盖`

- [x] 分组部分更新
  - 结论：`已覆盖`
  - 证据：`controller/channel.go`、`model/channel.go`、`controller/channel_concurrency_test.go`
  - 说明：`group` / `group_ids` 省略时会保留原绑定，不再误清空。
- [x] 并发字段恢复
  - 结论：`已覆盖`
  - 证据：`controller/channel.go`、`model/channel.go`、`model/channel_concurrency.go`
  - 说明：`ConcurrencyLimit` 已恢复持久化、回读和选择器参与。
- [ ] 管理侧限流隔离
  - 结论：`需运行验证`
  - 证据：`middleware/rate-limit.go`、`controller/channel.go`
  - 说明：静态能看到限流框架和管理员写路径，但还要看真实请求是否真的隔离。
- [ ] 审计日志过滤
  - 结论：`需运行验证`
  - 证据：`model/log.go`
  - 说明：`channel.update` 已做精确过滤，但需要再确认没有副作用。
- [x] 用户搜索契约
  - 结论：`已覆盖`
  - 证据：`controller/user.go`、`model/user.go`、`controller/user_search_test.go`
  - 说明：`search_type=id` 已恢复为独立 ID 精确匹配。

## 4. Classic / UI / 路由

主题状态：`部分覆盖`

- [x] 控制台页面平铺
  - 结论：`已覆盖`
  - 证据：`web/classic/src/pages/Dashboard/__tests__/flat-layout-contract.test.mjs`、`web/classic/src/pages/Dashboard/__tests__/console-shell-contract.test.mjs`
  - 说明：这是纯布局收敛，已形成稳定契约。
- [x] 页面级视觉收敛
  - 结论：`已覆盖`
  - 证据：`web/classic/src/index.css`、`web/classic/src/pages/Dashboard/__tests__/flat-layout-contract.test.mjs`
  - 说明：页面级 Card 和外壳已分层处理，不再混在一起。
- [ ] 通知筛选恢复
  - 结论：`需运行验证`
  - 证据：`web/classic/src/pages/NotificationCenter/index.jsx`、`web/classic/src/pages/NotificationCenter/__tests__/filter-config.test.mjs`
  - 说明：`channel_disabled` 的筛选保存逻辑已静态可见，但要看真实编辑流。
- [ ] 移动端弹窗可用性
  - 结论：`需运行验证`
  - 证据：`web/classic/src/pages/NotificationCenter/index.jsx`、`web/classic/src/pages/NotificationCenter/__tests__/modal-viewport.test.mjs`
  - 说明：CSS 已收口，但 375px 之类视口仍建议跑一次。
- [ ] 模型详情分组 / 标签收敛
  - 结论：`部分覆盖`
  - 证据：`web/classic/src/components/table/model-pricing/modal/components/ModelBasicInfo.jsx`、`web/classic/src/index.css`
  - 说明：Default 和 Classic 不是同一层问题，当前更像是两边语义收敛。
- [x] Default 计费折扣 / 供应商收敛
  - 结论：`已覆盖`
  - 证据：`29_default_model_details_discount_supplier_cleanup.md`
  - 说明：这是同一语义在 Default 侧的收敛，不要和 Classic 详情展示混成一条。

## 5. 需要优先运行验证的项

- `Classic 登录上限恢复`
- `Model Plaza 卡片计费展示`
- `订阅 / 发票链路`
- `管理侧限流隔离`
- `审计日志过滤`
- `通知筛选恢复`
- `移动端弹窗可用性`

## 6. 直接对比顺序

1. 先看 `30_maolaonewapi_latest_baseline_compare_plan.md`
2. 再按本清单逐项勾选
3. 最后只对 `需运行验证` 的项跑测试或浏览器
