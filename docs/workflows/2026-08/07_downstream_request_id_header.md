# Downstream Request ID Header

## Goal

Forward the current NewAPI request ID to OpenAI-compatible upstream requests with the `X-Downstream-Request-ID` header, so an upstream aggregator can persist the caller-side request identity separately from its own upstream request ID.

## Scope

- Applies to OpenAI-compatible relay formats and routes, including `/v1/chat/completions` and `/v1/responses`.
- Applies in the shared upstream request construction path after channel header overrides and before sending the HTTP request.
- The header value is strictly `c.GetString(common.RequestIdKey)`.
- If the context value is empty, `X-Downstream-Request-ID` is removed from the upstream request.

## Non-goals

- Does not change the client-facing `X-Oneapi-Request-Id` response header.
- Does not change existing `upstream_request_id` capture from upstream response headers.
- Does not trust client-provided `X-Downstream-Request-ID`, client `X-Oneapi-Request-Id`, IP, time, body content, or upstream response IDs.
- Does not authorize deployment or channel configuration changes.

## Implementation Notes

`relay/channel/api_request.go` now centralizes the logic in `applyDownstreamRequestIDHeader`. It first deletes any existing `X-Downstream-Request-ID` value created by wildcard passthrough, static channel header override, or client spoofing, then sets the header only when the Gin context request ID is non-empty.

This ordering keeps `header_override` behavior intact for ordinary headers while making `X-Downstream-Request-ID` a protected relay-derived header.

## Verification

Focused regression tests in `relay/channel/api_request_test.go` cover:

- `/v1/chat/completions` non-stream.
- `/v1/chat/completions` stream.
- `/v1/responses` non-stream.
- `/v1/responses` stream.
- Empty context request ID with client wildcard passthrough.
- Context request ID overriding a static channel `X-Downstream-Request-ID` override.

Command used:

```powershell
go test ./relay/channel
```
