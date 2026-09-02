# Responses WebSocket 支持实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 NewAPI 增加可开关的下游 Responses WebSocket，保持 HTTP/SSE 兼容，补齐 CCS/Codex 能力声明和使用日志传输标记，并为 TokensPro 第二阶段留下稳定合同。

**Architecture:** 在现有 `/v1` Bearer Relay 中增加 `GET /v1/responses` 握手入口。WebSocket 会话按 `response.create`/`response.append` 串行处理，每个 turn 复用现有 Responses 计费、渠道选择和重试链路；第一阶段上游仍使用 HTTP/SSE，并通过事件 sink 转成下游 JSON WebSocket 帧。传输元数据统一写入 `Log.Other`，Default 与 Classic 分别接入同一合同。

**Tech Stack:** Go 1.22、Gin、GORM、Gorilla WebSocket、现有 relay/Responses 转换器、React 19 + TypeScript + TanStack Query、Classic React + Semi UI、Vitest、Node `node:test`。

**Spec:** `docs/superpowers/specs/2026-09-02-responses-websocket-design.md`

## Global Constraints

- `ResponsesWebsocketEnabled` 默认 `false`；关闭时 GET 握手返回可触发客户端 HTTP/SSE 回退的 404/等价响应。
- `POST /v1/responses`、`GET /v1/realtime` 和现有鉴权、限流、提示词审计、分发中间件语义保持不变。
- 下游 WS 使用 JSON 文本帧，首轮为 `response.create`，增量为 `response.append` 或带 `previous_response_id` 的 `response.create`。
- 每个 turn 使用独立 request ID；相同 request ID 不得重复扣费或重复写消费日志。
- 第一阶段上游固定使用现有 HTTP/SSE 适配器；`upstream_transport: "websocket"` 只能由第二阶段 TokensPro 集成产生。
- 所有 JSON 编解码调用使用 `common.Marshal`/`common.Unmarshal` 等项目包装器，不直接调用 `encoding/json` 的编解码函数。
- 后端必须兼容 SQLite、MySQL >= 5.7.8 和 PostgreSQL >= 9.6；行锁使用项目 `lockForUpdate` 约定。
- Default 与 Classic 分别测试和构建；不使用本地假数据代替真实协议验收。
- 每个任务完成后运行该任务列出的测试并提交独立 commit；完成后同步勾选对应 GitHub Issue 项目。

---

### Task 1: 后端运行开关与传输元数据基础

**Files:**
- Modify: `common/constants.go`
- Modify: `model/option.go`
- Modify: `controller/option.go`
- Modify: `relay/common/relay_info.go`
- Modify: `service/log_info_generate.go`
- Modify: `service/text_quota.go`
- Modify: `service/quota.go`
- Test: `model/option_settings_test.go`
- Create: `service/log_info_generate_test.go`

**Interfaces:**
- Produces `common.ResponsesWebsocketEnabled bool` with default `false`.
- Produces `service.AppendTransportInfo(other map[string]interface{}, downstream, upstream string)`.
- `RelayInfo` exposes the downstream transport state needed by all billing log builders.

- [x] **Step 1: Write failing backend tests for the option and metadata contract**

```go
func TestResponsesWebsocketOptionDefaultsToFalse(t *testing.T) {
    require.False(t, common.ResponsesWebsocketEnabled)
}

func TestAppendTransportInfoWritesStableFields(t *testing.T) {
    other := map[string]interface{}{}
    AppendTransportInfo(other, "websocket", "http")
    require.Equal(t, "websocket", other["transport"])
    require.Equal(t, "http", other["upstream_transport"])
}
```

- [x] **Step 2: Run the focused tests and verify they fail**

Run: `go test ./model ./service -run 'ResponsesWebsocketOption|AppendTransportInfo' -count=1 -timeout 60s`

Expected: FAIL because the option and helper do not exist yet.

- [x] **Step 3: Add the option to the common defaults and `/api/option/` load/save flow**

Use the existing `PasswordLoginEnabled`/`LogConsumeEnabled` pattern: initialize `ResponsesWebsocketEnabled=false`, expose it in `model.OptionMap`, parse it as a boolean in `model.UpdateOption`, and include it in the controller's protected-option validation path without changing unrelated options.

- [x] **Step 4: Add one transport metadata helper and wire it into all text/WebSocket log builders**

```go
func AppendTransportInfo(other map[string]interface{}, downstream, upstream string) {
    if other == nil { return }
    if downstream == "" { downstream = "http" }
    if upstream == "" { upstream = "http" }
    other["transport"] = downstream
    other["upstream_transport"] = upstream
}
```

Call it from `GenerateTextOtherInfo` and ensure `GenerateWssOtherInfo` passes `"websocket"` for the downstream transport. The new Responses WebSocket turn will set `RelayInfo` to downstream `websocket` while retaining upstream `http`.

- [x] **Step 5: Run tests, format, and commit**

Run: `go test ./model ./service -run 'ResponsesWebsocketOption|AppendTransportInfo|LogType' -count=1 -timeout 60s`; `gofmt -w common/constants.go model/option.go controller/option.go relay/common/relay_info.go service/log_info_generate.go service/text_quota.go service/quota.go model/option_settings_test.go service/log_info_generate_test.go`; `git diff --check`.

Commit: `feat: add Responses WebSocket option and transport metadata`.

### Task 2: Responses WebSocket message normalization and session state

**Files:**
- Create: `relay/responses_websocket_session.go`
- Test: `relay/responses_websocket_session_test.go`

**Interfaces:**
- `type ResponsesWebsocketState struct { Model string; LastRequest []byte; LastResponseID string; LastResponseOutput []byte; PendingToolCallIDs []string }`.
- `func NormalizeResponsesWebsocketRequest(raw []byte, state ResponsesWebsocketState) (request []byte, next ResponsesWebsocketState, err error)`.
- `func ResponsesWebsocketErrorPayload(code, message string) []byte`.

- [ ] **Step 1: Write failing table tests for create, append, previous response and full transcript replacement**

```go
func TestNormalizeResponsesWebsocketRequest(t *testing.T) {
    tests := []struct { name, raw string; state ResponsesWebsocketState; wantModel, wantInput string }{
        {"create", `{"type":"response.create","model":"gpt-test","input":[{"role":"user","content":"hi"}]}`, ResponsesWebsocketState{}, "gpt-test", "hi"},
        {"append", `{"type":"response.append","input":[{"role":"user","content":"next"}]}`, ResponsesWebsocketState{Model:"gpt-test", LastRequest: []byte(`{"model":"gpt-test","input":[]}`)}, "gpt-test", "next"},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            request, _, err := NormalizeResponsesWebsocketRequest([]byte(tc.raw), tc.state)
            require.NoError(t, err)
            var got struct { Model string `json:"model"`; Input []map[string]any `json:"input"` }
            require.NoError(t, common.Unmarshal(request, &got))
            require.Equal(t, tc.wantModel, got.Model)
            require.NotEmpty(t, got.Input)
        })
    }
}
```

Add separate assertions that a mismatched `previous_response_id` returns code `previous_response_not_found` and that a full transcript does not duplicate the saved output.

- [ ] **Step 2: Run the session tests and verify they fail**

Run: `go test ./relay -run 'ResponsesWebsocket' -count=1 -timeout 60s`

Expected: FAIL because the state type and normalizer are absent.

- [ ] **Step 3: Implement event parsing with project JSON wrappers**

Parse `type`, `model`, `input`, and `previous_response_id` using a typed envelope plus `common.Unmarshal`. For the first `response.create`, require a non-empty model and normalize missing input to `[]`. For append, merge the new input with the saved request/output while preserving pending tool-call pairs. For complete transcript input, replace the old transcript instead of appending it.

- [ ] **Step 4: Implement stable error payloads and bounded state updates**

Return a JSON object shaped as `{"type":"error","error":{"code":"invalid_request","message":"request is invalid"}}`. Reject unknown event types, non-array append input, and oversized state before it reaches the relay pipeline. Never include raw credentials or request bodies in error messages.

- [ ] **Step 5: Run tests, format, and commit**

Run: `go test ./relay -run 'ResponsesWebsocket' -count=1 -timeout 60s`; `gofmt -w relay/responses_websocket_session.go relay/responses_websocket_session_test.go`; `git diff --check`.

Commit: `feat: normalize Responses WebSocket turns`.

### Task 3: HTTP/SSE-to-WebSocket event sink

**Files:**
- Create: `relay/responses_websocket_sink.go`
- Test: `relay/responses_websocket_sink_test.go`
- Modify: `relay/responses_handler.go`

**Interfaces:**
- `type ResponsesWebsocketSink interface { WriteEvent([]byte) error; WriteError(*types.NewAPIError) error; MarkTerminal() bool }`.
- `func NewResponsesWebsocketSink(conn *websocket.Conn) ResponsesWebsocketSink`.
- Existing HTTP Responses output remains on its current writer path.

- [ ] **Step 1: Write failing sink tests for SSE data, `[DONE]`, duplicate terminal and errors**

Use a local Gorilla WebSocket test server. Feed `data: {"type":"response.output_text.delta"}\n\n` and assert the peer receives one JSON text frame. Feed `[DONE]` after `response.completed` and assert no second terminal frame. Feed a non-JSON data line and assert a protocol error rather than raw SSE leakage.

- [ ] **Step 2: Run the sink tests and verify they fail**

Run: `go test ./relay -run 'ResponsesWebsocketSink' -count=1 -timeout 60s`

Expected: FAIL because the sink is absent.

- [ ] **Step 3: Implement a serialized sink around one WebSocket connection**

Guard writes with a mutex. Parse SSE boundaries, strip the `data:` prefix, pass JSON event payloads through unchanged, map `[DONE]` to one synthetic `response.done` only when no terminal event has been sent, and map HTTP/relay errors to the stable `error` event payload.

- [ ] **Step 4: Expose the sink at the Responses handler boundary without changing POST behavior**

Refactor only the output writer dependency needed by `ResponsesHelper`; keep request conversion, parameter overrides, disabled-field removal, usage extraction, and existing HTTP response writes unchanged for `POST /v1/responses`.

- [ ] **Step 5: Run tests, format, and commit**

Run: `go test ./relay -run 'ResponsesWebsocketSink|Responses' -count=1 -timeout 60s`; `gofmt -w relay/responses_websocket_sink.go relay/responses_websocket_sink_test.go relay/responses_handler.go`; `git diff --check`.

Commit: `feat: bridge Responses SSE events to WebSocket frames`.

### Task 4: WebSocket controller lifecycle, billing and retry integration

**Files:**
- Create: `controller/responses_websocket.go`
- Modify: `controller/relay.go`
- Modify: `relay/common/relay_info.go`
- Modify: `service/text_quota.go`
- Test: `controller/responses_websocket_test.go`

**Interfaces:**
- `func ResponsesWebsocket(c *gin.Context)` handles one upgraded connection.
- `func runResponsesWebsocketTurn(c *gin.Context, frame []byte, state relay.ResponsesWebsocketState, sink relay.ResponsesWebsocketSink) (relay.ResponsesWebsocketState, error)` processes one serialized turn.

- [ ] **Step 1: Write failing controller tests for disabled/enabled handshake, auth and one complete turn**

Build a Gin test router with the existing middleware and a local HTTP upstream fixture. Assert disabled GET returns 404 without Upgrade; enabled GET requires Bearer auth, accepts `response.create`, forwards `stream=true` upstream, and emits a JSON `response.completed` frame.

- [ ] **Step 2: Run the controller tests and verify they fail**

Run: `go test ./controller -run 'ResponsesWebsocket' -count=1 -timeout 60s`

Expected: FAIL because the controller and route integration are absent.

- [ ] **Step 3: Implement connection upgrade and serialized read loop**

Check `common.ResponsesWebsocketEnabled` before upgrade. Reuse the existing Bearer route middleware. Read text/binary JSON frames, normalize each turn with Task 2, and send all writes through Task 3's sink. Reject concurrent turn execution by processing one frame at a time.

- [ ] **Step 4: Reuse the existing relay billing/retry pipeline per turn**

Extract the shared request setup from `controller.Relay` into an internal function that accepts a prepared Responses request and an output sink. Preserve model mapping, `PreConsumeBilling`, channel retry, `PostTextConsumeQuota`, failure refund, channel metrics, request archive and prompt audit. Set a fresh turn request ID and mark `RelayInfo` downstream transport as `websocket`.

- [ ] **Step 5: Implement disconnect and partial-output behavior**

Create a child context canceled when the client read loop exits. Before the first output event, allow existing retry logic. After any output event, do not replay automatically; send one error/close payload and release the turn. Ensure all goroutines, response bodies and channel slots are released on every return path.

- [ ] **Step 6: Add billing/idempotency regression tests, then format and commit**

Assert duplicate turn request IDs write one consume log and apply one quota change; assert client disconnect refunds pre-consumed quota; assert two sequential turns preserve response state but create independent request IDs. Run: `go test ./controller ./service ./relay -run 'ResponsesWebsocket|Billing|Idempot' -count=1 -timeout 60s`; `gofmt -w controller/responses_websocket.go controller/relay.go relay/common/relay_info.go service/text_quota.go controller/responses_websocket_test.go`; `git diff --check`.

Commit: `feat: serve Responses over downstream WebSocket`.

### Task 5: Router and backend contract verification

**Files:**
- Modify: `router/relay-router.go`
- Create: `router/relay_router_websocket_test.go`
- Test: `controller/responses_websocket_test.go`

**Interfaces:**
- Route contract: `GET /v1/responses` invokes `controller.ResponsesWebsocket`; `POST /v1/responses` remains unchanged.

- [ ] **Step 1: Add route and middleware order regression tests**

Assert the route is under `/v1`, has `RouteTag("relay")`, system performance check, `TokenAuth`, model rate limit, prompt audit and distribution middleware, and does not register a second `/v1/responses` POST handler.

- [ ] **Step 2: Implement the GET registration**

Add the GET handler beside the existing POST response route. Keep `/realtime` in its existing WebSocket group and do not reuse `RelayFormatOpenAIRealtime` for Responses frames.

- [ ] **Step 3: Verify HTTP fallback and error status behavior**

Run the route tests with the option false and true. Confirm disabled GET returns 404/compatibility response, invalid Bearer returns 401 before Upgrade, malformed frames return WS `error`, and POST responses still pass the existing Responses test suite.

- [ ] **Step 4: Run backend verification and commit**

Run: `go test ./router ./controller ./relay ./service ./model -count=1 -timeout 60s`; `gofmt -w router/relay-router.go router/relay_router_websocket_test.go`; `git diff --check`.

Commit: `test: verify Responses WebSocket route contract`.

### Task 6: Default CCS/Codex import and usage-log UI

**Files:**
- Modify: `web/src/features/keys/lib/cc-switch.ts`
- Modify: `web/src/features/keys/lib/__tests__/cc-switch.test.ts`
- Modify: `web/src/features/usage-logs/types.ts`
- Modify: `web/src/features/usage-logs/lib/format.ts`
- Modify: `web/src/features/usage-logs/components/columns/common-logs-columns.tsx`
- Modify: `web/src/features/usage-logs/components/usage-logs-mobile-card.tsx`
- Modify: `web/src/i18n/static-keys.ts`
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/vi.json`
- Modify: `web/src/features/system-settings/types.ts`
- Modify: `web/src/features/system-settings/content/index.tsx`
- Modify: `web/src/features/system-settings/content/section-registry.tsx`
- Modify: `web/src/features/system-settings/content/chat-settings-section.tsx`
- Test: `web/src/features/usage-logs/components/__tests__/transport-label.test.tsx`

**Interfaces:**
- `buildCCSwitchURL({ app: 'codex', ... })` includes `supports_websockets=true`; Claude/Gemini URLs do not.
- `LogOtherData.transport?: 'websocket' | 'http'`; missing values render the translated unknown state.

- [ ] **Step 1: Add failing tests for Codex-only URL parameter and three log states**

```ts
const base = { name: 'Test', models: { model: 'gpt-test' }, apiKey: 'sk-test', origin: 'https://example.test' }
expect(new URL(buildCCSwitchURL({ ...base, app: 'codex' })).searchParams.get('supports_websockets')).toBe('true')
expect(new URL(buildCCSwitchURL({ ...base, app: 'claude' })).searchParams.has('supports_websockets')).toBe(false)
```

Render a usage-log row with `transport` equal to `websocket`, `http`, and absent, and assert the corresponding labels.

- [ ] **Step 2: Run Default focused tests and verify they fail**

Run from `web/`: `node ./node_modules/vitest/vitest.mjs run src/features/keys/lib/__tests__/cc-switch.test.ts src/features/usage-logs/components/__tests__/transport-label.test.tsx`

Expected: FAIL because the URL parameter and label renderer are absent.

- [ ] **Step 3: Add the Codex parameter and typed log parsing**

Set `supports_websockets=true` only in the `app === 'codex'` branch of `buildCCSwitchURL`. Extend `LogOtherData` parsing to accept only the three known transport states; unknown values map to the unknown display state without leaking raw codes.

- [ ] **Step 4: Add the usage-log label to desktop and mobile details**

Use existing `useTranslation()` and status-badge conventions. Keep labels compact and visible in the request detail/summary without exposing administrator-only metadata to ordinary users.

- [ ] **Step 5: Add the Default system-settings switch**

Extend `ContentSettings` defaults, option resolution, section registry and `ChatSettingsSection` with `ResponsesWebsocketEnabled`, submitting it through the existing `/api/option/` mutation. Default UI state is off when the option is absent.

- [ ] **Step 6: Run tests, typecheck, lint, format, and commit**

Run from `web/`: `node ./node_modules/vitest/vitest.mjs run src/features/keys src/features/usage-logs`; `tsgo -b`; `oxlint -c .oxlintrc.json src/features/keys src/features/usage-logs src/features/system-settings`; `oxfmt --check src/features/keys/lib/cc-switch.ts src/features/keys/lib/__tests__/cc-switch.test.ts src/features/usage-logs/types.ts src/features/usage-logs/lib/format.ts src/features/usage-logs/components/columns/common-logs-columns.tsx src/features/usage-logs/components/usage-logs-mobile-card.tsx src/features/system-settings/types.ts src/features/system-settings/content/index.tsx src/features/system-settings/content/section-registry.tsx src/features/system-settings/content/chat-settings-section.tsx`; `git diff --check`.

Commit: `feat(default): expose Responses WebSocket capability and transport labels`.

### Task 7: Classic CCS/Codex import and usage-log UI

**Files:**
- Modify: `web/classic/src/helpers/ccSwitch.js`
- Modify: `web/classic/src/helpers/ccSwitch.test.mjs`
- Modify: `web/classic/src/hooks/usage-logs/useUsageLogsData.jsx`
- Modify: `web/classic/src/components/table/usage-logs/UsageLogsColumnDefs.jsx`
- Modify: `web/classic/src/components/table/usage-logs/UsageLogsTable.jsx`
- Modify: `web/classic/src/components/settings/SystemSetting.jsx`
- Modify: `web/classic/src/i18n/locales/en.json`
- Modify: `web/classic/src/i18n/locales/zh-CN.json`
- Modify: `web/classic/src/i18n/locales/zh-TW.json`
- Modify: `web/classic/src/i18n/locales/fr.json`
- Modify: `web/classic/src/i18n/locales/ru.json`
- Modify: `web/classic/src/i18n/locales/ja.json`
- Modify: `web/classic/src/i18n/locales/vi.json`
- Test: `web/classic/src/cc-switch-integration.test.mjs`
- Test: `web/classic/src/transport-log-label.test.mjs`

**Interfaces:**
- Classic `buildCCSwitchURL` has the same Codex-only `supports_websockets=true` rule as Default.
- Classic log rendering consumes the shared backend `other.transport` field and shows websocket/http/unknown.

- [ ] **Step 1: Add failing native Node tests for URL and log labels**

Assert the Codex URL query parameter, the absence of that parameter for Claude/Gemini, and the three transport display states in the existing Classic log presenter.

- [ ] **Step 2: Run Classic tests and verify they fail**

Run from `web/classic/`: `node --test src/cc-switch-integration.test.mjs src/transport-log-label.test.mjs`

Expected: FAIL because the URL parameter and transport presenter are absent.

- [ ] **Step 3: Implement the URL and log label changes**

Keep Semi UI table and mobile layout structure intact. Use existing `useTranslation()` calls and the Classic status/text conventions; never render a raw unknown backend transport value.

- [ ] **Step 4: Add the Classic system-settings checkbox**

Normalize `ResponsesWebsocketEnabled` through the existing `toBoolean` path, include it in `inputs`/`originInputs`, and submit changes through `updateOptions`. Preserve all existing settings save ordering and error handling.

- [ ] **Step 5: Run tests, lint, format, build, and commit**

Run from `web/classic/`: `node --test src/cc-switch-integration.test.mjs src/transport-log-label.test.mjs`; `eslint src/helpers/ccSwitch.js src/helpers/ccSwitch.test.mjs src/hooks/usage-logs/useUsageLogsData.jsx src/components/table/usage-logs/UsageLogsColumnDefs.jsx src/components/table/usage-logs/UsageLogsTable.jsx src/components/settings/SystemSetting.jsx`; `prettier --check src/helpers/ccSwitch.js src/helpers/ccSwitch.test.mjs src/hooks/usage-logs/useUsageLogsData.jsx src/components/table/usage-logs/UsageLogsColumnDefs.jsx src/components/table/usage-logs/UsageLogsTable.jsx src/components/settings/SystemSetting.jsx`; `vite build`; `git diff --check`.

Commit: `feat(classic): expose Responses WebSocket capability and transport labels`.

### Task 8: Documentation, integration checklist and zzapi acceptance

**Files:**
- Modify: `docs/developer/custom-development.md`
- Modify: `docs/developer/README.md`
- Create: `docs/workflows/2026-09/02_responses_websocket_phase1.md`
- Modify: `docs/superpowers/specs/2026-09-02-responses-websocket-design.md` (set status and record final contract)
- Modify: `docs/superpowers/plans/2026-09-02-responses-websocket.md` (check completed tasks)

**Interfaces:**
- Developer docs record the option name, GET/POST routes, frame types, log fields, fallback and rollback.
- Workflow records commit SHAs, tests, image/tag, zzapi service-by-service evidence and skipped TokensPro validation.

- [ ] **Step 1: Add failing documentation checks**

Check that `docs/developer/README.md` links the design and plan, `custom-development.md` lists the capability, and the workflow includes the exact `ResponsesWebsocketEnabled`, `transport`, `upstream_transport`, `response.create`, `response.append` and `GET /v1/responses` strings.

- [ ] **Step 2: Update long-term developer documentation**

Document that the first phase accepts downstream WS but uses HTTP/SSE upstream, that disabling the option preserves POST fallback, and that TokensPro upstream WS is a separate phase.

- [ ] **Step 3: Record local verification and release boundaries**

Write the workflow with backend/frontend test commands, known environment gaps, default-off rollout, rollback by option or image, and explicit separation between local protocol tests and live deployment.

- [ ] **Step 4: Perform zzapi-only live acceptance after an explicitly authorized release**

Use CloudSSH against the already-audited zzapi target (`serverId=52`, `/home/docker/zzapi`) only after confirming image/tag and service scope. Verify all three application services one at a time for health, restart count, `/api/status`, disabled fallback, enabled handshake, one-turn and multi-turn Codex frames, HTTP/SSE fallback and log transport labels. Do not touch maolaoapi in this task.

- [ ] **Step 5: Check all documents and commit**

Run: `git diff --check`; repository Markdown/link checks available in the project; verify no protected identifiers changed. Commit: `docs: record Responses WebSocket phase one contract and zzapi acceptance`.

### Task 9: TokensPro phase-two handoff (separate repository/Issue)

**Files:**
- No implementation files in this repository during Phase 1.
- Reference: `docs/superpowers/specs/2026-09-02-responses-websocket-design.md`

**Interfaces:**
- TokensPro connects to NewAPI `GET /v1/responses` with the same Bearer and event-frame contract.
- Only after a verified upstream WS connection does `upstream_transport` become `"websocket"`.

- [ ] **Step 1: Confirm NewAPI Phase 1 acceptance evidence is attached to the TokensPro Issue**

Include the final NewAPI commit SHA, route contract, frame examples, fallback behavior, log fields, and zzapi evidence without including tokens, cookies or credentials.

- [ ] **Step 2: Create the TokensPro implementation plan in its own repository**

Split TokensPro work into client handshake, upstream WS adapter, reconnect/failover, usage propagation, and integration tests; do not modify this repository to simulate the second phase.

- [ ] **Step 3: Keep Phase 2 unchecked until real TokensPro integration passes**

The main NewAPI Issue may be closed for Phase 1 only after Tasks 1-8 pass. The TokensPro follow-up Issue remains open until both projects demonstrate the full WS chain.

## Final Verification Checklist

- [ ] Backend: `go test ./... -count=1 -timeout 60s` (including root embed prerequisites)
- [ ] Backend: SQLite lifecycle/ billing/ disconnect/ idempotency tests pass
- [ ] Default: benefits/keys/usage-logs/system-settings Vitest, `tsgo -b`, scoped `oxlint`, `oxfmt --check`, Rsbuild build
- [ ] Classic: native contract tests, scoped ESLint, Prettier, Vite build
- [ ] Documentation: README links, custom-development registration, workflow and plan checkboxes synchronized
- [ ] zzapi: authorized live acceptance with exact image/tag, health, restart count, API status and WS/HTTP evidence
- [ ] TokensPro: separate Issue and integration evidence, not claimed by Phase 1
