# 通用动态路由

日期：2026-08-23

## 目标

新增一个位于“系统设置”的独立“动态路由”页面，用于在渠道已选定后，
按公开模型、上游渠道类型、请求路径和请求条件选择实际发送给上游的模型或协议动作。

新版前端和 Classic 前端都提供该全局设置入口。Classic 的入口位于
`/console/setting?tab=dynamic-routing`；它与新版前端保存相同的两个系统选项，
因此切换前端主题不会产生两份独立配置。

该能力默认关闭，不限定 Gemini。它可用于 Gemini、Claude、OpenAI 图片等
任意已支持的渠道类型；渠道可以声明自己的规则，并在相同模型与作用范围内
覆盖全局规则。

## 本期范围

本期实现三个动作。

`model_redirect`：

- 保持下游请求所使用的接口、协议和响应格式不变。
- 将已经选中渠道的最终上游模型名改为 `target_model`。
- 命中后阻止已知的思考后缀适配器再次改写该最终模型名，避免
  `-high`、`-thinking` 等管理员指定的上游模型后缀被剥离。

`responses_image_tool_bridge`：

- 仅处理下游 `POST /v1/responses`，并且请求同时声明内置
  `image_generation` 工具、`tool_choice` 明确选择该工具时才命中。
- 从 Responses `input` 提取文本，构造目标图片模型的 OpenAI Images 请求，
  目标路径明确记录在 `target_path`。当前允许 `POST /v1/responses` 保持
  Responses 原生协议，或 `POST /v1/images/generations` 转成同步生图请求；
  裸的 `/v1/images/` 不是可执行端点。
- 原生 Responses 路径会同时改写顶层 `model` 与 `image_generation` 工具内的
  `model` 为 `target_model`，避免上游按工具字段回落到默认图片模型。
- 上游响应中的 `data[].b64_json` 被包装成 Responses 的
  `image_generation_call.result`；下游 `stream=true` 时合成为 Responses SSE。
  走 Images API 时固定请求 `response_format: "b64_json"`，并强制经过适配器
  转换，不受全局或渠道“透传原始请求体”开关影响。
- Images API 转换会保留工具中的 `size`、`quality`、`background`、`moderation`、
  `output_format` 和 `output_compression`。`partial_images` 只能为 `0`；其他
  不支持的工具参数会返回 400，不能静默降级成默认规格。
- 图片输入、`/v1/images/edits` 和异步 `/v1/images/tasks` 不在本期范围，命中后
  明确返回错误，不降级成普通文本请求。

该动作不是普通模型重定向：桥接会重新选择支持目标图片模型和规则 `target_path`
的渠道，并以目标图片模型进行预扣、结算和日志计费。
`responses_image_function_bridge`：

- 适用于下游 `POST /v1/responses` 的文本模型。命中规则后，网关注入私有
  `newapi_image_generation` 函数定义和指令标记；只有源模型实际返回该函数调用
  时才进入第二阶段。
- 函数参数目前为 `prompt`、`size`、`quality`、`output_format`。网关转换为
  `ImageRequest`，固定调用目标模型的 `POST /v1/images/generations`；裸
  `/v1/images/`、编辑和异步任务路径不接受。
- 非流式源请求直接捕获 `function_call`。流式源请求先缓存 Responses SSE，未触发
  私有函数时按原顺序回放；触发时丢弃源事件并在目标图片完成后合成
  `image_generation_call` Responses JSON/SSE。
- 源渠道启用 `responses_to_chat_enabled` 时，Chat 转 Responses 的非流式和流式
  路径同样捕获函数调用，中间函数事件不会泄露给下游。
- 源文本阶段和目标图片阶段使用独立请求 ID、渠道指标、预扣/结算和重试状态。
  目标图片失败只处理目标渠道，不重试源文本请求，也不禁用源文本渠道。
- 图片阶段强制执行 Images 适配器转换，避免把 Responses JSON 原样发到 Images
  端点；源文本 usage 与目标图片 usage 分别结算，不增加私有函数工具附加费。

## 配置契约

全局配置由通用系统设置接口保存为两个选项：

- `dynamic_routing.enabled`：布尔总开关，默认 `false`。
- `dynamic_routing.rules`：规则数组 JSON。

渠道独立配置存放在渠道 `setting` JSON 的
`dynamic_routing` 字段中，而不是渠道类型专用的 `settings` 字段：

```json
{
  "dynamic_routing": {
    "enabled": true,
    "rules": []
  }
}
```

渠道字段 `enabled` 是三态：缺省时继承全局总开关，`false` 时完全关闭该渠道的
动态路由，`true` 时即使全局关闭也启用该渠道规则。

每条规则的结构如下：

```json
{
  "id": "gemini-flash-high",
  "enabled": true,
  "action": "model_redirect",
  "source_model": "gemini-3.7-flash",
  "target_model": "gemini-3.7-flash-high",
  "channel_types": [],
  "request_paths": ["/v1/chat/completions"],
  "conditions": [
    {
      "field": "reasoning_effort",
      "operator": "equals",
      "value": "high"
    }
  ],
  "priority": 100
}
```

图片工具桥接规则示例：

```json
{
  "id": "codex-image-generation",
  "enabled": true,
  "action": "responses_image_tool_bridge",
  "source_model": "gpt-5.6-sol",
  "target_model": "gpt-image-2",
  "target_path": "/v1/images/generations",
  "source_groups": ["codex"],
  "target_group": "image",
  "request_paths": ["/v1/responses"]
}
```

字段含义：

- `id`：启用规则的唯一标识，供运行时追踪；同一作用域内的启用规则不可重复。
- `enabled`：规则开关。禁用规则不参与匹配。
- `action`：支持 `model_redirect`、`responses_image_tool_bridge` 和
  `responses_image_function_bridge`；缺省按 `model_redirect` 处理，其他值会被
  拒绝。桥接动作的请求路径只能是 `/v1/responses`；工具桥接的 `target_path`
  只能是 `/v1/responses` 或 `/v1/images/generations`，函数桥接固定为
  `/v1/images/generations`。
- `source_model`：下游公开模型名，精确匹配，不使用通配符。
- `target_model`：普通重定向时是传给已选渠道上游的模型名；桥接动作时是目标
  图片模型名。管理员必须确保目标渠道和上游支持此模型及图片生成路径。
  桥接目标模型不能使用 `tiered_expr`；该表达式只定义 token 计费，尚未定义按
  实际出图张数结算的语义，运行时会直接拒绝以避免错误扣费。
- `target_path`：仅桥接动作使用的上游目标路径。缺省按
  `/v1/images/generations` 处理；允许填写 `/v1/responses` 或
  `/v1/images/generations`。后者会将 Responses 文本输入转换为 Images 请求，
  前者保持 Responses 协议；不允许填写裸 `/v1/images/`、编辑或异步任务路径，
  以免把同步生成误发到不兼容接口。
- `source_groups`：可选的源分组代码数组；为空表示不限源分组。管理页面按分组
  显示名称提供多选下拉，保存时写入对应 code。它匹配渠道初次选定后的实际分组，
  因此可为同一个公开模型设置不同分组的策略。
- `target_group`：可选的目标分组代码；管理页面按分组显示名称提供下拉，保存时
  写入对应 code；为空时桥接沿用源分组，非空时固定在该
  分组选择目标模型渠道并在重试时保持该分组。请求用户必须可使用该分组，且令牌
  必须已显式声明它，或将其列在自身 Auto 分组中；未声明分组的继承令牌只能留在
  当前源分组。
- 函数桥接在私有函数实际触发后重新选择目标图片渠道；目标渠道重试使用目标分组
  的排除列表，并保持目标模型和图片路径。
- `channel_types`：可选的上游渠道类型编号数组；为空表示不限类型。页面按渠道
  类型选择，配置不应手写不明编号。
- `request_paths`：可选的下游请求路径精确匹配数组；为空表示不限路径。查询串
  不参与匹配。
- `conditions`：全部条件同时满足才命中；为空表示不附加条件。
- `priority`：数值越大优先级越高；相同优先级按配置列表顺序匹配。

支持的条件为：

- `reasoning_effort`：已归一化的思考等级，例如 `low`、`medium`、`high`。
- `request.<简单 JSON 路径>`：从已解析请求中读取字段，例如
  `request.reasoning.effort`。路径只能由字母、数字、下划线和点组成。

支持的条件运算符为 `equals`（缺省）、`not_equals`、`exists` 和
`not_exists`。条件值统一按字符串比较；因此应优先用于稳定的标量请求字段。

## 页面预设

新版和 Classic 的动态路由设置页均提供“快速应用预设”。预设会直接新增一条可编辑
规则，只预填不会依赖某个部署环境的结构化字段；管理员仍需填写自身实际存在的
`source_model`、`target_model`，跨分组时从目标分组名称下拉框选择 `target_group`。

- 基础模型重定向：`model_redirect`，不附加请求条件，不改变下游请求路径。
- 思考等级重定向：`model_redirect`，预填
  `reasoning_effort = high`，适合指向带思考后缀的 Gemini、Claude 等上游模型。
- Responses 图片工具转 Responses：`responses_image_tool_bridge`，固定下游
  `/v1/responses`，目标路径预填 `/v1/responses`。
- Responses 图片工具转 Images API：`responses_image_tool_bridge`，固定下游
  `/v1/responses`，目标路径预填 `/v1/images/generations`。

两个图片预设都不添加通用 `tool_choice` 条件。桥接动作自身会同时验证
`tools` 中存在 `image_generation` 以及 `tool_choice` 明确选择该工具；额外写成
`tool_choice = image_generation` 既不能让客户端获得图片工具，也可能因对象形式的
`tool_choice` 而额外限制本应有效的请求。

## 解析顺序

运行时在渠道已选定、协议适配器执行前解析规则。顺序如下：

1. 渠道配置显式为 `false` 时，停止动态路由，继续原有静态
   `model_mapping`。
2. 渠道动态路由已启用（显式为 `true`，或继承到已开启的全局开关）时，先找该
   渠道中与当前公开模型、渠道类型和请求路径相符的候选规则。
3. 只要渠道存在上述候选规则，便只在渠道候选中按优先级检查条件；即使条件都
   不满足，也不会再回退到全局同模型规则。这使渠道能可靠覆盖全局默认策略。
4. 渠道没有候选规则且全局总开关开启时，按同样规则匹配全局规则。
5. 两个作用域都未命中时，保持旧的静态 `model_mapping` 链式重定向行为。

普通 `model_redirect` 只决定模型名，不重新选择渠道。因此它不会让请求跳到
另一个本不支持 `source_model` 的渠道。桥接动作是明确例外：命中后会按目标图片
模型、`target_path`（默认 `/v1/images/generations`）和可选的 `target_group`
重新选择渠道；令牌绑定的单渠道仍必须自身支持目标图片模型，否则请求会直接失败。
首次与重试选渠均以目标分组为准。当前桥接目标渠道通过缓存随机选择，尚未接入
目标模型的会话粘性；需要粘性时应在后续独立接入目标模型 affinity。
管理员指定渠道遇到 Auto 或多分组令牌时，桥接会按令牌的有序授权分组，选择首个
同时启用该固定源渠道和公开源模型的分组作为 `source_groups` 的匹配口径；若无法
解析来源分组，规则必须显式填写 `target_group`。

函数桥接在源文本 relay 成功返回后继续执行第二阶段；函数未触发时，流式缓存按
原协议回放，普通文本请求完全保持原响应。目标图片阶段使用独立请求 ID、计费和
渠道指标，失败只在目标图片渠道范围内重试。

## 校验与安全边界

- 每个作用域最多 100 条规则，每条最多 8 个条件；模型、规则 ID、条件值和路径
  均有长度上限。
- `source_groups` 与 `target_group` 只能填写单个有效分组代码；不接受 `auto`、
  逗号分隔组或重复源分组。
- 图片工具桥接的目标模型不能配置 `tiered_expr` 计费。需要表达式按张定价时，
  必须先扩展表达式契约，不能把 token 表达式直接用于图片张数结算。
- `channel_types` 不得含非正数或重复值；`request_paths` 必须以 `/` 开头、不得
  含查询串或重复项。
- 仅在保存时校验配置；运行时对过期或不支持的规则失败关闭，不应阻断原有静态
  映射链路。
- 页面与系统设置根路由均继承超级管理员权限；渠道覆盖仍遵循现有渠道编辑权限。
- 规则配置中不得写入上游密钥、Cookie 或其他凭据。

## 计费、日志与兼容性

`model_redirect` 保留 `OriginModelName` 作为下游公开模型。现有预扣费、结算、
模型价格和大部分用户可见账单均继续以公开模型为口径；实际调用模型只写入
`UpstreamModelName` 供上游请求和管理员排障使用。

`responses_image_tool_bridge` 则以目标图片模型作为本次实际调用和计费模型，保留
源模型用于 Responses 返回的 `model` 字段和桥接关联信息；不会叠加 Responses
内置工具附加费。当前实现不创建源文本模型的虚假消费记录。令牌的模型限额继续
以客户端请求的公开 `source_model` 为口径；`target_model` 是管理员内部路由目标，
不要求在令牌模型白名单中单列。桥接预扣按一张图片执行，终态按成功的
`image_generation_call` 或 Images `data[]` 实际数量结算；即使上游没有 usage，
成功图片仍会扣费，多图会按实际数量补扣。没有成功图片时会全额结算为零，即使
上游同时返回了文本 usage。

`responses_image_function_bridge` 的源文本调用与目标图片调用分别记录 usage 和
消费日志；私有函数本身不产生工具附加费。源文本阶段成功后即结算，目标阶段失败
不会回放或重试源文本请求。

关闭全局开关、关闭规则或没有命中时，既有渠道模型映射、请求适配和计费行为不变。

## 验证

实现完成后应至少验证：

- 规则 DTO 与渠道设置的保存校验，包括非法动作、重复 ID、非法条件和作用范围。
- 全局匹配、渠道覆盖、渠道关闭、全局回退、静态 `model_mapping` 回退与优先级。
- `reasoning_effort` 和 `request.*` 条件的确定性匹配。
- Gemini、Claude、OpenAI/兼容渠道的思考后缀不会在动态命中后改写目标模型。
- Responses 图片工具桥接只在明确的 `tool_choice.image_generation` 命中，且上游
  请求 URL 精确为 `/v1/responses` 或 `/v1/images/generations`；后者必须使用
  适配器转换后的 `b64_json` 响应。
- 桥接 JSON 和下游 Responses SSE 均返回 `image_generation_call.result`，缺少
  `b64_json` 时失败关闭。
- Responses 图片函数桥接应验证非流式函数调用捕获、流式参数 delta 拼接、无触发
  时原序回放、触发时不泄露源 SSE、`responses_to_chat_enabled` 两条转换路径，
  以及目标分组/渠道重试和源/目标计费指标隔离。
- 原生 Responses 的图片工具模型必须改为 `target_model`；Images 转换必须保留
  支持的图片规格参数，遇到不支持参数不能静默忽略。
- 零 usage 的成功单图、多图和“有文本 usage 但没有成功图片”分别验证正确的
  预扣补差与退款。
- 目标分组的继承、显式令牌和 Auto 令牌授权，以及重试不回退到源分组。
- 前端类型检查/生产构建，以及 `git diff --check`。

常规开发与回归仅使用 `zzapi`；不得把 `maolaoapi` 作为本功能的测试或部署目标。
