# 渠道启停旧版 PUT 请求兼容

## 问题

`zzapi` 渠道列表仍有前端静态资源发出旧版启停请求：

```http
PUT /api/channel/
{"id":255,"status":1}
```

当前后端通用渠道编辑接口会拒绝任意包含 `status` 的 payload，返回 `common.invalid_params`。新前端已使用专用接口 `POST /api/channel/:id/status`，但旧静态资源或缓存页面仍会触发旧请求。

## 修复

- `UpdateChannel` 仅在 payload 精确为 `id + status` 时走兼容状态更新分支。
- `status + 其他字段` 仍按无效参数拒绝，防止普通渠道编辑绕过专用状态接口。
- 专用接口 `POST /api/channel/:id/status` 与批量接口不变。

## 验证

- `go test ./controller -run 'TestUpdateChannelAcceptsLegacyStatusOnlyPayload|TestUpdateChannelRejectsMixedStatusField|TestChannelHasSensitiveChanges|TestClearChannelReadOnlyFields|TestChannelStatusValidation' -count=1 -timeout 120s`
- `go test ./controller -count=1 -timeout 180s`
