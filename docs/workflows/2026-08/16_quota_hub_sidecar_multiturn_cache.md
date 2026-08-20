# Quota Hub sidecar 多轮对话修复与缓存验证

## 问题与目标

`zzapi.maolaoapi.com` 的 Quota Hub 渠道通过本机 sidecar 将上游 `POST /api/v1/chat` 适配成 OpenAI-compatible `/v1/chat/completions`。旧 sidecar 只从 `messages` 数组倒序取最后一条非空文本发送给上游，导致客户端传完整 OpenAI history 时，上游只能看到最后一个问题，无法使用前文上下文。

本次目标：

- 保持单轮 OpenAI-compatible 请求可用。
- 让多轮 `system/user/assistant` history 在上游单字符串 `message` 中可见。
- 不伪造 prompt cache 命中；上游不返回 cache usage 时，New API 日志继续显示真实 `cache_tokens=0`。
- 通过公网 `zzapi.maolaoapi.com` 验证多轮、重复 prompt 缓存字段和模型计费档位。

## 实现范围

远端 server 52：

- 备份 `/opt/quota_hub_openai_proxy.py` 到 `/opt/quota_hub_openai_proxy.py.bak-20260816-1503`。
- 更新 `/opt/quota_hub_openai_proxy.py`：
  - 新增 role label 渲染：`System`、`User`、`Assistant`、`Developer`、`Tool`。
  - 单条 user 消息保持原文本，避免改变普通单轮 prompt。
  - 多条消息渲染为 transcript，并加前缀：使用完整 transcript 作为上下文回答最后一个 user message。
  - `/v1/responses` 的 `instructions` 与 `input` 同样拼入单条上游 message。
- 重启 Docker sidecar：`quota-hub-openai-proxy`。

不变范围：

- 不改变 New API 主程序、渠道 ID、分组、模型价格、订阅计费和上游 key。
- 不伪造 `prompt_tokens_details.cached_tokens` 或其他 cache usage 字段。
- 不新增持久测试 token；测试 token 已在验证后删除并清理 Redis token cache。

## 行为契约

### Chat Completions

单条 user 消息：

```json
{"messages":[{"role":"user","content":"hello"}]}
```

发送给上游：

```text
hello
```

多轮消息：

```json
{
  "messages": [
    {"role":"user","content":"Remember marker X"},
    {"role":"assistant","content":"READY"},
    {"role":"user","content":"What marker?"}
  ]
}
```

发送给上游：

```text
The following is a multi-turn conversation transcript. Use the full transcript as context and answer the final user message.

User:
Remember marker X

Assistant:
READY

User:
What marker?
```

### Cache

Quota Hub 上游当前 usage 只返回 `input_tokens` 和 `output_tokens`。sidecar 只映射为 OpenAI usage：

```json
{"prompt_tokens": input_tokens, "completion_tokens": output_tokens, "total_tokens": input_tokens + output_tokens}
```

因此 New API 的真实 cache hit 仍以日志 `cache_tokens` 为准。`cache_ratio=0.1` 是计费配置参数，不代表命中。

## 验证

- sidecar 容器健康：`quota-hub-openai-proxy Up`，`/health -> {"ok":true}`。
- 公网多轮 marker：
  - 请求包含三条 history：user 给出 `ZQH-ORCHID-6319`、assistant `READY`、user 追问 marker。
  - 响应：`ZQH-ORCHID-6319`。
  - usage：`prompt_tokens=91`，`completion_tokens=13`。
- 公网重复长 prompt：
  - 第一次：`prompt_tokens=1473`，`completion_tokens=7`，`cache_tokens=0`。
  - 第二次：`prompt_tokens=1473`，`completion_tokens=8`，`cache_tokens=0`。
  - 结论：当前上游没有可见 prompt cache 命中，实际缓存比为 0%。
- 模型计费档位直连上游验证：
  - `claude-sonnet-4-6`：usage `21/7`，实扣 `0.168`，等于 Sonnet 公式 `(21*3 + 7*15)/1000`。
  - `claude-haiku-4-5`：usage `20/7`，实扣 `0.055`，等于 Haiku 公式 `(20*1 + 7*5)/1000`。

## 回滚

如新 transcript 渲染导致兼容性问题，可在 server 52 执行：

```bash
cp /opt/quota_hub_openai_proxy.py.bak-20260816-1503 /opt/quota_hub_openai_proxy.py
docker restart quota-hub-openai-proxy
```

回滚后会恢复旧行为：只发送最后一条消息给 Quota Hub 上游，多轮 history 不可用。
