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

`native v1` 的 SDK 模块表按目标模板分别定义，不能跨模板复用别名或组件：Default 的
`@/components/*`、`@/lib/api` 和 React Query 不属于 Classic 契约；Classic 页面必须只
使用 Classic 宿主公开的模块（例如 `react`、`react/jsx-runtime`、`react-i18next`、
`../../helpers` 及按需的 Semi UI）。每个 `targets.default` 与 `targets.classic` 入口都应
在对应宿主 SDK 下独立加载验证。

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

模型广场折扣展示按 `分组倍率 * (price / usd_exchange_rate)` 计算综合折扣；
`usd_exchange_rate` 失败时回退配置的 `USDExchangeRate`，不直接读取 OKX 模块汇率。

旧值 `okx-alipay-tier` 仍表示 OKPay 内置的 OKX 档位配置路径，与本模块配置分开。

## 对话归档扩展

`conversation-archive` 是仅 Root 可用的补丁模块，用于定位异常对话。配置 API 位于 `/api/extensions/conversation-archive`，请求体在认证后按用户 ID 和稳定分组代码筛选，命中后才进入清洗和持久化。

清洗载荷只保留 `messages[].role` 与纯文本 `messages[].text`，以及模型、协议、请求 ID、用户和分组等必要元数据；媒体、base64、工具 schema、请求头、Cookie、Authorization 和 URL 查询均丢弃。未知协议仅在能提取到有限文本时保存，协议字段保留调用链提供的标识；无可识别文本时跳过。单条消息、消息数和总字节数均有硬上限。OpenAI Realtime 会合并客户端和上游增量文本，忽略音频与完成事件重复正文。

列表接口仅返回元数据，详情接口才返回清洗后的消息。所有接口使用 `RootAuth`、禁缓存和限流，详情按纯文本渲染，防止扩展页面执行 HTML 或再次加载超大 JSON。配置在进程内使用 2 秒 TTL 快照，更新通过版本 CAS 后立即失效本地快照。

Default 使用 Default 原生 SDK 与 `@/lib/api`；Classic 使用 Classic 原生 SDK 与
`../../helpers.API`，并分别维护入口和样式。两套入口不能互相复制宿主组件依赖。

## 兼容性与运维

归档模型使用 GORM 标准字段，正文采用项目的大文本类型封装，兼容 SQLite、MySQL 5.7.8+ 与 PostgreSQL 9.6+。配置了稳定的 `CRYPTO_SECRET` 时正文使用 AES-GCM 加密后入库，详情接口在服务端解密；未配置时保留明确的明文兼容模式。过期记录不会出现在列表或详情接口；主节点每小时按 ID 小批量删除数据库记录。当前实现没有外部对象存储，配置更新使用 `config_version` 乐观锁。升级前确认数据库迁移已执行，回滚时先停用扩展再处理新增表。
