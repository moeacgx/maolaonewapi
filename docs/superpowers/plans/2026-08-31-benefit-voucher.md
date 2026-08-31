# Benefit Voucher Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 Issue #77 定义的绑定稳定分组的一次性时效额度券，包括活动、领取、组合计费、分组单用户并发、双前端和审计报表。

**Architecture:** 使用四张独立福利表保存活动、预拆份额、用户券和流水；在现有 `BillingSession` 内引入组合资金计划，命中福利券时按福利券、订阅、钱包顺序预扣和结算。分组并发在最终分组确定后按 `user_id + group_id` 建立进程内请求租约，Default 与 Classic 通过同一组 `/api/benefit` 接口提供管理和用户体验。

**Tech Stack:** Go 1.22、Gin、GORM v2、testify、SQLite/MySQL/PostgreSQL；React 19、TypeScript、TanStack Router、React Query、Base UI/Tailwind；Classic React/Semi UI；Bun/Vitest/node:test。

**Spec:** `docs/plans/2026-08-31-benefit-voucher-design.md`

## Global Constraints

- 只修改当前分支的代码、测试和开发文档；不部署、不修改生产环境、不创建 PR。
- 所有 JSON 编解码调用使用 `common` 包装器；业务代码不得直接调用 `encoding/json`。
- 数据库同时支持 SQLite、MySQL 5.7.8+ 和 PostgreSQL 9.6+；标准行锁使用 `lockForUpdate(tx)`。
- 金额输入只接受整数分语义，quota 使用 `int64` 存储；请求级 quota 转换继续使用 `common/quota_math.go`。
- 新增或重写的 Go 测试使用 `require` 和 `assert`，单个后台测试命令超时不超过 60 秒。
- Default 与 Classic 都在范围内，所有用户可见文案支持 i18n。
- 每个实现步骤先运行新增测试看到预期失败，再写最小生产代码并重新运行至通过。

---

### Task 1: 福利数据模型、迁移与分组字段

**Files:**
- Create: `model/benefit_voucher.go`
- Create: `model/benefit_voucher_test.go`
- Modify: `model/main.go`
- Modify: `model/group_identity.go`
- Modify: `model/group_identity_test.go`

**Interfaces:**
- Produces: `BenefitActivity`、`BenefitActivityShare`、`BenefitUserVoucher`、`BenefitVoucherLedger` GORM 模型。
- Produces: `SplitBenefitShares(input BenefitShareSplitInput) ([]BenefitShareAllocation, error)`。
- Produces: `Group.SingleUserConcurrencyLimit int` 和同名 `GroupConfig` JSON 字段。

- [ ] **Step 1: 写金额拆分、模型索引和分组字段失败测试**

```go
func TestSplitBenefitSharesPreservesBudgetAndBounds(t *testing.T) {
    shares, err := SplitBenefitShares(BenefitShareSplitInput{
        Mode: BenefitAmountModeRandom, TotalAmountCents: 1000,
        TotalCount: 4, MinAmountCents: 100, MaxAmountCents: 700,
        RandomIntn: func(max int) int { return max / 2 },
    })
    require.NoError(t, err)
    require.Len(t, shares, 4)
    var total int64
    for _, share := range shares {
        assert.GreaterOrEqual(t, share.AmountCents, int64(100))
        assert.LessOrEqual(t, share.AmountCents, int64(700))
        total += share.AmountCents
    }
    assert.Equal(t, int64(1000), total)
}

func TestGroupConfigPersistsSingleUserConcurrencyLimit(t *testing.T) {
    // 使用现有 SQLite group fixture 保存值 3，再读取 / ToConfig 断言仍为 3。
}
```

- [ ] **Step 2: 运行测试并确认因类型/字段不存在而失败**

Run: `go test ./model -run 'TestSplitBenefitShares|TestGroupConfigPersistsSingleUserConcurrencyLimit' -count=1 -timeout 60s`

- [ ] **Step 3: 实现模型和确定性随机拆分**

```go
type BenefitShareSplitInput struct {
    Mode string
    TotalAmountCents int64
    TotalCount int
    FixedAmountCents int64
    MinAmountCents int64
    MaxAmountCents int64
    RandomIntn func(max int) int
}

type BenefitShareAllocation struct {
    AmountCents int64
    Quota int64
}
```

固定模式验证 `fixed * count == total`；随机模式先给每份 `min`，再按每份剩余容量随机分配剩余整数分，最后洗牌。`Quota` 使用活动发布时传入的显示金额到 quota 换算结果，不在拆分器内读取动态全局配置。

- [ ] **Step 4: 把四张表和分组字段注册到主迁移**

在 `migrateDB()` 与分模型迁移列表中加入四个模型；不给 `SingleUserConcurrencyLimit` 添加布尔式默认 tag，创建/保存时把负值拒绝并把缺失值归一为 `0`。

- [ ] **Step 5: 运行模型测试、格式化和提交**

Run: `gofmt -w model/benefit_voucher.go model/benefit_voucher_test.go model/group_identity.go model/group_identity_test.go model/main.go`

Run: `go test ./model -run 'Benefit|GroupConfig' -count=1 -timeout 60s`

Commit: `feat: add benefit voucher data model`

### Task 2: 活动状态机、发布与报表模型

**Files:**
- Modify: `model/benefit_voucher.go`
- Modify: `model/benefit_voucher_test.go`

**Interfaces:**
- Consumes: Task 1 四个模型和 `SplitBenefitShares`。
- Produces: `CreateBenefitActivity`、`UpdateBenefitActivityDraft`、`PublishBenefitActivity`、`TransitionBenefitActivity`、`TerminateBenefitActivity`。
- Produces: `GetBenefitActivityReport(activityID int, now int64) (*BenefitActivityReport, error)`。

- [ ] **Step 1: 写发布冻结、时间重叠和两种终止模式失败测试**

```go
func TestPublishBenefitActivityCreatesExactSharesAndRejectsOverlap(t *testing.T) {
    // 创建同分组时间相交的两个草稿；第一个发布生成 N 份且总额精确，第二个发布返回 ErrBenefitActivityOverlap。
}

func TestTerminateUnusedKeepsClaimedVoucherActive(t *testing.T) {
    // 终止未用券后 available share 变 voided，已领取券余额和 active 状态不变。
}

func TestTerminateAllVoidsRemainingVoucherBalance(t *testing.T) {
    // 终止全部券后 remaining=0、used 保持、写入 void ledger。
}
```

- [ ] **Step 2: 运行新增测试并确认因状态机函数不存在而失败**

Run: `go test ./model -run 'TestPublishBenefitActivity|TestTerminate' -count=1 -timeout 60s`

- [ ] **Step 3: 用 GORM 事务实现状态机**

状态常量固定为 `draft/published/paused/ended/terminated`；发布锁定活动和同分组候选活动，验证 `[starts_at, ends_at)` 区间不相交后创建份额。`TransitionBenefitActivity` 只允许 `published -> paused`、`paused -> published`、`published|paused -> ended`，并在恢复时重复冲突检查。

- [ ] **Step 4: 实现预算报表聚合**

```go
type BenefitActivityReport struct {
    TotalQuota int64 `json:"total_quota"`
    UndistributedQuota int64 `json:"undistributed_quota"`
    DistributedQuota int64 `json:"distributed_quota"`
    UsedQuota int64 `json:"used_quota"`
    ExpiredUnusedQuota int64 `json:"expired_unused_quota"`
}
```

报表从份额和券余额聚合，不从使用日志反推；先执行活动/券惰性过期，再读取终态。

- [ ] **Step 5: 运行测试并提交**

Run: `go test ./model -run 'BenefitActivity|BenefitReport|Terminate' -count=1 -timeout 60s`

Commit: `feat: add benefit activity lifecycle`

### Task 3: 用户资格、领取和券查询

**Files:**
- Modify: `model/benefit_voucher.go`
- Modify: `model/benefit_voucher_test.go`
- Create: `controller/benefit.go`
- Create: `controller/benefit_test.go`
- Modify: `router/api-router.go`

**Interfaces:**
- Produces: `GetBenefitClaimEligibility(userID int, activity *BenefitActivity, now int64) (BenefitClaimEligibility, error)`。
- Produces: `ClaimBenefitActivity(activityID, userID int, now int64) (*BenefitUserVoucher, error)`。
- Produces user routes `GET /api/benefit/activities`、`GET /api/benefit/vouchers`、`POST /api/benefit/activities/:id/claim`。

- [ ] **Step 1: 写真实实付、注册 30 分钟、单用户单券和最后一份竞争失败测试**

```go
func TestBenefitEligibilityUsesSuccessfulPaidSnapshotsWithoutDoubleCountingSubscription(t *testing.T) {
    // 成功 topup 的 paid_amount_cny 计入，pending/赠送不计；同 trade_no 的订阅映射只计一次。
}

func TestClaimBenefitActivityReturnsAlreadyClaimedAndSoldOutContracts(t *testing.T) {
    // 重复领取返回 ErrBenefitAlreadyClaimed；最后一份被占用后另一个用户返回 ErrBenefitSoldOut。
}
```

- [ ] **Step 2: 运行模型测试确认失败**

Run: `go test ./model -run 'BenefitEligibility|ClaimBenefit' -count=1 -timeout 60s`

- [ ] **Step 3: 实现资格和事务领取**

只汇总 `top_ups.status=success` 且 `paid_amount_cny > 0` 的真实快照。领取事务锁定用户、活动和一个 available 份额，先检查现有券，再用条件更新把份额变为 claimed，并创建唯一用户券；唯一冲突映射为 `ErrBenefitAlreadyClaimed`。

- [ ] **Step 4: 写并实现用户接口行为测试**

```go
func TestClaimBenefitActivityHidesEligibilityReason(t *testing.T) {
    // 注册不足或实付不足都断言 HTTP 200 success=false message="不符合领取条件"。
}
```

活动列表返回 `claimable/claimed/history` 分类、`eligible`、`claimed_voucher` 和并发上限；券列表返回原始、已用、剩余、状态、分组快照和失效时间。

- [ ] **Step 5: 运行控制器测试并提交**

Run: `go test ./model ./controller -run 'Benefit' -count=1 -timeout 60s`

Commit: `feat: add benefit voucher claiming`

### Task 4: 管理端 API 与操作审计

**Files:**
- Modify: `controller/benefit.go`
- Modify: `controller/benefit_test.go`
- Modify: `router/api-router.go`

**Interfaces:**
- Produces design spec 中全部 `/api/benefit/admin/*` 路由。
- Consumes: Task 2 生命周期及报表函数、现有 `recordManageAudit`。

- [ ] **Step 1: 写管理权限、字段冻结、强确认和审计失败测试**

```go
func TestBenefitTerminateRequiresConfirmationAndReason(t *testing.T) {
    // confirm=false 或 reason 为空均失败且数据库无变化；合法请求记录 benefit.activity.terminate 审计。
}
```

- [ ] **Step 2: 运行测试确认路由/handler 缺失**

Run: `go test ./controller ./router -run 'Benefit' -count=1 -timeout 60s`

- [ ] **Step 3: 实现 DTO 校验和管理路由**

所有金额字段使用 `int64 *_amount_cents`，所有时间使用 Unix 秒。创建/草稿更新验证名称、分组存在且启用、正数预算/份数/有效期、`starts_at < ends_at`；发布后普通 PUT 只复制 name/description。强终止请求为：

```go
type BenefitTerminateRequest struct {
    Mode string `json:"mode"`
    Confirm bool `json:"confirm"`
    Reason string `json:"reason"`
}
```

- [ ] **Step 4: 为每个管理变化写结构化审计**

动作名使用 `benefit.activity.create/update/publish/pause/resume/end/terminate` 和 `benefit.voucher.void`；params 只含 ID、模式和原因，不写用户凭证或券内部流水 metadata。

- [ ] **Step 5: 运行测试并提交**

Run: `go test ./controller ./router -run 'Benefit' -count=1 -timeout 60s`

Commit: `feat: add benefit voucher administration`

### Task 5: 分组单用户并发

**Files:**
- Create: `model/group_user_concurrency.go`
- Create: `model/group_user_concurrency_test.go`
- Modify: `middleware/distributor.go`
- Create: `middleware/group_user_concurrency_test.go`
- Modify: `controller/group.go`

**Interfaces:**
- Produces: `TryAcquireGroupUserConcurrency(userID, groupID, limit int) (*GroupUserConcurrencyLease, bool)` 和 `Release()`。
- Produces: Context 所有权键 `group_user_concurrency_lease`。

- [ ] **Step 1: 写 key 隔离、上限 0、精确释放和流式生命周期失败测试**

```go
func TestGroupUserConcurrencyIsolatedByUserAndGroup(t *testing.T) {
    lease, ok := TryAcquireGroupUserConcurrency(7, 11, 1)
    require.True(t, ok)
    defer lease.Release()
    _, same := TryAcquireGroupUserConcurrency(7, 11, 1)
    _, otherGroup := TryAcquireGroupUserConcurrency(7, 12, 1)
    _, otherUser := TryAcquireGroupUserConcurrency(8, 11, 1)
    assert.False(t, same)
    assert.True(t, otherGroup)
    assert.True(t, otherUser)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./model ./middleware -run 'GroupUserConcurrency' -count=1 -timeout 60s`

- [ ] **Step 3: 实现本地租约并接入 Distribute**

在 `SetupContextForSelectedChannel` 成功、最终 `SelectedChannelGroup` 已确定后解析稳定 Group.Id 并占用；失败时在 `c.Next()` 前返回 HTTP 429。释放与渠道并发一样由 Context 所有权控制，所有退出路径幂等。

- [ ] **Step 4: 区分福利和普通分组文案**

通过 `model.HasBenefitActivityForGroup(groupID, now)` 判断福利文案；两者状态码都为 429，福利文案精确为“福利分组限制，你请求太快啦！”。

- [ ] **Step 5: 运行测试并提交**

Run: `go test ./model ./middleware ./controller -run 'GroupUserConcurrency|GroupConfig' -count=1 -timeout 60s`

Commit: `feat: limit concurrency per user and group`

### Task 6: 福利券资金源和组合计费

**Files:**
- Modify: `model/benefit_voucher.go`
- Modify: `model/benefit_voucher_test.go`
- Modify: `service/funding_source.go`
- Modify: `service/billing_session.go`
- Modify: `service/billing_session_test.go`
- Modify: `service/billing.go`
- Modify: `relay/common/relay_info.go`

**Interfaces:**
- Produces: `BenefitVoucherFunding` 和 `CompositeFunding`，均实现 `FundingSource`。
- Produces: `BillingBreakdown`，字段为 `VoucherQuota`、`SubscriptionQuota`、`WalletQuota`、`ActivityID`、`VoucherID`。
- Produces: `BillingSession.GetBreakdown() relaycommon.BillingBreakdown`。

- [ ] **Step 1: 写四种扣费组合和失败逆序回滚测试**

```go
func TestCompositeFundingUsesVoucherSubscriptionWalletOrder(t *testing.T) {
    // 预扣 100：券 30、订阅 40、钱包 30；断言调用顺序、breakdown 和每个余额。
}

func TestCompositeFundingRollsBackVoucherWhenWalletFails(t *testing.T) {
    // 券和订阅成功、钱包不足；断言券/订阅恢复且 ledger 有 refund。
}

func TestCompositeFundingSettlesRefundInReversePriority(t *testing.T) {
    // 预扣 100、实际 55：钱包 30 全退、订阅退 15、券保持 30。
}
```

- [ ] **Step 2: 运行服务测试确认失败**

Run: `go test ./service -run 'CompositeFunding|BenefitBilling' -count=1 -timeout 60s`

- [ ] **Step 3: 实现券原子预扣和幂等流水**

```go
func ReserveBenefitVoucherQuota(requestID string, userID, groupID int, amount int64, now int64) (*BenefitVoucherReservation, error)
func SettleBenefitVoucherQuota(requestID string, actual int64) error
func RefundBenefitVoucherQuota(requestID string) error
```

三函数使用数据库事务、行锁和 request id 流水保护；只选一张最早失效的 active 券，不叠加多券。

- [ ] **Step 4: 把组合资金计划接入 NewBillingSession**

命中显式稳定分组和可用券时，先建立 `CompositeFunding`；剩余额度固定尝试订阅再钱包，不读取用户计费偏好。未命中券时保留原 `subscription_only/wallet_only/wallet_first/subscription_first` 分支不变。token quota 仍按总 `preConsumedQuota` 一次处理。

- [ ] **Step 5: 同步 RelayInfo 并运行回归**

Run: `go test ./service ./model -run 'BillingSession|Funding|Benefit' -count=1 -timeout 60s`

Commit: `feat: bill requests with benefit vouchers`

### Task 7: 消费日志、券流水关联与收入边界

**Files:**
- Modify: `model/log.go`
- Modify: `model/log_test.go`
- Modify: `service/log_info_generate.go`
- Modify: `service/text_quota.go`
- Modify: `service/quota.go`
- Create: `service/benefit_log_test.go`
- Modify: `web/src/features/usage-logs/types.ts`
- Modify: `web/src/features/usage-logs/lib/billing-details.ts`
- Modify: `web/src/features/usage-logs/lib/billing-details.test.ts`

**Interfaces:**
- Changes: `RecordConsumeLog(...) int` 返回成功写入的日志 ID，失败返回 0；现有调用可忽略返回值。
- Produces log JSON `other.billing_breakdown`。

- [ ] **Step 1: 写日志 ID、拆分字段和流水回填失败测试**

```go
func TestBenefitBillingLogLinksLedgerAndPreservesTotalQuota(t *testing.T) {
    // 结算 100，breakdown 30/40/30；消费日志 quota 仍为 100，other 含拆分，ledger.log_id 等于日志 ID。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./model ./service -run 'BenefitBillingLog|RecordConsumeLog' -count=1 -timeout 60s`

- [ ] **Step 3: 返回日志 ID 并注入拆分**

`createLog(log)` 成功后返回 `log.Id`；`Generate*OtherInfo` 通过 `relayInfo.Billing` 的 breakdown 写入稳定结构。日志创建成功后调用 `LinkBenefitLedgerLogID(requestID, logID)`。

- [ ] **Step 4: 保护报表和收入边界**

消费 quota、`QuotaData` 和渠道 used quota 继续使用总实际费用；收入统计不新增福利表 join，只继续读取成功支付订单的 actual money，因此福利券不会进入现金收入。

- [ ] **Step 5: 运行后端和 Default 纯逻辑测试并提交**

Run: `go test ./model ./service -run 'Benefit|RecordConsumeLog|Revenue' -count=1 -timeout 60s`

Run: `cd web; bun run test -- src/features/usage-logs/lib/billing-details.test.ts`

Commit: `feat: expose benefit billing breakdown`

### Task 8: Default 管理端和用户端

**Files:**
- Create: `web/src/features/benefits/api.ts`
- Create: `web/src/features/benefits/types.ts`
- Create: `web/src/features/benefits/index.tsx`
- Create: `web/src/features/benefits/components/benefit-activities-panel.tsx`
- Create: `web/src/features/benefits/components/benefit-activity-form.tsx`
- Create: `web/src/features/benefits/components/benefit-terminate-dialog.tsx`
- Create: `web/src/features/benefits/components/__tests__/activity-form.test.tsx`
- Create: `web/src/features/benefits/components/__tests__/terminate-dialog.test.tsx`
- Create: `web/src/routes/_authenticated/benefits/index.tsx`
- Modify: `web/src/features/redemption-codes/index.tsx`
- Modify: `web/src/features/wallet/index.tsx`
- Modify: `web/src/hooks/use-sidebar-config.ts`
- Modify: `web/src/i18n/locales/*.json`
- Modify: `web/src/i18n/static-keys.ts`

**Interfaces:**
- Consumes: Tasks 3/4 `/api/benefit` contracts。
- Produces: `/console/benefits` user route and third “Time-limited Vouchers” admin tab。

- [ ] **Step 1: 写表单、危险确认和用户状态失败测试**

```tsx
it('disables publish when random bounds cannot satisfy total budget', async () => {
  // 输入 total=100、count=2、min=60、max=80，断言字段错误和提交未调用。
})

it('requires confirmation and reason before terminating all vouchers', async () => {
  // 选择 terminate_all 后按钮在 confirm 未勾选或 reason 为空时不可用。
})
```

- [ ] **Step 2: 运行 Vitest 确认组件不存在而失败**

Run: `cd web; bun run test -- src/features/benefits/components/__tests__/activity-form.test.tsx src/features/benefits/components/__tests__/terminate-dialog.test.tsx`

- [ ] **Step 3: 实现 API、React Query 页面和管理标签**

使用项目 `api` 实例、数组 query key、mutation 成功后 invalidate；活动表单使用 React Hook Form + Zod。按钮使用 lucide 图标并保留文本标签，强终止为二次确认对话框。

- [ ] **Step 4: 实现用户页、钱包摘要和 i18n**

用户页分为已领取、可领取、往期三个未嵌套页面区段；钱包摘要显示总剩余额度和最近失效时间。更新全部现有 locale，缺少专业翻译时使用清晰英文回退，不留裸中文。

- [ ] **Step 5: 运行测试、类型检查、受影响 lint 和构建后提交**

Run: `cd web; bun run test -- src/features/benefits src/features/usage-logs/lib/billing-details.test.ts`

Run: `cd web; bun run typecheck`

Run: `cd web; bun run lint`

Run: `cd web; bun run build`

Commit: `feat(web): add benefit voucher experience`

### Task 9: Classic 管理端和用户端

**Files:**
- Create: `web/classic/src/pages/Benefits/index.jsx`
- Create: `web/classic/src/components/table/benefits/BenefitActivitiesPanel.jsx`
- Create: `web/classic/src/hooks/benefits/useBenefitsData.jsx`
- Create: `web/classic/src/hooks/benefits/__tests__/benefit-contract.test.mjs`
- Modify: `web/classic/src/pages/Redemption/index.jsx`
- Modify: `web/classic/src/pages/TopUp/index.jsx`
- Modify: `web/classic/src/App.jsx`
- Modify: `web/classic/src/components/layout/SiderBar.jsx`
- Modify: `web/classic/src/hooks/usage-logs/useUsageLogsData.jsx`
- Modify: `web/classic/src/i18n/locales/en.json`

**Interfaces:**
- Consumes: Tasks 3/4 API contracts and Task 7 `billing_breakdown` shape。
- Produces: Classic `/console/benefits` route and admin third tab。

- [ ] **Step 1: 写 API 路径、路由、终止确认和日志拆分契约失败测试**

```js
test('classic benefit actions use underscore-free /api/benefit contract', () => {
  // 从实际 hook 导出 endpoint builder，断言 activities/claim/terminate/ledger 路径。
})
```

- [ ] **Step 2: 运行 node:test 确认失败**

Run: `cd web/classic; node --test src/hooks/benefits/__tests__/benefit-contract.test.mjs`

- [ ] **Step 3: 实现紧凑 Semi UI 页面和路由**

复用 Classic 控制台外壳、Tabs、Table、Modal 和 Toast；不创建营销落地页。管理端和用户端覆盖与 Default 相同的状态和危险确认。

- [ ] **Step 4: 实现钱包摘要、日志拆分和英文翻译**

Classic 中文源文案通过 `t()`；`en.json` 提供完整英文。日志拆分将 quota 依次展示为福利券、订阅、钱包，缺少字段时保持旧单资金源展示。

- [ ] **Step 5: 运行测试和构建后提交**

Run: `cd web/classic; node --test src/hooks/benefits/__tests__/benefit-contract.test.mjs`

Run: `cd web/classic; bun run build`

Commit: `feat(classic): add benefit voucher experience`

### Task 10: 开发文档、跨模块回归与最终验证

**Files:**
- Create: `docs/developer/benefit-vouchers.md`
- Create: `docs/developer/custom-development.md`（当前索引已引用但文件缺失，创建能力总览并登记本功能）
- Create: `docs/workflows/2026-08/31_benefit_voucher_implementation.md`
- Modify: `docs/developer/README.md`
- Modify: `docs/plans/2026-08-31-benefit-voucher-design.md` only if implementation proves a contract correction is required

**Interfaces:**
- Documents: API 请求/响应、状态机、金额与 quota、权限、数据生命周期、迁移、回滚、测试和已知限制。

- [ ] **Step 1: 对照 Issue #77 和设计逐条核验实现**

检查固定/随机活动、实时资格、一人一券、两种终止、券->订阅->钱包、失败回滚、token 总价、分组并发、双前端、日志、报表和三数据库；任何未覆盖项先补失败测试和实现，不在文档中伪报完成。

- [ ] **Step 2: 写专题文档、能力总览和工作记录**

专题文档列出全部路由、DTO 字段、时间边界、权限和示例；工作记录写明根因、修改范围、验证结果和未部署边界；README 链接三个文档。

- [ ] **Step 3: 运行后端完整相关验证**

Run: `go test ./model ./service ./controller ./middleware ./router ./relay -count=1 -timeout 60s`

Run: `cd relaykit; $env:GOWORK='off'; go build ./...`

- [ ] **Step 4: 运行双前端完整验证**

Run: `cd web; bun run test`

Run: `cd web; bun run typecheck`

Run: `cd web; bun run lint`

Run: `cd web; bun run build`

Run: `cd web/classic; bun run test`

Run: `cd web/classic; bun run build`

- [ ] **Step 5: 检查格式、差异和文档链接**

Run: `git diff --check`

Run: `git status --short`

手工核对所有新文档链接存在，API 路径与 router 注册一致，且没有修改 VERSION、部署配置或生产资料。

- [ ] **Step 6: 提交收口文档**

Commit: `docs: document benefit voucher workflow`

## Self-Review Result

- Spec coverage: 数据模型、活动状态、领取、组合计费、token 总价、日志、报表、分组并发、Default、Classic、i18n、三数据库、文档和非部署边界均有对应任务。
- Placeholder scan: 所有任务都给出确定文件、接口、测试和验证命令，没有待定实现项。
- Type consistency: 活动/券/流水模型、`BillingBreakdown`、`RecordConsumeLog` 返回值、API 路径和前端字段在各任务中保持一致。
