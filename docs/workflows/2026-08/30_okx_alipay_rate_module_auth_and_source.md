# OKX 支付宝汇率模块鉴权、OKPay 汇率源与模型广场折扣修复

## 问题

`okx-alipay-rate` 模块仍在内置模块目录中，但从后台扩展页面打开后无法正常进入或
读取配置。与此同时，OKPay 支付设置已经提供 `okx-alipay-rate-module` 选项，
但后端汇率源归一化没有识别这个新值，保存后会回落到 CoinGecko 路径。

后续核对 v243 行为时发现，模型广场折扣展示必须恢复历史的公开汇率链路。
卡片改版后 Classic 卡片丢失了折扣徽标，同时当前版本缺少 v243 的
`usd_exchange_rate_source`、`usd_exchange_rate_last_updated_at`、
`usd_exchange_rate_is_fallback` 和 `auto_usd_exchange_rate` 状态字段，导致排查时
无法判断模型广场到底使用的是自动公开汇率还是配置兜底。

## 根因

静态 iframe 页面直接请求 `/api/extension-admin/okx-alipay-rate/*`，浏览器资源环境
不会自动带宿主后台的 `Authorization` 头，因此会被 `RootAuth` 拒绝。扩展原生资源
本身也走 `/api/extensions/**` 的后台会话鉴权，动态加载资源时同样需要一个不能被
模块脚本读取的浏览器会话桥接。

OKPay 侧原有后端只识别旧的 `okx-alipay-tier`。新版前端保存
`okx-alipay-rate-module` 后，`normalizeOkpayRateSource` 将其当作未知值处理，最终
退回 `coingecko`。

模型广场的前端公式仍是 v243 的 `groupRatio * (priceRate / usdExchangeRate)`。
这里的 `priceRate` 来自 `/api/status.price`，`usdExchangeRate` 来自
`/api/status.usd_exchange_rate`。243 中 OKX 支付宝模块只影响 OKPay 的 USDT/CNY
支付换算，不直接参与模型广场状态接口的公开汇率。

## 修改范围

- 为扩展资源新增 `new_api_extension` HttpOnly scoped cookie，路径限定在
  `/api/extensions`，由已认证的 `GET /api/extensions/` 发放。
- `UserSessionAuth` 在没有 bearer token 时接受对应路径的 scoped ticket：
  `/canvas` 只认 Canvas ticket，`/api/extensions` 只认扩展资源 ticket；两者仍只
  校验真实后台登录会话，不接受 Dashboard PAT 或 Relay API Key。
- `okx-alipay-rate` 内置模块升级到 `0.3.0`，声明 `native v1` 页面和
  `ui.native` capability，Default 与 Classic 分别提供原生入口和样式。
- OKPay 汇率源识别 `okx-alipay-rate-module`，选择该源时通过
  `extension.FetchEnabledOkxAlipayRateQuote()` 读取模块配置和 OKX 档位价格。
- `/api/status` 恢复 v243 的公开汇率字段：
  `usd_exchange_rate_source`、`usd_exchange_rate_last_updated_at`、
  `usd_exchange_rate_is_fallback` 和 `auto_usd_exchange_rate`。
- `/api/status.price` 保持为 `operation_setting.Price`，模型广场继续用
  `price / usd_exchange_rate` 作为汇率因子。
- `usd_exchange_rate` 在 `auto_usd_exchange_rate` 开启时来自 CoinGecko USDT/CNY，
  获取失败时回退配置的 `USDExchangeRate`。
- Classic 模型广场卡片恢复 v243 语义的综合折扣徽标，仍不在页脚恢复分组文案。
- Default 模型广场卡片同步展示同一综合折扣，详情页既有分组折扣逻辑保持不变。
- 保留旧的 `okx-alipay-tier` 路径，避免已有 OKPay 内置档位配置被破坏。

## 行为边界

`USDExchangeRate` 和 `/api/status.price` 不直接由 `okx-alipay-rate` 模块覆盖。
`USDExchangeRate` 仍是全站美元到人民币展示、账单金额展示和部分统计换算使用的
通用兜底汇率；`price` 仍是普通充值价格率。

当 OKPay 使用模块源时，支付链路为：

1. 订单先按 `OkpayExchangeRate`、用户充值分组倍率和充值折扣算出 CNY 应付金额。
2. 模块返回 OKX 支付宝档位的最终 USDT/CNY 汇率，通常是 6.x 到 7.x 附近。
3. OKPay 用 `CNY 应付金额 / 模块最终 USDT-CNY 汇率` 得到实际 USDT 币数。

模型广场折扣链路保持 v243：

1. `/api/status.price` 返回 `operation_setting.Price`。
2. `/api/status.usd_exchange_rate` 返回公开展示汇率；自动模式下来自 CoinGecko
   USDT/CNY，失败时回退 `USDExchangeRate`。
3. 前端按 `分组倍率 * (price / usd_exchange_rate)` 展示综合折扣。

所以 `okx-alipay-rate` 的 6.x 汇率不会直接改写模型广场徽标。模型广场中
`0.2`、`0.3` 分组倍率在 `price` 与 `usd_exchange_rate` 接近时，应展示为约
`2折`、`3折`。

OKPay 要使用本模块汇率时，必须满足：

- 内置模块 `okx-alipay-rate` 已安装且启用。
- 支付设置的 `OkpayRateSource` 为 `okx-alipay-rate-module`。
- 模块配置中的 OKX 接口地址、方向、档位和调价值通过校验。

## 验证

- 中间件测试覆盖扩展资源 cookie 的签发属性，以及无 bearer token 时使用扩展
  ticket 访问 `/api/extensions/**` 资源；同时覆盖 Canvas 与 Extension ticket
  不能跨路径互相重放。
- 路由测试覆盖 `/api/extensions/` 在认证后发放扩展资源 cookie。
- OKPay 控制器测试覆盖 `okx-alipay-rate-module` 会读取模块配置和档位调价，
  不再回落到 CoinGecko 或旧 OKPay 内置配置。
- 状态接口测试覆盖 v243 汇率字段：`price` 仍保留普通充值价格，
  `usd_exchange_rate` 走公开展示汇率，并返回来源、更新时间、兜底标记和自动开关。
- Classic 卡片源码契约覆盖折扣徽标仍依赖 `getBillingFactors`、
  `getBillingDiscountText` 和 `hasBillingDiscount`。
- Default 定价契约覆盖综合折扣仍由 `price / usd_exchange_rate` 与分组倍率共同
  计算，并确认卡片继续接入 `getBillingCompositeFactor`。
- 扩展注册测试覆盖内置模块刷新到 `0.3.0`，并安装 Default/Classic 原生资源。

已执行：

- `go test ./middleware ./router ./controller ./extension -run "TestIssueExtensionSessionCookieMatchesExtensionResourceContract|TestUserSessionAuthAcceptsLiveExtensionTicket|TestExtensionsRouteServesAuthenticatedSidebarAndEnforcesRootBoundary|TestOkpayRateQuoteUsesOkxAlipayRateModuleConfig|TestGetStatusReturnsV243ExchangeRateContract|TestInstallBuiltinModulesIncludesOkxAlipayRateNativeAssets|TestInstallBuiltinModulesRefreshesOlderBuiltinVersion" -count=1 -timeout 60s`
- 在 `web/classic` 执行
  `node --test src/components/table/model-pricing/billing/utils.test.mjs src/components/table/model-pricing/view/card/__tests__/card-layout-contract.test.mjs`
- `node --check extension/builtin/okx-alipay-rate/public/native/default.mjs`
- `node --check extension/builtin/okx-alipay-rate/public/native/classic.mjs`
- 在 `web` 执行
  `npx --yes oxfmt -c .oxfmtrc.json --check src/features/pricing/components/model-card.tsx src/features/pricing/hooks/use-pricing-data.ts src/features/pricing/lib/__tests__/pricing-contracts.test.ts src/features/auth/types.ts`
- 在 `web` 执行
  `npx --yes bun x oxlint -c .oxlintrc.json src/features/pricing/components/model-card.tsx src/features/pricing/hooks/use-pricing-data.ts src/features/pricing/lib/__tests__/pricing-contracts.test.ts src/features/auth/types.ts`
- `git diff --check`

本地 Default 前端的 Vitest 契约测试未执行成功，原因是 `web/node_modules` 缺失且
`vitest.config.ts` 无法解析 `vitest/config`；本轮未安装依赖。已用格式化、
lint 和源码契约补充覆盖 Default 卡片接线。
