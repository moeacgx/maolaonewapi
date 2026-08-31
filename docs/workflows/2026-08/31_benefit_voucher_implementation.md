# 福利营销时效额度券实现记录

## 目标与范围

依据 GitHub Issue #77 的功能口径实现绑定分组的一次性时效额度券，忽略 Issue 中的
历史开发进度描述。本次范围包含后端数据模型、活动/领取接口、福利券组合计费、分组
单用户并发、使用日志和报表字段、Default/Classic 页面与基础 i18n。

明确边界：只修改当前分支代码、测试和开发文档；不部署 `dev.nu11.me`，不修改
`maolaoapi` 生产环境，不创建 PR。

## 关键实现

- 新增活动、预拆份额、用户券和流水模型，使用整数分和有符号 64 位 quota。
- 活动状态机支持草稿、发布、暂停、恢复、提前结束、两种强终止；发布后冻结关键字段。
- 领取事务锁定活动和份额，唯一索引保证同用户同活动一张券，最后一份竞争返回“已领完”。
- 计费会话加入福利券 -> 订阅 -> 钱包组合资金源，支持追加预扣、正负差额、失败回滚和
  同 request ID 幂等；零差额也会完成福利券结算流水。后续来源失败时，福利券使用独立
  `settle_rollback` 流水和原 `request_id`/类型组合键补偿，避免原 `settle_delta`
  判断吞掉反向操作；预扣回滚错误通过 `errors.Join` 返回并写入系统日志。
- 追加预扣退款写入 `refund_additional` 流水，记录退款后的预扣余额版本；重复退款不会
  再次增加券余额，多轮追加后仍可继续退款并对账。
- 福利券只在 token 或请求明确选择稳定分组时启用，继承用户分组和 `auto` 不会误扣。
- 普通标准 Relay JSON 请求的 `group` 会写入最终 `ModelRequest` 和 `using_group`，并执行
  用户可用分组、token 显式绑定校验；省略 `group` 继续继承，`auto` 保持自动门禁。
- `terminate_mode=unused` 的已领取券在活动 `terminated` 后仍可追加预扣至活动 `ends_at`
  或券 `expires_at`；`terminate_mode=all` 后报表以 `original_quota - used_quota` 统计
  已领取未用额度，即使终止动作已将 `remaining_quota` 清零。
- 管理 HTTP 接口忽略请求体 `now`，状态和审计时间统一由服务端时钟生成；模型层显式时间
  参数仅供内部测试和事务调用，不属于生产请求契约。
- `BillingBreakdown` 及消费日志同时保存 `activity_id`、`voucher_id`、`request_id` 和
  `log_id`，确保活动、券、请求和日志可追溯。
- `groups.single_user_concurrency_limit` 贯通后端、Default 和 Classic 分组设置。
- 两套前端提供活动管理、用户券详情、钱包摘要、报表、券流水和危险操作确认。
- Classic 活动表单补齐个人有效期、北京时间起止时间和随机预算范围校验。

## 本轮阻断修复验证记录

- SQLite 真实账务回归覆盖：福利券预扣、后续来源失败、独立结算补偿流水、最终退款和余额/已用额度一致性。
- 追加预扣覆盖 `terminated + terminate_mode=unused` 的 ends_at/券 expires_at 边界。
- 报表覆盖 `terminate_mode=all` 清零后的 original-used 统计。
- 标准 `/v1/chat/completions` JSON 显式分组、`auto`、继承和 token 绑定拒绝均有回归测试。
- 管理终止接口覆盖 body 伪造 `now` 被忽略；模型调用仍支持测试时钟注入。

## 验证记录

- Go 定向测试覆盖活动模型、接口、组合计费、追加预扣、零差额结算和显式分组门禁。
- Default 已执行 TypeScript typecheck；相关 Vitest 用例通过。
- Classic 福利 API/表单/构建契约测试、受影响 ESLint/Prettier 检查和生产构建通过。
- 未升级依赖版本；入口改用 Semi UI 已导出的基础样式路径，并将 LinkedIn/Slack 图标切换为
  当前 `react-icons` 实际导出的组件，清除既有构建阻断。
- `git diff --check` 作为最终收口检查。

## 回滚与软关闭

回滚实现时保留福利表和分组字段，旧版本会忽略它们。运营侧不发布新活动即可软关闭，
已有活动可暂停或终止。若发现组合计费异常，应先暂停活动，再依据 request/ledger 对账，
不能直接删除流水或用户券。
