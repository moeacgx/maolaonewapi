# 图片模型误用 Chat/Responses 端点的自动路由

## 问题

部分 OpenAI SDK 会把图片模型请求发送到 `/v1/chat/completions` 或
`/v1/responses`。网关按路径把这类请求当成文本请求，导致图片渠道进入错误的
转换链路，最终请求失败或被上游拒绝。

本次线上复盘还发现，截图中的第一次“无可用渠道”发生在新建渠道后、渠道缓存
下一次同步前的短暂窗口；随后请求已选中该渠道，但 Playground 路径未进入图片
转换链路，最终由上游返回 `Responses output cannot be represented by the requested
format`。

后续回归发现，旧判定只要全局模型端点缓存包含 `image-generation` 就会改写路径。
模型管理中的 `ModelMeta.Endpoints` 会覆盖默认端点，而该缓存按模型跨渠道聚合；
因此 `gpt-5.5-openai-compact` 一旦被错误声明为图片端点，就会从
`/pg/chat/completions` 改写到 `/pg/images/generations`，再经渠道模型映射变成
`gpt-5.5`，最终被上游以 `images endpoint requires an image model` 拒绝。

## 修改范围

- 仅处理 JSON 请求，且请求格式为 OpenAI Chat 或 OpenAI Responses。
- 非 Codex 渠道上的内置图片模型继续启用自动路由，不依赖模型元数据。
- 自定义模型只有在有效端点集合唯一且为 `image-generation` 时才启用自动路由；
  只要同时声明任意其他端点，就保持客户端原路径，不猜测本次调用意图。
- 模型名以 `-openai-compact` 结尾，或当前已选渠道为 Codex 时，禁止自动改写。
- `/v1/chat/completions`、`/v1/responses` 在网关内部改写为
  `/v1/images/generations`，不向客户端发送 HTTP 重定向。
- Responses 的 `input`、Chat 的 `messages`、以及 `instructions` 会提取文本写入
  图片请求的 `prompt`；已有非空 `prompt` 时保持原值。
- Playground 的 `/pg/chat/completions` 改写为 `/pg/images/generations`；Canvas 的
  `/canvas/v1/chat/completions` 同样改写为
  `/canvas/v1/images/generations`。Responses compact、普通文本模型和非 JSON
  请求保持原行为。
- 客户端原本请求图片端点时 Relay 格式已经是图片格式，不进入自动改写判定。
- Classic 与 Default 模型管理提示明确说明端点配置的有限路由影响，以及 Codex、
  `openai-compact` 和自定义多端点模型的排除规则。

## 兼容性与安全边界

- 请求体仍由统一的 `common.BodyStorage` 管理，改写后会重建可重复读取的请求体，
  不影响重试和上游请求体透传。
- 不修改模型名、计费模型或首次渠道选择逻辑；自动路由发生在渠道分配后的 Relay
  请求校验前，因此可以读取当前选中渠道类型。只有自动改写后的后续重试增加
  Codex 类型排除，图片计费与图片响应处理保持原链路。
- 不新增数据库字段或独立开关。模型端点元数据仍是跨渠道的模型级信息，因此只把
  “唯一图片端点”视为非内置图片模型的自动路由信号；自定义多端点声明不会替
  客户端选择接口。
- 自动改写出的图片请求在跨渠道重试时会排除 Codex 渠道类型；选择器跳过不适用
  渠道后继续寻找同模型的下一条候选，不会把已改写的图片请求送入 Codex。渠道
  ID 与渠道类型在模型选渠层一次性取并集过滤，不占用仅用于防止自引用渠道循环的
  64 次守卫额度；内存缓存和数据库选渠路径遵循相同规则。
- 无法解析的 JSON 不会被自动改写，继续由原有请求校验返回错误。
- 不对 `/v1/responses/compact`、Realtime、multipart 图片编辑或其他非图片模型做
  猜测性转换。

## 测试计划与结果

- 单元测试覆盖 Chat、Responses `input`、上下文模型回退、Playground/Canvas 路径、
  文本模型和原生图片端点保持不变，以及 Responses compact 排除。
- 纯函数测试覆盖自定义纯图片端点、多端点、`gpt-5.5-openai-compact`、Codex
  渠道、内置图片模型和自动改写后的重试排除类型。
- 渠道选择测试覆盖高优先级 Codex 被排除后继续选择低优先级 OpenAI 渠道、
  65 个高优先级 Codex 的守卫边界、渠道 ID 与类型组合排除，以及内存缓存和
  数据库两条选渠路径；非零重试时两条路径保持相同的原始优先级索引语义。
- 服务层测试覆盖显式多分组和 `auto` 跨组选择、`auto` 跨组重试开关的两种状态，
  并分别验证 `auto` 的底层选渠错误和显式多分组的自引用守卫错误不会被跨组逻辑
  吞掉。
- 执行
  `go test ./controller -run 'TestAutoRouteImageRequest|TestImageGenerationPath|TestShouldAutoRouteImageModel|TestImageAutoRouteExcludedChannelTypes'`
  以及
  `go test ./service -run 'TestChannelTypeExclusionSkipsHigherPriorityChannel'`。
- 上述 Go 定向测试通过；最终执行并通过
  `go test ./model ./service ./controller -count=1 -timeout 60s`。Classic 相关 JSX 和
  八个语言文件通过 Prettier，两个 JSX 文件通过 ESLint，八个语言文件均通过 JSON
  解析和新翻译键完整性检查；Default 相关 TSX、六种语言和生产构建同步通过。
- `git diff --check` 通过。固定测试库存在；最终检查时 3000/3001 上已有本项目测试
  进程。由于工作树同时存在另一项开发改动，本次未重启这些进程，因而不把现有
  服务状态计作本修复的集成验证。本次不修改固定测试库，也不涉及数据库迁移或
  配置新增。

## 回滚

回滚 `controller/image_auto_route.go`、对应测试、`controller/relay.go` 的调用点，
`service/channel_select.go`、`model/channel_cache.go`、`model/ability.go` 中的渠道类型
排除，以及对应的 service/model 测试，即可恢复原有按客户端路径选择 Relay 格式的
行为；不涉及数据库回滚。
