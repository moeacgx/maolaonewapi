# 使用日志与渠道测试显示上游响应模型

日期：2026-09-04

## 变更目标

渠道映射后的请求模型不一定等于上游实际处理并在响应中声明的模型。管理员在
使用日志和渠道测试中需要看到上游响应返回的模型标识，用于排障和审计；普通
用户不得看到该内部信息。

## 实现契约

- `relaycommon.RelayInfo.UpstreamResponseModelName` 单独保存上游响应声明的模型，
  不覆盖请求阶段的 `UpstreamModelName`。
- OpenAI Chat/Responses（含流式末块、Responses-to-Chat 转换和可选的 compaction
  `model` 字段）、OpenAI 图片、
  Claude 消息（含 `message_start` 和非流式响应）、Gemini `modelVersion`、Ollama
  与 xAI 在解析响应时写入该字段。
- 使用日志 `Other.upstream_response_model_name` 记录非空值；管理员日志接口保留
  原始字段，`formatUserLogs` 在普通用户、令牌日志接口中移除该字段。
- Default 与 Classic 使用日志均增加独立的“上游响应模型”列和详情项，列集合仅对
  管理员日志视图开放；Default 移动端卡片复用同一管理员列。
- 上游响应模型使用独立的青色模型徽标与方向图标强化视觉层级；Classic 长模型名
  通过悬浮提示查看完整值，Default 复用 `ModelBadge` 的供应商图标和复制能力。
- 渠道测试成功响应增加 `upstream_response_model_name`；没有上游模型声明时返回空值，
  不用请求模型或映射模型猜测填充。

## 兼容性与安全边界

- 不新增数据库列或迁移，历史日志继续可读。
- 上游模型字段只在管理员使用日志中展示；普通用户日志的其他计费字段和请求模型
  展示不变。
- 流式响应优先保留已观测到的非空模型，后续不带模型的生命周期事件不会覆盖它。
- 未声明模型的 Responses compaction、任务轮询数据等路径保持空值，不伪造上游标识。

## 验证

```text
go test ./relay/common ./relay/channel/openai ./relay/channel/claude ./relay/channel/gemini ./relay/channel/ollama ./relay/channel/xai ./service ./model ./controller -count=1 -timeout=60s
cd relaykit && GOWORK=off go build ./...
node --test web/classic/src/channel-test-upstream-model.test.mjs web/classic/src/usage-log-upstream-response-model.test.mjs
git diff --check
go test ./... -count=1 -timeout=60s
```

定向回归覆盖：

- OpenAI 普通响应和 Chat/Responses 流式末块采集实际模型；
- Responses 转 Chat、图片路径和 Claude 响应采集模型；
- Ollama 与 xAI 的非流式响应采集模型；
- Gemini 普通、流式、Responses 和原生响应采集 `modelVersion`；
- `GenerateTextOtherInfo` 写入日志字段；
- `formatUserLogs` 删除普通用户不可见字段，同时管理员原始日志不经该整形。

完整 Go 测试若在未构建前端资源的工作树执行，可能因 `web/classic/dist` 缺失而在
根模块初始化阶段失败；各受影响 Go 包仍应单独通过。
