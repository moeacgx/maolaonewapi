# Default 渠道供应商选择与保存恢复

## 变更目标

恢复渠道供应商的选择与保存。当前版本既缺少 Default 渠道抽屉的供应商入口，后端 `Channel` 模型也丢失了 `vendor_id` 字段，导致供应商绑定不能可靠写入，渠道管理按供应商筛选时看不到对应渠道。

## 根因与范围

- v243 的 Default 实现包含 `/api/vendors/` 查询和 `vendor_id` 下拉框。
- 当前 `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx` 在迁移后保留了 `vendor_id` schema、默认值和 payload 转换，却遗漏了供应商查询、选项和 Basic Information 表单控件。
- 后端 `model.Channel` 在核心迁移时误删 `VendorID`，Go JSON 绑定会忽略未知字段，GORM 更新也无法写入供应商列。
- 本次修复 Default 模板（`web/src`）和后端保存链路；Classic 模板已有独立的供应商控件，不在前端改动范围内。

## 实现与契约

- 通过 `getVendors({ page_size: 1000 })` 加载供应商，并在渠道抽屉与渠道列表统一使用 `vendorsQueryKeys` 复用 React Query 缓存。
- 基本信息中恢复 `Vendor` 下拉框；选择供应商时提交其数字 ID，选择 `No vendor` 时提交 `undefined`，由既有 payload 转换为 `null`。
- `transformFormDataToCreatePayload` 和 `transformFormDataToUpdatePayload` 的 `vendor_id` 契约保持不变；后端恢复 `*int` 可空字段和既有索引，兼容历史 NULL 数据，无需新增迁移脚本。
- 更新接口会记录 `vendor_id` 是否显式出现，确保数字值可以绑定、`null` 可以清除，而省略该字段的部分更新继续保留原绑定。
- 模型层继续支持直接修改已加载渠道的 `VendorID` 后调用 `Update()`；非空指针值会被显式写入，避免仅为兼容部分更新而破坏既有调用契约。

## 安全与兼容性

- 供应商名称仅用于管理界面展示，渠道转发逻辑不读取该字段。
- 供应商 ID 仍受表单 `z.number().optional()` 校验；后端按可空整数接收，未知或已删除的供应商不会影响渠道转发。
- 不改变已有渠道的供应商 ID；编辑时由详情接口回填，列表筛选继续使用 `vendor` 查询参数。

## 验证

- 新增前端回归用例，确认选择供应商 ID 后创建渠道 payload 保留该 ID。
- 新增控制器回归用例，确认 `PUT /api/channel/` 写入的供应商 ID 可从数据库重新读取。
- 覆盖 `vendor_id: null` 清除绑定，避免 GORM 默认跳过 nil 导致旧供应商残留。
- 覆盖省略 `vendor_id` 的部分更新继续保留已有供应商绑定。
- 覆盖模型层直接设置非空 `VendorID` 后调用 `Update()` 的持久化行为，再验证显式 `null` 清除绑定。
- 覆盖渠道列表与搜索接口按 `vendor` 筛选、返回供应商计数，以及 `AutoMigrate` 补回缺失列。
- 受限于当前工作环境未安装 Bun 或前端依赖，`bun test`、`bun run typecheck` 和 oxlint 无法执行；交付前应在具备 Bun 的环境运行：

```text
cd web
bun test src/features/channels/lib/__tests__/retained-channel-contracts.test.ts
bun run typecheck
bunx oxlint -c .oxlintrc.json src/features/channels/components/drawers/channel-mutate-drawer.tsx src/features/channels/lib/__tests__/retained-channel-contracts.test.ts
```
