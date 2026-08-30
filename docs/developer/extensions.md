# 扩展模块开发

## 目标

扩展模块用于把可信的一方后台能力挂载到宿主控制台。模块清单、资源安装、
权限校验和宿主 API 调用都必须由 NewAPI 统一收口，避免扩展页面自行保存或读取
用户访问令牌。

## 模块清单

内置和外部模块都通过 `manifest.json` 声明：

- `id` 使用小写字母、数字、`-` 或 `_`。
- `runtime.type` 支持 `static` 或 `http`；原生 UI 只允许 `static`。
- `ui.pages[].path` 必须是模块内绝对路径，不能包含跳转或路径穿越。
- `ui.pages[].render.type = native` 时，必须声明 `sdk = v1`。
- 使用原生 UI 时，`permissions.capabilities` 必须包含 `ui.native`。

## 原生 UI

`native v1` 页面由宿主前端动态加载模块声明的入口和样式资源。入口文件必须默认
导出 React 组件，宿主会注入 `globalThis.__NEW_API_EXTENSION_NATIVE_SDK__`。

Default 前端可使用宿主 SDK 暴露的 `@/lib/api` 客户端；Classic 前端可使用
`../../helpers` 中的 `API`。模块页面应通过这些宿主客户端访问后端接口，继承当前
后台登录态，不应在扩展资源中持久化 bearer token、个人访问令牌或 API Key。

原生资源通过同源 `/api/extensions/{id}/native/{pageKey}/{target}/{asset}` 加载。
浏览器加载这类资源时不一定带 `Authorization` 头，因此宿主会在已认证的扩展列表
请求上发放 `new_api_extension` HttpOnly cookie。该 cookie 仅限
`/api/extensions` 路径，用于资源加载阶段识别当前后台会话。

## OKX 支付宝汇率模块

`okx-alipay-rate` 是内置 Root 模块，用于读取 OKX C2C 支付宝 USDT/CNY 档位，
再按模块配置做固定值或百分比调价。

该模块不会直接覆盖全站 `USDExchangeRate`，也不会改写 `/api/status.price`。
`USDExchangeRate` 仍是站点计价展示和美元转 CNY 的通用兜底汇率；`price` 继续表示
普通充值使用的 `operation_setting.Price`。

OKPay 充值要使用本模块时，支付设置中的 `OkpayRateSource` 必须保存为
`okx-alipay-rate-module`。此时 OKPay 先按 `OkpayExchangeRate` 计算 CNY 应付金额，
再用模块返回的最终 USDT/CNY 汇率换算为实际 USDT 币数创建支付订单。
模块启用状态是 OKPay 模块源缓存契约的一部分；模块被禁用后，新订单必须立即回退到
手动兜底汇率，不能继续使用禁用前缓存的 OKX 报价。

模型广场折扣展示恢复 v243 链路：状态接口返回 `price`、`usd_exchange_rate`、
`usd_exchange_rate_source`、`usd_exchange_rate_last_updated_at`、
`usd_exchange_rate_is_fallback` 和 `auto_usd_exchange_rate`。前端按
`分组倍率 * (price / usd_exchange_rate)` 计算综合折扣。`usd_exchange_rate` 在
`auto_usd_exchange_rate` 开启时来自 CoinGecko 的 USDT/CNY，失败时回退配置的
`USDExchangeRate`；它不直接读取 OKX 支付宝模块汇率。
卡片和详情页只在综合因子低于原价时展示折扣徽标；原价或加价因子不显示折扣徽标。

因此，OKX 模块里的 6.x 是 OKPay 的 USDT/CNY 换算汇率；模型广场的折扣徽标只看
分组倍率、`price` 和公开 `usd_exchange_rate`。当 `price` 与 `usd_exchange_rate`
接近时，`0.2`、`0.3` 分组倍率就分别展示为约 `2折`、`3折`。

旧值 `okx-alipay-tier` 仍表示 OKPay 内置的 OKX 档位配置路径，与本模块配置分开。
