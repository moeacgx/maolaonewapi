# 营销福利额度、删除能力与双模板重设计实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让福利活动、福利券、券流水、兑换码和优惠码统一使用系统当前额度展示类型，并补齐安全可审计的单条/批量删除及已确认的 Default/Classic 页面设计。

**Architecture:** 内部 quota 继续作为福利预算、券余额和流水的唯一计费真值，前端只通过全站 formatter 展示；领取门槛继续保存并比较 CNY 实付快照。后端先建立展示金额转换、活动额度配置迁移和删除状态机，再分别实现 Default 与 Classic，两套模板共享 API 合同但不共享组件源码。

**Tech Stack:** Go 1.22+、Gin、GORM v2、SQLite/MySQL/PostgreSQL；React 19 + TypeScript + TanStack Table + shadcn/Base UI（Default）；React 18 + Semi UI（Classic）；Bun、Vitest、node:test、Playwright。

**Spec:** `docs/superpowers/specs/2026-09-01-marketing-benefits-redesign-design.md`

## Global Constraints

- 保留当前脏工作区；禁止 reset、clean、checkout 或覆盖既有福利金额、小时有效期、分组选择和 UI 改动。
- 当前工作区已有 `controller/batch_delete.go`、删除路由、模型方法及部分 Classic 批量按钮草案；每项必须由本计划的失败测试验证后才能保留。
- 页面不得展示裸 `quota` 数字，也不得写死人民币、美元或自定义符号。
- 福利预算、券余额和流水以内部 quota 为真值；领取门槛以 CNY 实付快照为真值。
- 货币展示输入最多两位小数；`TOKENS` 输入必须为正整数。
- 删除使用 GORM 软删除，保留 share、券、流水、支付、充值、优惠使用和管理审计。
- 数据库代码同时支持 SQLite、MySQL 5.7.8+、PostgreSQL 9.6+。
- JSON 解析使用 `common.DecodeJson` 等项目包装函数，不直接调用 `encoding/json` marshal/unmarshal。
- 新增 Go 测试使用 `require`/`assert`，单次命令最大超时 60 秒。
- Default 和 Classic 分别测试、构建和截图；一套通过不能替代另一套。
- 不部署 `maolaoapi`；任何 zzapi 部署也必须由用户在实现完成后再次明确要求。

---

### Task 1: 建立活动展示金额到内部额度的后端合同

**Files:**
- Create: `model/benefit_amount_display.go`
- Create: `model/benefit_amount_display_test.go`
- Modify: `model/benefit_voucher.go`
- Modify: `model/main.go`
- Modify: `controller/benefit.go`
- Modify: `controller/benefit_test.go`

**Interfaces:**
- Consumes: `operation_setting.GetQuotaDisplayType()`、`operation_setting.GetUsdToCurrencyRate()`、`common.QuotaPerUnit`、`common.WalletQuotaFromDecimalStrict()`。
- Produces: `BenefitAmountDisplayContext`、`CurrentBenefitAmountDisplayContext()`、`DisplayAmountToQuota()`、`DisplayAmountToCNYCents()`；`BenefitActivity.FixedQuota/MinQuota/MaxQuota` 和展示设置快照字段。

- [ ] **Step 1: 为四种展示类型写失败测试**

```go
func TestBenefitDisplayAmountToQuota(t *testing.T) {
    tests := []struct {
        name        string
        displayType string
        amount      string
        quota       int64
    }{
        {"USD", operation_setting.QuotaDisplayTypeUSD, "1.25", 625000},
        {"CNY", operation_setting.QuotaDisplayTypeCNY, "7.20", 500000},
        {"CUSTOM", operation_setting.QuotaDisplayTypeCustom, "2.00", 500000},
        {"TOKENS", operation_setting.QuotaDisplayTypeTokens, "500000", 500000},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            ctx := BenefitAmountDisplayContext{
                DisplayType: tc.displayType,
                QuotaPerUnit: decimal.NewFromInt(500000),
                DisplayRate: decimal.NewFromFloat(map[string]float64{
                    "USD": 1, "CNY": 7.2, "CUSTOM": 2, "TOKENS": 1,
                }[tc.name]),
            }
            got, err := ctx.DisplayAmountToQuota(decimal.RequireFromString(tc.amount))
            require.NoError(t, err)
            assert.Equal(t, tc.quota, got)
        })
    }
}
```

同时覆盖货币三位小数、Tokens 小数、零/负数、无效汇率、无效 `QuotaPerUnit` 和超出 BIGINT/钱包安全边界。

- [ ] **Step 2: 运行测试确认合同尚未实现**

Run: `go test ./model -run 'TestBenefitDisplayAmount' -count=1 -timeout 60s`

Expected: FAIL，提示 `BenefitAmountDisplayContext` 或转换方法不存在。

- [ ] **Step 3: 实现独立转换上下文**

```go
type BenefitAmountDisplayContext struct {
    DisplayType string
    QuotaPerUnit decimal.Decimal
    DisplayRate decimal.Decimal
}

func (ctx BenefitAmountDisplayContext) DisplayAmountToQuota(amount decimal.Decimal) (int64, error) {
    if amount.LessThanOrEqual(decimal.Zero) {
        return 0, errors.New("额度必须大于 0")
    }
    if ctx.DisplayType == operation_setting.QuotaDisplayTypeTokens {
        if !amount.IsInteger() {
            return 0, errors.New("Tokens 额度必须为整数")
        }
        return common.WalletQuotaFromDecimalStrict(amount)
    }
    if amount.Exponent() < -2 {
        return 0, errors.New("金额最多只能保留两位小数")
    }
    if ctx.DisplayRate.LessThanOrEqual(decimal.Zero) || ctx.QuotaPerUnit.LessThanOrEqual(decimal.Zero) {
        return 0, errors.New("额度展示配置无效")
    }
    return common.WalletQuotaFromDecimalStrict(
        amount.Div(ctx.DisplayRate).Mul(ctx.QuotaPerUnit),
    )
}
```

`DisplayAmountToCNYCents()` 先转 USD，再乘当前 CNY 汇率并乘 `100`，用于领取门槛；不得复用 quota 结果反推。

- [ ] **Step 4: 将活动配置切换为内部额度字段**

```go
type BenefitActivity struct {
    // 保留现有 Id、Name、Description、GroupId、状态、时间和审计字段
    TotalQuota                    int64  `json:"total_quota"`
    FixedQuota                    int64  `json:"fixed_quota"`
    MinQuota                      int64  `json:"min_quota"`
    MaxQuota                      int64  `json:"max_quota"`
    AmountDisplayTypeSnapshot     string `json:"amount_display_type_snapshot" gorm:"size:16"`
    AmountDisplayRateSnapshot     string `json:"amount_display_rate_snapshot" gorm:"size:64"`
    QuotaPerUnitSnapshot          string `json:"quota_per_unit_snapshot" gorm:"size:64"`
    ClaimPaidThresholdCents       int64  `json:"claim_paid_threshold_cents"`
}
```

把份额拆分输入改为 `TotalQuota/FixedQuota/MinQuota/MaxQuota`。固定模式直接以
`FixedQuota * TotalCount` 安全计算 `TotalQuota`，避免展示汇率换算的两次舍入产生冲突；随机模式验证
`MinQuota * count <= TotalQuota <= MaxQuota * count`；发布后 share 的 `Quota` 总和必须严格等于 `TotalQuota`。

- [ ] **Step 5: 增加幂等迁移并在 AutoMigrate 后调用**

迁移函数签名：

```go
func migrateBenefitActivityQuotaConfig(db *gorm.DB) error
```

已发布活动从现有 shares 的 `Quota` 推导固定/最小/最大额度；旧草稿按旧 CNY 语义转换一次。
迁移只更新新字段为零的记录，重复启动不再次换算。异常活动返回错误并停止迁移，不补零。

- [ ] **Step 6: 更新管理请求和响应合同**

```go
type benefitActivityRequest struct {
    AmountDisplayType string          `json:"amount_display_type"`
    TotalAmount       decimal.Decimal `json:"total_amount"`
    FixedAmount       decimal.Decimal `json:"fixed_amount"`
    MinAmount         decimal.Decimal `json:"min_amount"`
    MaxAmount         decimal.Decimal `json:"max_amount"`
    ClaimPaidThreshold decimal.Decimal `json:"claim_paid_threshold"`
}
```

请求的 `amount_display_type` 与服务端当前设置不一致时返回可识别错误
`benefit_amount_display_changed`。响应暴露内部 `*_quota` 与快照，旧 `*_amount_cents` 不再作为新前端回显合同。

- [ ] **Step 7: 运行金额、拆分和迁移测试**

Run: `go test ./model ./controller -run 'Benefit.*(Amount|Quota|Share|Migration|Request)' -count=1 -timeout 60s`

Expected: PASS；固定和随机拆分总和精确，领取门槛仍为 CNY 实付快照。

- [ ] **Step 8: 提交后端金额合同**

```bash
git add model/benefit_amount_display.go model/benefit_amount_display_test.go model/benefit_voucher.go model/main.go controller/benefit.go controller/benefit_test.go
git commit -m "fix: 统一福利活动额度展示合同"
```

---

### Task 2: 补齐券列表、用户流水与管理报表 API

**Files:**
- Modify: `model/benefit_voucher.go`
- Modify: `controller/benefit.go`
- Modify: `router/api-router.go`
- Modify: `model/benefit_voucher_test.go`
- Modify: `controller/benefit_test.go`
- Create: `controller/benefit_voucher_view_test.go`

**Interfaces:**
- Consumes: Task 1 的 `BenefitActivity` 内部额度字段和现有 `BenefitVoucherLedger`。
- Produces: `BenefitVoucherAdminView`、`BenefitVoucherListFilter`、`BenefitVoucherBatchResult`、分页管理券列表、`VoidBenefitVouchers()`；用户自有券流水接口。

- [ ] **Step 1: 写用户越权和管理筛选失败测试**

```go
func TestGetBenefitVoucherLedgerRejectsOtherUser(t *testing.T) {
    // voucher belongs to user 10; request context user is 11
    recorder := requestBenefitLedgerAsUser(t, 11, voucher.Id)
    assert.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestListBenefitVouchersForAdminFiltersByUserAndStatus(t *testing.T) {
    got, total, err := ListBenefitVouchersForAdmin(activity.Id, BenefitVoucherListFilter{
        Keyword: "alice", Status: BenefitVoucherStatusActive,
    }, 0, 20)
    require.NoError(t, err)
    assert.Equal(t, int64(1), total)
    assert.Equal(t, "alice", got[0].Username)
}

func TestVoidBenefitVouchersWritesOneLedgerPerActiveVoucher(t *testing.T) {
    result, err := VoidBenefitVouchers([]int{active.Id, exhausted.Id}, 99, "campaign cleanup", now)
    require.NoError(t, err)
    assert.Equal(t, []int{active.Id}, result.UpdatedIds)
    assert.Equal(t, "not_active", result.Skipped[0].Reason)
}
```

- [ ] **Step 2: 运行测试确认现有接口不满足**

Run: `go test ./model ./controller -run 'BenefitVoucher.*(Ledger|Filter|View)' -count=1 -timeout 60s`

Expected: FAIL，当前用户无自有流水路由，管理列表不支持筛选/用户名。

- [ ] **Step 3: 建立管理券视图与分页查询**

```go
type BenefitVoucherListFilter struct {
    Keyword string
    Status string
}

type BenefitVoucherAdminView struct {
    BenefitUserVoucher
    ActivityName string `json:"activity_name"`
    GroupNameSnapshot string `json:"group_name_snapshot"`
    Username string `json:"username"`
}

func ListBenefitVouchersForAdmin(
    activityID int,
    filter BenefitVoucherListFilter,
    offset, limit int,
) ([]BenefitVoucherAdminView, int64, error)
```

使用 GORM joins 和参数绑定；活动历史名称使用活动/券快照，不能因活动软删除而丢失。

- [ ] **Step 4: 扩展管理券接口查询参数**

`GET /api/benefit/admin/activities/:id/vouchers?p=1&page_size=20&keyword=alice&status=active`
返回通用分页对象 `{items,total,page,page_size}`，旧的无分页数组不再作为新 UI 合同。

- [ ] **Step 5: 新增用户自有券流水接口**

```go
userBenefitRoute.GET("/vouchers/:id/ledger", controller.GetBenefitVoucherLedger)
```

控制器必须先按 `voucher_id + user_id` 验证所有权；普通用户响应剥离操作人和
`admin_info`，管理员原接口保留受保护元数据。

- [ ] **Step 6: 让报表直接返回实际额度汇总**

保持 `BenefitActivityReport` 的 quota 汇总字段，删除控制器和前端依赖的人民币比例换算。
为报告增加份数统计（总份数、已发放、已使用、过期）时直接由 share/voucher 状态聚合。

- [ ] **Step 7: 新增批量作废接口并复用单券流水规则**

```go
type VoidBenefitVoucherRequest struct {
    Ids []int `json:"ids"`
    Reason string `json:"reason"`
    Confirm bool `json:"confirm"`
}

type BenefitVoucherBatchSkipped struct {
    Id int `json:"id"`
    Reason string `json:"reason"`
}

type BenefitVoucherBatchResult struct {
    UpdatedIds []int `json:"updated_ids"`
    Skipped []BenefitVoucherBatchSkipped `json:"skipped"`
}

func VoidBenefitVouchers(ids []int, operatorID int, reason string, now int64) (BenefitVoucherBatchResult, error)
```

接口为 `POST /api/benefit/admin/vouchers/batch-void`。事务内锁定所有券，仅 active 且有剩余
额度的券可作废；每张券写一条带 operator/reason 的 `void` 流水。数据库错误整批回滚，业务
状态不允许的券返回逐 ID 原因。

- [ ] **Step 8: 运行 API 行为测试**

Run: `go test ./model ./controller ./router -run 'Benefit.*(Voucher|Ledger|Report|Route|Void)' -count=1 -timeout 60s`

Expected: PASS；用户不能读取他人流水，管理员可筛选，已删活动的券仍可审计。

- [ ] **Step 9: 提交券查询合同**

```bash
git add model/benefit_voucher.go model/benefit_voucher_test.go controller/benefit.go controller/benefit_test.go controller/benefit_voucher_view_test.go router/api-router.go
git commit -m "feat: 完善福利券列表与流水接口"
```

---

### Task 3: 完成三类资源的安全删除状态机

**Files:**
- Create: `model/marketing_delete.go`
- Modify: `controller/batch_delete.go`
- Modify: `controller/batch_delete_test.go`
- Modify: `controller/benefit.go`
- Modify: `controller/redemption.go`
- Modify: `controller/promo_code.go`
- Modify: `controller/audit.go`
- Modify: `model/benefit_voucher.go`
- Modify: `model/benefit_voucher_test.go`
- Modify: `model/redemption.go`
- Modify: `model/redemption_test.go`
- Modify: `model/promo_code.go`
- Modify: `model/promo_code_lifecycle_test.go`
- Modify: `router/api-router.go`
- Modify: `router/promo_code_router_test.go`
- Create: `router/marketing_delete_router_test.go`
- Modify: `docs/workflows/2026-09/04_benefit_batch_delete_contract.md`

**Interfaces:**
- Consumes: Task 2 的活动/券状态和既有 PromoCode reservation `Unscoped` 结算能力。
- Produces: `model.BatchDeleteResult`、福利活动单条/批量删除、兑换码批量删除、优惠码批量/失效删除。

- [ ] **Step 1: 把现有草案改写成状态矩阵测试**

```go
type BatchDeleteSkipped struct {
    Id int `json:"id"`
    Reason string `json:"reason"`
}

type BatchDeleteResult struct {
    DeletedIds []int `json:"deleted_ids"`
    Skipped []BatchDeleteSkipped `json:"skipped"`
}
```

上述类型定义在 `model/marketing_delete.go`；controller 直接返回 `model.BatchDeleteResult`，
model 不得依赖 controller。

测试必须覆盖：draft 可删；published/paused 拒绝；ended 归一化后可删；terminated/all 可删；
terminated/unused 有 active 券时拒绝；不存在 ID 返回 `not_found`；重复调用幂等。

- [ ] **Step 2: 运行删除测试确认当前草案不完整**

Run: `go test ./model ./controller ./router -run '(BatchDelete|DeleteBenefit|DeletePromo|DeleteRedemption|MarketingDelete)' -count=1 -timeout 60s`

Expected: FAIL；当前草案只返回数量、未允许安全 draft、未返回逐 ID 原因，且缺少优惠码失效清理。

- [ ] **Step 3: 实现活动删除判定和单条接口**

```go
func DeleteBenefitActivitiesByIDs(ids []int, now int64) (BatchDeleteResult, error)
```

事务内锁定活动，调用过期归一化，再查询 active 券和未完成额度状态。只软删活动；share、券、
流水不删。增加 `DELETE /api/benefit/admin/activities/:id`，单条复用同一模型函数。

- [ ] **Step 4: 完成兑换码与优惠码批量删除**

```go
func DeleteRedemptionsByIDs(ids []int) ([]int, error)
func DeletePromoCodesByIDs(ids []int) ([]int, error)
func DeleteInvalidPromoCodes(now int64) ([]int, error)
```

优惠码逐条先写 `deleted_id=id` 再软删；已有 reservation 的支付回调继续通过 `Unscoped`
结算。兑换码保留充值日志和返佣来源。

- [ ] **Step 5: 注册路由并增加关键操作限流**

```go
redemptionRoute.DELETE("/invalid", middleware.CriticalRateLimit(), controller.DeleteInvalidRedemption)
redemptionRoute.DELETE("/batch", middleware.CriticalRateLimit(), controller.BatchDeleteRedemptions)
promoCodeRoute.DELETE("/invalid", middleware.CriticalRateLimit(), controller.DeleteInvalidPromoCodes)
promoCodeRoute.DELETE("/batch", middleware.CriticalRateLimit(), controller.BatchDeletePromoCodes)
adminBenefitRoute.DELETE("/activities/batch", middleware.CriticalRateLimit(), controller.BatchDeleteBenefitAdminActivities)
adminBenefitRoute.DELETE("/activities/:id", middleware.CriticalRateLimit(), controller.DeleteBenefitAdminActivity)
```

静态 `/batch`、`/invalid` 路由必须在 `/:id` 之前注册。

- [ ] **Step 6: 补齐单条、批量和失效清理审计**

审计详情只记录资源 ID、实际删除数、跳过数和原因代码，不记录完整兑换码/优惠码明文。

- [ ] **Step 7: 验证三数据库兼容查询和 reservation 回调**

Run: `go test ./model ./controller ./router -run '(BatchDelete|DeleteInvalid|Promo.*Reservation|Benefit.*Delete)' -count=1 -timeout 60s`

Expected: PASS；软删除后的已预留优惠订单仍能结算，新的使用请求查不到软删除优惠码。

- [ ] **Step 8: 提交删除合同**

```bash
git add controller/batch_delete.go controller/batch_delete_test.go controller/benefit.go controller/redemption.go controller/promo_code.go controller/audit.go model/marketing_delete.go model/benefit_voucher.go model/benefit_voucher_test.go model/redemption.go model/redemption_test.go model/promo_code.go model/promo_code_lifecycle_test.go router/api-router.go router/promo_code_router_test.go router/marketing_delete_router_test.go docs/workflows/2026-09/04_benefit_batch_delete_contract.md
git commit -m "feat: 完善营销资源批量软删除"
```

---

### Task 4: 重设计 Default 用户活动福利与券流水

**Files:**
- Modify: `web/src/features/benefits/api.ts`
- Modify: `web/src/features/benefits/types.ts`
- Modify: `web/src/features/benefits/index.tsx`
- Create: `web/src/features/benefits/components/benefit-summary.tsx`
- Create: `web/src/features/benefits/components/user-voucher-card.tsx`
- Create: `web/src/features/benefits/components/claimable-activity-card.tsx`
- Create: `web/src/features/benefits/components/user-voucher-ledger-sheet.tsx`
- Create: `web/src/features/benefits/components/__tests__/user-benefits.test.tsx`
- Create: `web/src/features/benefits/components/__tests__/user-voucher-ledger.test.tsx`

**Interfaces:**
- Consumes: Task 2 的用户 activities/vouchers/ledger API；`formatQuota()`、`formatTimestampToDate()`。
- Produces: 已确认渲染稿中的用户摘要、券卡、活动卡和流水 Sheet。

- [ ] **Step 1: 写 formatter 与交互失败测试**

```tsx
it('renders original used and remaining values through formatQuota', () => {
  renderUserBenefits({ original_quota: 500000, used_quota: 125000, remaining_quota: 375000 })
  expect(screen.getByText('$1.00')).toBeInTheDocument()
  expect(screen.getByText('$0.25')).toBeInTheDocument()
  expect(screen.getByText('$0.75')).toBeInTheDocument()
  expect(screen.queryByText('500000')).not.toBeInTheDocument()
})

it('opens an owned voucher ledger sheet', async () => {
  renderUserBenefits(activeVoucher)
  await user.click(screen.getByRole('button', { name: /view ledger/i }))
  expect(await screen.findByText(/settlement deduction/i)).toBeVisible()
})
```

- [ ] **Step 2: 运行测试确认旧页面失败**

Run: `cd web && bun run test --run src/features/benefits/components/__tests__/user-benefits.test.tsx src/features/benefits/components/__tests__/user-voucher-ledger.test.tsx`

Expected: FAIL；旧页面写死 `¥`、显示原始字段且没有用户流水 Sheet。

- [ ] **Step 3: 更新 API 类型和用户流水调用**

删除 `original_amount/original_amount_cents` 的 UI 依赖；增加：

```ts
export async function getBenefitVoucherLedger(id: number) {
  const response = await api.get<ApiResponse<BenefitLedgerEntry[]>>(
    `/api/benefit/vouchers/${id}/ledger`
  )
  if (!response.data.success) throw new Error(response.data.message)
  return response.data.data ?? []
}
```

- [ ] **Step 4: 实现四项摘要和券卡**

摘要由券状态与实际 quota 汇总；券卡展示 `formatQuota(original_quota)`、
`formatQuota(used_quota)`、`formatQuota(remaining_quota)`，进度用
`used_quota / original_quota`，不得用显示货币做运算。

- [ ] **Step 5: 实现可领取活动卡与状态文案**

展示分组、剩余份额、每份额度、个人有效期和资格原因；已领取、未开始、已结束、已领完都
提供明确状态，只有 `eligible=true && has_claimed=false` 时显示可点击领取按钮。

- [ ] **Step 6: 实现用户流水 Sheet**

用 `formatLogQuota(entry.quota_delta)` 和 `formatQuota(entry.balance_after)`，按时间倒序；
普通用户不渲染管理员元数据。提供 loading、error、empty 和 retry 状态。

- [ ] **Step 7: 运行 Default 用户页测试与静态检查**

Run: `cd web && bun run test --run src/features/benefits/components/__tests__/user-benefits.test.tsx src/features/benefits/components/__tests__/user-voucher-ledger.test.tsx && bun run typecheck && bun run lint`

Expected: PASS，无裸 quota、无固定货币符号。

- [ ] **Step 8: 提交 Default 用户页**

```bash
git add web/src/features/benefits
git commit -m "feat: 重设计新版活动福利与券流水"
```

---

### Task 5: 完成 Default 福利活动管理、券列表和报表

**Files:**
- Modify: `web/src/features/benefits/api.ts`
- Modify: `web/src/features/benefits/types.ts`
- Modify: `web/src/features/benefits/components/benefit-activity-form.tsx`
- Modify: `web/src/features/benefits/components/benefit-activities-panel.tsx`
- Create: `web/src/features/benefits/components/benefit-activity-report.tsx`
- Create: `web/src/features/benefits/components/benefit-vouchers-sheet.tsx`
- Create: `web/src/features/benefits/components/admin-voucher-ledger.tsx`
- Modify: `web/src/features/benefits/components/__tests__/activity-form.test.tsx`
- Create: `web/src/features/benefits/components/__tests__/activity-delete.test.tsx`
- Create: `web/src/features/benefits/components/__tests__/admin-vouchers.test.tsx`

**Interfaces:**
- Consumes: Tasks 1-3 的活动额度、分页券列表和删除结果。
- Produces: Default 活动多选删除、动态展示单位表单、实际额度报表、完整券表和管理流水。

- [ ] **Step 1: 写动态单位和删除状态失败测试**

```tsx
it.each([
  ['USD', '$', 0.01],
  ['CNY', '¥', 0.01],
  ['CUSTOM', '¤', 0.01],
  ['TOKENS', 'Tokens', 1],
])('uses %s activity amount inputs', (type, unit, step) => {
  setCurrencyConfig({ quotaDisplayType: type })
  render(<BenefitActivityForm />)
  expect(screen.getByLabelText(/total budget/i)).toHaveAttribute('step', String(step))
  expect(screen.getByText(unit)).toBeVisible()
})

it('keeps running activities unselected for deletion', () => {
  renderActivities([publishedActivity, endedActivity])
  expect(rowCheckbox(publishedActivity)).toBeDisabled()
  expect(rowCheckbox(endedActivity)).toBeEnabled()
})
```

- [ ] **Step 2: 运行测试确认当前实现失败**

Run: `cd web && bun run test --run src/features/benefits/components/__tests__/activity-form.test.tsx src/features/benefits/components/__tests__/activity-delete.test.tsx src/features/benefits/components/__tests__/admin-vouchers.test.tsx`

Expected: FAIL；旧表单按人民币字段，活动表无批量选择，券列表和报表仍是简化视图。

- [ ] **Step 3: 用全局 formatter 更新活动表单**

使用 `useSystemConfigStore((s) => s.config.currency)`、`quotaUnitsToEditableAmount()`、
`parseQuotaFromDollars()` 和 `getEditableQuotaStep()`。提交 `amount_display_type`；后端返回
`benefit_amount_display_changed` 时保留输入并提示刷新。

- [ ] **Step 4: 增加活动多选和批量删除 API**

```ts
export type BatchDeleteResult = {
  deleted_ids: number[]
  skipped: Array<{ id: number; reason: string }>
}

export async function deleteAdminBenefitActivities(ids: number[]) {
  const response = await api.delete<ApiResponse<BatchDeleteResult>>(
    '/api/benefit/admin/activities/batch',
    { data: { ids } }
  )
  return response.data
}
```

成功后清除已删除选择；`skipped` 转为逐项可读提示；当前页为空时回退上一页。

- [ ] **Step 5: 抽出实际额度报表组件**

报表所有金额直接 `formatQuota(report.*_quota)`；进度只使用原始额度比率。删除
`reportAmountFromQuota`、`total_amount` 比例反推和“所有金额按人民币”提示。

- [ ] **Step 6: 实现管理券表和流水**

券表字段按渲染稿：券 ID、活动/分组、用户、原始/已用/剩余、状态、领取/失效时间、操作。
支持 keyword/status、分页、批量作废；流水展示 request/log 关联和管理员元数据。

- [ ] **Step 7: 运行 Default 管理页测试和构建**

Run: `cd web && bun run test --run src/features/benefits/components/__tests__ && bun run typecheck && bun run lint && bun run build`

Expected: PASS；构建产物无类型错误，报表无人民币硬编码或裸 quota。

- [ ] **Step 8: 提交 Default 管理页**

```bash
git add web/src/features/benefits
git commit -m "feat: 完善新版福利活动管理与券报表"
```

---

### Task 6: 完成 Default 兑换码与优惠码批量管理

**Files:**
- Modify: `web/src/features/redemption-codes/api.ts`
- Modify: `web/src/features/redemption-codes/types.ts`
- Modify: `web/src/features/redemption-codes/components/data-table-bulk-actions.tsx`
- Modify: `web/src/features/redemption-codes/components/redemptions-table.tsx`
- Modify: `web/src/features/redemption-codes/components/redemptions-primary-buttons.tsx`
- Modify: `web/src/features/redemption-codes/components/promo-codes-panel.tsx`
- Create: `web/src/features/redemption-codes/components/__tests__/redemption-bulk-delete.test.tsx`
- Create: `web/src/features/redemption-codes/components/__tests__/promo-code-bulk-delete.test.tsx`

**Interfaces:**
- Consumes: Task 3 的 `/redemption/batch|invalid`、`/promo_code/batch|invalid`。
- Produces: Default 兑换码删除所选、优惠码多选/删除所选/清理失效及正确 API 路径。

- [ ] **Step 1: 写批量工具栏失败测试**

```tsx
it('deletes selected redemption ids and clears selection', async () => {
  renderRedemptions([code(1), code(2)])
  await selectRows(1, 2)
  await user.click(screen.getByRole('button', { name: /delete selected/i }))
  await confirmDelete()
  expect(api.delete).toHaveBeenCalledWith('/api/redemption/batch', {
    data: { ids: [1, 2] },
  })
})

it('uses the registered promo_code route for batch delete', async () => {
  renderPromoCodes([promo(7)])
  await selectRows(7)
  await deleteSelected()
  expect(api.delete).toHaveBeenCalledWith('/api/promo_code/batch', {
    data: { ids: [7] },
  })
})
```

- [ ] **Step 2: 运行测试确认 Default 现状失败**

Run: `cd web && bun run test --run src/features/redemption-codes/components/__tests__/redemption-bulk-delete.test.tsx src/features/redemption-codes/components/__tests__/promo-code-bulk-delete.test.tsx`

Expected: FAIL；兑换码工具栏只有复制，优惠码无选择，且 API 仍误用 `/promo-code`。

- [ ] **Step 3: 统一 Default API 路径和批量方法**

所有优惠码请求改为注册路由 `/api/promo_code`。增加：

```ts
export const deleteRedemptions = (ids: number[]) =>
  api.delete('/api/redemption/batch', { data: { ids } }).then((r) => r.data)
export const deletePromoCodes = (ids: number[]) =>
  api.delete('/api/promo_code/batch', { data: { ids } }).then((r) => r.data)
export const deleteInvalidPromoCodes = () =>
  api.delete('/api/promo_code/invalid').then((r) => r.data)
```

- [ ] **Step 4: 给兑换码批量栏增加删除所选**

复用 `ConfirmDialog`；删除成功清空 selection 并触发 refresh，保留现有复制和“清理失效”。

- [ ] **Step 5: 给优惠码表增加复选框与批量栏**

使用受控 `Set<number>` 保存当前页选择；表头全选只选择当前页。工具栏提供删除所选和清理
失效；分页后清除不可见选择，避免误删上一页记录。

- [ ] **Step 6: 处理批量成功、部分跳过和空页回退**

对 `deleted_ids` 逐项从本地状态移除；有 `skipped` 时显示原因；删除当前页最后一项后把页码
减一并重新查询。

- [ ] **Step 7: 运行 Default 营销码测试和构建**

Run: `cd web && bun run test --run src/features/redemption-codes/components/__tests__ && bun run typecheck && bun run lint && bun run build`

Expected: PASS；所有 promo API 使用下划线路由，批量栏键盘可操作。

- [ ] **Step 8: 提交 Default 营销码管理**

```bash
git add web/src/features/redemption-codes
git commit -m "feat: 增加新版营销码批量管理"
```

---

### Task 7: 重设计 Classic 用户活动福利与券流水

**Files:**
- Modify: `web/classic/src/pages/Benefits/index.jsx`
- Modify: `web/classic/src/hooks/benefits/useBenefitsData.jsx`
- Create: `web/classic/src/components/benefits/BenefitSummary.jsx`
- Create: `web/classic/src/components/benefits/UserVoucherCard.jsx`
- Create: `web/classic/src/components/benefits/ClaimableActivityCard.jsx`
- Create: `web/classic/src/components/benefits/UserVoucherLedgerSheet.jsx`
- Modify: `web/classic/src/index.css`
- Modify: `web/classic/src/hooks/benefits/__tests__/benefit-contract.test.mjs`
- Create: `web/classic/src/components/benefits/__tests__/user-benefits-contract.test.mjs`

**Interfaces:**
- Consumes: Task 2 用户 API；Classic `renderQuota()`、`timestamp2string()`、Semi Card/SideSheet。
- Produces: 与 Default 信息一致、符合 Classic 容器习惯的用户福利页。

- [ ] **Step 1: 写无裸 quota 和流水交互失败测试**

```js
test('Classic voucher cards format all quota fields', () => {
  assert.match(source, /renderQuota\(voucher\.original_quota\)/)
  assert.match(source, /renderQuota\(voucher\.used_quota\)/)
  assert.match(source, /renderQuota\(voucher\.remaining_quota\)/)
  assert.doesNotMatch(source, /\{voucher\.original_quota\}/)
})
```

源码合同断言 SideSheet 使用独立 loading/error/data 状态，并包含变更额度、变更后余额和空态；
真实点击行为留到 Task 10 Playwright 验收。

- [ ] **Step 2: 运行 Classic 失败测试**

Run: `cd web/classic && node --test src/hooks/benefits/__tests__/benefit-contract.test.mjs src/components/benefits/__tests__/user-benefits-contract.test.mjs`

Expected: FAIL；旧页面仍直接输出 `original_quota/used_quota`，没有用户流水。

- [ ] **Step 3: 扩展 Classic hook**

增加 `loadVoucherLedger(voucherId)`，独立管理 ledger loading/error/data；领取成功只刷新活动和券，
不复用管理员 API。

- [ ] **Step 4: 实现摘要、券卡和活动卡**

所有额度调用 `renderQuota`；进度按 quota 比率。使用 `classic-console-panel`，卡片内部不再套
无意义外层 Card；长活动名和分组名可换行，不遮挡状态和按钮。

- [ ] **Step 5: 实现 Classic 流水 SideSheet**

时间线显示类型、`renderQuota(quota_delta)`、`renderQuota(balance_after)`、时间和请求/日志关联；
移动端 width 使用 `min(620px, 100vw)` 并保证 footer/关闭按钮可达。

- [ ] **Step 6: 增加响应式和暗色样式**

在 `index.css` 使用 Semi 主题变量；桌面双列、窄屏单列，摘要在 768px 以下两列、390px 以下
单列。不得增加硬编码白色背景导致暗色失效。

- [ ] **Step 7: 运行 Classic 用户页验证**

Run: `cd web/classic && node --test src/hooks/benefits/__tests__/benefit-contract.test.mjs src/components/benefits/__tests__/user-benefits-contract.test.mjs && npx eslint src/pages/Benefits/index.jsx src/components/benefits src/hooks/benefits/useBenefitsData.jsx --no-cache && npx prettier --check src/pages/Benefits/index.jsx src/components/benefits src/hooks/benefits/useBenefitsData.jsx src/index.css`

Expected: PASS。

- [ ] **Step 8: 提交 Classic 用户页**

```bash
git add web/classic/src/pages/Benefits web/classic/src/components/benefits web/classic/src/hooks/benefits web/classic/src/index.css
git commit -m "feat: 重设计经典版活动福利与券流水"
```

---

### Task 8: 完成 Classic 福利活动管理、券列表和报表

**Files:**
- Modify: `web/classic/src/components/table/benefits/BenefitActivitiesPanel.jsx`
- Create: `web/classic/src/components/table/benefits/BenefitActivityReport.jsx`
- Create: `web/classic/src/components/table/benefits/BenefitVoucherTable.jsx`
- Create: `web/classic/src/components/table/benefits/BenefitVoucherLedger.jsx`
- Create: `web/classic/src/components/table/benefits/BenefitActivityBatchActions.jsx`
- Modify: `web/classic/src/hooks/benefits/__tests__/benefit-contract.test.mjs`
- Create: `web/classic/src/components/table/benefits/__tests__/activity-management-contract.test.mjs`

**Interfaces:**
- Consumes: Tasks 1-3 的管理活动、券、流水、报表和删除 API。
- Produces: Classic 动态单位表单、活动多选删除、实际额度报表、完整券表和管理流水。

- [ ] **Step 1: 写活动删除和报表额度失败测试**

```js
test('Classic report formats actual quota directly', () => {
  assert.match(reportSource, /renderQuota\(report\.used_quota\)/)
  assert.doesNotMatch(reportSource, /reportAmountFromQuota/)
  assert.doesNotMatch(reportSource, /人民币|formatAmount\(/)
})

test('Classic activity table selects deletable history only', () => {
  assert.match(source, /rowSelection/)
  assert.match(source, /deleted_ids/)
  assert.match(source, /skipped/)
})
```

- [ ] **Step 2: 运行失败测试**

Run: `cd web/classic && node --test src/hooks/benefits/__tests__/benefit-contract.test.mjs src/components/table/benefits/__tests__/activity-management-contract.test.mjs`

Expected: FAIL；当前报表仍按人民币比例反推，现有批量草案未返回逐项结果。

- [ ] **Step 3: 拆分 1000+ 行活动面板**

保留数据加载和活动状态操作在 `BenefitActivitiesPanel.jsx`；报表、券表、流水和批量栏各自独立
文件。不要抽取只有一个机械调用的无意义 helper。

- [ ] **Step 4: 更新动态单位表单**

通过 `quotaToDisplayAmount`/`displayAmountToQuota` 回显和提交，单位由 `getCurrencyConfig()`
决定；Tokens step=1，货币 step=0.01。提交 `amount_display_type`，不提交 `*_cents`。

- [ ] **Step 5: 实现活动多选删除与结果提示**

仅 draft/ended/安全 terminated 行可选；进行中行 checkbox disabled 并有 tooltip。确认弹窗展示选择
数量；成功 Toast 显示删除数，跳过项按 reason 列出。

- [ ] **Step 6: 实现券表、筛选、分页和流水**

Semi Table 使用稳定 `rowKey=id`；所有额度 `renderQuota`。支持 keyword/status；选中 active 券时
显示批量作废。流水用独立详情面板，不再把 `entry.type/quota_delta/balance_after` 挤成两列。

- [ ] **Step 7: 运行 Classic 管理页测试和构建**

Run: `cd web/classic && node --test src/hooks/benefits/__tests__/benefit-contract.test.mjs src/components/table/benefits/__tests__/activity-management-contract.test.mjs && npx eslint src/components/table/benefits --no-cache && npx prettier --check src/components/table/benefits src/hooks/benefits/__tests__/benefit-contract.test.mjs && npx vite build`

Expected: PASS；报表无固定货币文案，券列表和流水信息完整。

- [ ] **Step 8: 提交 Classic 管理页**

```bash
git add web/classic/src/components/table/benefits web/classic/src/hooks/benefits/__tests__/benefit-contract.test.mjs
git commit -m "feat: 完善经典版福利活动与券报表"
```

---

### Task 9: 完成 Classic 兑换码与优惠码批量管理

**Files:**
- Modify: `web/classic/src/hooks/redemptions/useRedemptionsData.jsx`
- Modify: `web/classic/src/components/table/redemptions/RedemptionsActions.jsx`
- Modify: `web/classic/src/components/table/redemptions/RedemptionsTable.jsx`
- Modify: `web/classic/src/hooks/promo-codes/usePromoCodesData.jsx`
- Modify: `web/classic/src/components/table/promo-codes/PromoCodesPanel.jsx`
- Modify: `web/classic/src/hooks/promo-codes/__tests__/usePromoCodesData-contract.test.mjs`
- Create: `web/classic/src/hooks/redemptions/__tests__/bulk-delete-contract.test.mjs`

**Interfaces:**
- Consumes: Task 3 的批量和失效清理 API。
- Produces: Classic 兑换码删除所选 + 清理失效、优惠码多选 + 删除所选 + 清理失效。

- [ ] **Step 1: 写现有草案行为失败测试**

```js
test('selected redemption deletion is distinct from invalid cleanup', () => {
  assert.match(source, /batchDeleteSelectedRedemptions/)
  assert.match(source, /\/api\/redemption\/batch/)
  assert.match(source, /\/api\/redemption\/invalid/)
  assert.doesNotMatch(actions, /onClick=\{batchDeleteRedemptions\}[\s\S]*删除所选/)
})
```

优惠码测试断言 rowSelection、`/api/promo_code/batch`、`/api/promo_code/invalid` 和选择清空。

- [ ] **Step 2: 运行测试确认草案接线问题**

Run: `cd web/classic && node --test src/hooks/redemptions/__tests__/bulk-delete-contract.test.mjs src/hooks/promo-codes/__tests__/usePromoCodesData-contract.test.mjs`

Expected: FAIL；当前工作区草案的按钮/函数命名和返回结构仍可能错接。

- [ ] **Step 3: 分离兑换码两个删除命令**

`deleteSelectedRedemptions()` 只调用 `/batch`；`deleteInvalidRedemptions()` 只调用 `/invalid`。
按钮文案分别为“删除所选”和“清理失效”，都用二次确认。

- [ ] **Step 4: 完成优惠码多选和失效清理**

`selectedRowKeys` 保存 ID，不保存整行对象；分页/搜索改变后清理选择。单条、批量和失效删除
都刷新正确页；最后一页清空时回退。

- [ ] **Step 5: 统一营销码模块边界和操作聚合**

表格放在 `classic-console-panel` 内容区；行内编辑/启停/删除聚合到操作菜单，批量命令只在选择
工具栏出现，避免按钮堆叠。

- [ ] **Step 6: 运行 Classic 营销码验证**

Run: `cd web/classic && node --test src/hooks/redemptions/__tests__/bulk-delete-contract.test.mjs src/hooks/promo-codes/__tests__/usePromoCodesData-contract.test.mjs && npx eslint src/hooks/redemptions src/hooks/promo-codes src/components/table/redemptions src/components/table/promo-codes --no-cache && npx prettier --check src/hooks/redemptions src/hooks/promo-codes src/components/table/redemptions src/components/table/promo-codes && npx vite build`

Expected: PASS。

- [ ] **Step 7: 提交 Classic 营销码管理**

```bash
git add web/classic/src/hooks/redemptions web/classic/src/hooks/promo-codes web/classic/src/components/table/redemptions web/classic/src/components/table/promo-codes
git commit -m "feat: 增加经典版营销码批量管理"
```

---

### Task 10: 同步 i18n、开发文档并完成端到端验收

**Files:**
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/vi.json`
- Modify: `web/src/i18n/static-keys.ts`
- Modify: `web/classic/src/i18n/locales/en.json`
- Modify: `web/classic/src/i18n/locales/zh.json`
- Modify: `web/classic/src/i18n/locales/zh-CN.json`
- Modify: `web/classic/src/i18n/locales/zh-TW.json`
- Modify: `web/classic/src/i18n/locales/fr.json`
- Modify: `web/classic/src/i18n/locales/ru.json`
- Modify: `web/classic/src/i18n/locales/ja.json`
- Modify: `web/classic/src/i18n/locales/vi.json`
- Modify: `docs/developer/benefit-vouchers.md`
- Modify: `docs/developer/custom-development.md`
- Modify: `docs/developer/README.md`
- Create: `docs/workflows/2026-09/05_marketing_benefits_redesign.md`
- Create: `output/playwright/marketing-benefits-default-1440.png`
- Create: `output/playwright/marketing-benefits-classic-1440.png`
- Create: `output/playwright/marketing-benefits-mobile-390.png`

**Interfaces:**
- Consumes: Tasks 1-9 的完整行为与两套页面。
- Produces: 完整翻译、长期文档、测试/截图/构建证据和可发布结论。

- [ ] **Step 1: 同步两套翻译键**

新增并核对：真实额度、原始/已用/剩余、即将到期、查看流水、删除所选、清理失效、跳过原因、
展示设置变化、进行中不可删除、批量作废等文案。Default 执行 `bun run i18n:sync`；Classic
执行 `npx i18next-cli` 对应检查，禁止只补中文。

- [ ] **Step 2: 运行完整后端验证**

Run: `go test ./model ./controller ./router -count=1 -timeout 60s`

Expected: PASS。

Run: `go test ./... -count=1 -timeout 60s`

Expected: PASS；若根模块 embed 需要前端产物，先完成下面两套 build 再重跑，不修改 embed 合同规避测试。

- [ ] **Step 3: 运行完整 Default 验证**

Run: `cd web && bun run test && bun run typecheck && bun run lint && bun run format:check && bun run copyright:check && bun run build`

Expected: PASS；若仓库基线存在无关失败，必须另列文件和错误，受影响文件仍需全过。

- [ ] **Step 4: 运行完整 Classic 验证**

Run: `cd web/classic && node --test src/hooks/benefits/__tests__/benefit-contract.test.mjs src/hooks/redemptions/__tests__/bulk-delete-contract.test.mjs src/hooks/promo-codes/__tests__/usePromoCodesData-contract.test.mjs && npx eslint src/pages/Benefits src/components/benefits src/components/table/benefits src/components/table/redemptions src/components/table/promo-codes src/hooks/benefits src/hooks/redemptions src/hooks/promo-codes --no-cache && npx prettier --check src/pages/Benefits src/components/benefits src/components/table/benefits src/components/table/redemptions src/components/table/promo-codes src/hooks/benefits src/hooks/redemptions src/hooks/promo-codes src/index.css && npx vite build`

Expected: PASS。

- [ ] **Step 5: 启动本地服务并执行 Playwright 视觉验收**

验证路由：

```text
Default: /benefits, /redemption-codes
Classic: /console/benefits, /console/redemption
```

在 1440x900、768x1024、390x844 下检查用户福利、活动管理、券列表、流水、兑换码、优惠码；
切换 USD/CNY/CUSTOM/TOKENS；执行选择/取消、批量栏、删除确认和空态。截图保存到
`output/playwright/`，检查无重叠和横向溢出。

- [ ] **Step 6: 更新长期文档和工作流记录**

`benefit-vouchers.md` 写明内部 quota/当前展示类型/CNY 实付门槛三种边界、删除 API、状态矩阵、
迁移和回滚。`custom-development.md` 更新稳定性和已知限制。工作流记录实际命令、通过结果、
截图、未验证项和部署边界。

- [ ] **Step 7: 最终差异和安全检查**

Run: `git diff --check`

Expected: PASS。

检查：没有明文凭据、没有生产地址写入前端、没有裸 quota、没有固定人民币提示、没有物理删除
业务记录、没有误改 `relaykit/` 或受保护项目标识。

- [ ] **Step 8: 提交翻译、文档与验收证据**

```bash
git add web/src/i18n web/classic/src/i18n docs/developer docs/workflows/2026-09/05_marketing_benefits_redesign.md output/playwright
git commit -m "docs: 完善营销福利重构验收记录"
```

- [ ] **Step 9: 进行最终代码审查**

按严重程度审查计费换算、迁移幂等、删除并发、优惠码支付回调、用户流水权限、双模板功能一致性、
移动端与暗色主题。所有高/中风险问题修复并重跑相关验证后，才可声明实现完成或准备部署。
