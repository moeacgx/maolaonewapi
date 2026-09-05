# 管理端错误日志显示客户端替换结果

## 问题

上游返回的错误可能包含“余额不足”等敏感或不适合直接展示给终端用户的内容。
客户端响应已经支持按错误消息替换规则改写，但错误日志此前仍直接保存上游原文，管理员无法从“详情”确认用户实际看到的错误是否已经替换。

## 方案

- 错误日志 `content` 保存客户端最终可见的错误内容（包含客户端状态码）。
- 仅当上游错误命中替换规则时，在 `other.upstream_error` 保存脱敏后的上游原始错误。
- 管理员详情同时显示“错误详情”和“上游原始错误”；普通用户接口会在服务端剥离 `other.upstream_error`，不会暴露上游原文。
- 本站预扣费、用户额度等内部错误不进入上游错误替换链，保持原有内部错误语义。
- 历史日志没有 `upstream_error` 时继续按原有 `content` 展示。

## 兼容性与边界

`other.status_code` 继续记录内部错误的原始状态码，便于审计；日志正文中的状态码则反映客户端实际响应状态码。该变更不影响渠道重试、禁用、计费或用户余额判断。

Default 和 Classic 均已有管理员上游原文展示能力；本次后端日志契约对两套模板保持兼容。

## 验证

- 覆盖上游“用户额度不足”命中 `balance`/`额度` 规则时，日志正文为替换文案且保留 `other.upstream_error`。
- 覆盖本站预扣费额度错误不被替换且不生成 `upstream_error`。
- 执行 `go test ./controller -run 'TestRelayErrorLogDisplayContent|TestClientErrorReplacement'` 和 `git diff --check`。
