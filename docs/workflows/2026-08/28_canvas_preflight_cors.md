# Canvas 预检跨域失败修复

## 问题

升级并启用负载均衡后，外部 Infinite Canvas 通过 NewAPI 直登入口调用失败，浏览器提示未收到响应。`GET /canvas/v1/models` 正常，但浏览器发出的 `OPTIONS /canvas/v1/chat/completions` 返回了前端首页 HTML，未返回 Canvas 所需的 CORS 响应头，因此真正的 `POST` 没有发出。

## 根因

Canvas 路由组注册了 GET/POST 处理器和 `CanvasOriginGuard`，但全局预检边界没有把 `/canvas/**` 识别为 Canvas 路径。Gin 对没有 OPTIONS 路由的请求进入 Web fallback，最终返回首页 HTML。与此同时，Canvas 预检声明的 `x-api-key` 不在允许请求头列表中。

## 修复

- 全局 CORS 预检分派优先将 `/canvas` 和 `/canvas/**` 交给 `CanvasOriginGuard`。
- Canvas 继续使用精确的配置来源校验，允许凭据，不使用 `*`。
- 允许头列表补齐 `X-API-Key`，兼容 Canvas 浏览器请求。
- 不改变 Canvas 的会话鉴权、分组注入、计费和上游分流逻辑。

## 验证

- 路由回归测试覆盖正确来源返回 `204 No Content`、精确 `Access-Control-Allow-Origin`、凭据和 `X-API-Key`，以及错误来源被拒绝。
- 生产验证应确认：`OPTIONS /canvas/v1/chat/completions` 返回 `204`，随后实际 `POST` 能收到流式响应。

## 兼容性与发布

该修复只影响 `/canvas/**` 的浏览器预检响应，不影响普通 `/v1/**` API。发布到 zzapi 时按现有负载均衡滚动更新 worker，再更新 master；每次更新后确认实例健康和 Canvas 预检结果，再继续下一个节点。
