# 图片编辑路由公式计费

## 背景

图片编辑上游不一定能被固定规格价覆盖。AtlasCloud OpenAI `gpt-image-2/edit` 会按输出尺寸、质量、输入图像素和
文本输入成本组合计费；仅用 `size + quality` 固定档位，或者额外按输入图数量加价，都无法稳定覆盖这类成本模型。

## 配置结构

`ModelPriceVariants` 和 `ModelRoutePriceVariants` 的单个配置新增可选 `formula`：

```json
{
  "gpt-image-2-enterprise": {
    "image.edit": {
      "resolution_enabled": false,
      "quality_enabled": false,
      "formula": {
        "enabled": true,
        "expression": "(input_image_tokens(input_base) * input_image_token_price + output_tokens(quality == \"high\" ? high_output_base : quality == \"low\" ? low_output_base : medium_output_base) * output_token_price + text_input_price) * currency_rate",
        "variables": {
          "input_base": 48,
          "low_output_base": 16,
          "medium_output_base": 48,
          "high_output_base": 96,
          "input_image_token_price": 0.000008,
          "output_token_price": 0.00003,
          "text_input_price": 0.005,
          "currency_rate": 6.74
        },
        "defaults": {
          "size": "1024x1024",
          "quality": "medium",
          "input_image_fallback_resolution": "1024x1024"
        }
      }
    }
  }
}
```

公式结果是最终按次单价，单位与 `ModelPrice` 一致。之后仍会按分组倍率和 quota 换算逻辑扣费；`n` 只适用于
上游明确支持多输出的图片生成链路，不适用于图片编辑路由。

## 表达式变量

公式引擎使用安全的 `expr` 环境，不执行任意代码。内置数值变量包括：

- `base_price`：模型固定价；公式路由可以在没有固定价时工作，此时为 `0`。
- `width`、`height`、`short_side`、`long_side`、`pixels`：输出规格。
- `input_images`、`input_image_count`：输入图数量。
- `prompt_tokens_estimated`、`prompt_chars`：预扣阶段的文本估算信息。

内置字符串变量：

- `quality`：请求质量档位，缺省从 `formula.defaults.quality` 取值。

内置函数：

- `max`、`min`、`abs`、`ceil`、`floor`、`round`
- `area_tokens(base, width, height)`
- `output_tokens(base)`：按输出 `width/height` 计算 token。
- `input_image_tokens(base)`：按已探测输入图尺寸求和；探测失败但知道输入图数量时，使用
  `defaults.input_image_fallback_resolution` 补齐。

自定义 `variables` 会归一为小写 snake_case。内置变量名和函数名不能被覆盖。

## 计费优先级

固定按次计费进入预扣后，图片编辑路由的优先级为：

1. `ModelRoutePriceVariants[model]["image.edit"].formula` 且 `enabled=true`：公式结果作为最终单价。
2. `ModelRoutePriceVariants[model]["image.edit"].rules`：路由规格价。
3. `ModelPriceVariants[model].rules`：模型级规格价。
4. `ModelPrice[model]`：固定兜底价。

公式命中后不会再叠加 `extra_params`，因为公式应完整表达该路由的最终单价。未启用公式时，`extra_params`
仍按旧逻辑叠加到已选择的路由价或模型价上。

## AtlasCloud 计费事实

AtlasCloud adapter 只负责在预扣前补充公式需要的请求事实：

- 仅当当前模型的 `image.edit` 路由启用了公式时才探测输入图尺寸。
- JSON URL、data URI/base64 和 multipart 本地图片都会尽量探测宽高。
- multipart 本地上传的输入图数量由请求解析层统一写入 `InputImageCount`；AtlasCloud 公式事实层只在缺失时用
  multipart 文件数兜底，避免同一张上传文件被重复计入输入图成本。
- AtlasCloud 媒体 URL 探测复用 AtlasCloud 输出图下载的授权 header。
- adapter 不包含价格公式，不按模型硬编码扣费逻辑。

## 前端

default 和 classic 的按次计费编辑器在“图片编辑路由计费”中暴露公式配置：

- 启用公式计费。
- 编辑公式表达式。
- 编辑数值变量。
- 编辑字符串默认值。
- 提供场景化快速模板，管理员先选择“官方 token 公式”“输入图加价”或“固定路由价”，再只调整界面提示的变量数值和默认值。
- 模板入口已按运维场景收敛为“AtlasCloud gpt-image-2/edit”“输入图额外加价”和“固定编辑价格”，并在变量与默认值行内展示字段含义。
- 高级公式表达式默认折叠；常规配置只需要套用预设并调整变量值，只有上游公式变化或排查问题时才需要展开编辑表达式。
- 公式表达式在界面上标记为高级项；套用模板后通常不需要直接修改表达式，除非上游官方公式本身变化。
- 模板说明保留关键变量名，例如 `currency_rate`、`text_input_price`、`input_base`、`input_image_unit_price` 和
  `input_image_fallback_resolution`，避免把可保存字段翻译成无法对应 JSON 的名称。

后台保存仍使用同一份 `ModelRoutePriceVariants` JSON，不新增独立 option。

## 使用日志计费过程

公式命中后，后端会在使用日志 `other` 中写入结构化计费字段，字段统一带 `billing_` 前缀：

- `billing_mode=route_formula`、`billing_route_price_status=formula`：标识图片编辑路由公式计费。
- `billing_formula_price`：公式算出的最终按次单价，单位与 `ModelPrice` 一致。
- `billing_formula_width`、`billing_formula_height`、`billing_formula_quality`、`billing_formula_input_images`：
  本次计费用到的输出规格、质量和输入图片数。
- `billing_formula_var_<name>`：管理员配置的公式变量值，例如 `input_image_token_price`、`output_token_price`、
  `currency_rate`。
- `billing_formula_default_<name>`：管理员配置的默认值，例如 `size`、`quality`、
  `input_image_fallback_resolution`。
- `billing_formula_calc_<name>`：公式运行时计算出的可解释分项。目前会覆盖 AtlasCloud token 公式需要的
  `input_image_tokens`、`input_image_cost`、`output_base`、`output_tokens`、`output_cost`、
  `text_input_cost`、`subtotal`、`currency_rate`、`converted_total`，以及输入图按张加价模板的
  `base_price`、`input_image_extra_units`、`input_image_surcharge`。

异步图片任务 `/v1/images/tasks?action=edits` 会在后台复用普通图片编辑 relay；成功使用日志同样从
`relayInfo.PriceData.BillingMeta` 持久化这些 `billing_*` 字段，因此详情弹窗可以展示公式计费过程。

classic `renderModelPrice` 和 default 使用日志详情弹窗都会把这些字段渲染为逐行“计费过程”，例如：

```text
命中计费方式：图片编辑路由公式计费
计费表达式：输入图成本 + 输出图成本 + 文本输入成本
输出规格：1024x1024
输出质量：medium
输入图片：1 张
生成数量：1 张
输入图成本：1715 tokens × 0.000008 = 0.013720
输出图成本：1756 tokens × 0.000030 = 0.052680
文本输入成本：0.005000
公式小计：0.013720 + 0.052680 + 0.005000 = 0.071400
最终单价：0.481236 / 次
分组倍率：1x
最终扣费：0.481236
仅供参考，以实际扣费为准
```

计费过程面向终端用户解释成本构成，不展示 `currency_rate` 等内部换算变量；最终单价以
后端已经计算并落库的 `billing_formula_price` 为准，最终扣费优先使用日志真实 `quota` 换算结果，
避免 `n`、实际返回图片数或其它倍率导致前端重算偏差。如果公式不是内置可识别分项，例如完全自定义表达式，
前端会回退展示 `billing_formula_detail` 的简要说明，仍保留最终单价和最终扣费；`formula_detail`
不得拼接完整变量、默认值或计算分项，避免把内部换算变量暴露到普通日志内容。

非公式的图片编辑路由规格价、路由固定价和额外参数加价仍然属于按次计费，但使用日志的计费过程不能直接复用
图片请求内容（例如“大小、品质、生成数量”）作为解释。Classic 使用日志详情在检测到
`billing_price_route`、`billing_route_price_status`、`billing_variant_price_status`、
`billing_resolution`、`billing_quality` 或 `billing_extra_price` 时，会改用结构化计费过程，
展示命中计费方式、输出规格、输出质量、输入图片、生成数量、额外图片加价、最终单价、分组倍率和最终扣费。
最终扣费同样优先使用日志真实 `quota` 换算，保证展示与实际扣费一致。
Classic 金额格式化 helper 必须位于文件级作用域，供公式计费和非公式图片路由计费过程共用，避免运行时因
局部函数不可见出现 `formatCost is not defined`。

图片固定按次计费的数量倍率必须区分“输出数量”和“输入图片数量”：`n` 是图片生成链路的输出张数，不是图片编辑
链路的输入图数量；编辑链路的输入图数量通过 `input_images` 参与公式或额外参数计费。AtlasCloud OpenAI
`edit` 路由不转发 `n` / `num_images`，预扣和结算也不按 `n` 放大。

AtlasCloud OpenAI `text-to-image` 路由按最终上游模型判断输出数量能力：

- `openai/gpt-image-1/text-to-image` 官方支持 `n`，adapter 会把客户端 `n` 原样转发给 AtlasCloud，
  上限为 10。
- `openai/gpt-image-1.5/text-to-image` 和 `openai/gpt-image-2/text-to-image` 官方摘录未提供
  `n` / `num_images`。adapter 会在 `n > 1` 时 fan-out 为多次单图 `generateImage` 请求，每个子请求都不携带
  `n` / `num_images`，再按子请求索引合并输出为一次 OpenAI 图片响应。
- fan-out 上限同样为 10 张。fan-out 计费仍使用 NewAPI 的按次图片数量倍率；当部分子请求失败但已有可交付图片时，
  使用日志和最终结算按实际成功输出数记录，避免按请求数多扣。全部子请求失败时整单失败。
- fan-out 子请求共享下游请求的生命周期，但单个子请求失败不得主动取消其他子请求；失败结果单独收集，仍等待其余子请求完成。只有客户端取消或下游请求上下文本身结束时才统一终止所有子请求；此时返回取消错误，不把已完成的个别子请求作为部分成功结算。

## 验证

- `go test ./pkg/priceformula ./setting/ratio_setting ./relay/helper ./relay/channel/atlascloud ./controller`
- `go test ./relay/channel/atlascloud -run '^TestAtlasCloudFanoutKeepsSuccessfulSiblingsAfterChildFailure$' -count=1`
- `go test ./relay/channel/atlascloud -run '^TestAtlasCloudFanoutPropagatesCallerCancellation$' -count=1`
- `web/default`: `bun run typecheck`
- `web/default`: `bun run i18n:sync`
- `web/classic`: `bun run build`

## 限制

- 公式计费发生在预扣阶段，依赖请求体和可探测输入图尺寸；它不会等待上游真实账单回传。
- 如果输入图尺寸探测失败，应配置保守的 `input_image_fallback_resolution`。
- 公式语法错误由后端保存校验兜底；前端只做字段形态校验。

## 2026-08-14 文案修复补记

本次 dev 镜像顺手修复了中文 locale 文案。此前批量写入中文字符串时受 Windows PowerShell 5.1 编码影响，
部分 `zh` / `zh-CN` / `zh-TW` 内容显示为 `????`。已恢复 `web/default/src/i18n/locales/zh.json`、
`web/classic/src/i18n/locales/zh.json`、`web/classic/src/i18n/locales/zh-CN.json` 和
`web/classic/src/i18n/locales/zh-TW.json` 的正常中文，并重新部署到
`maolao-newapi-dev:route-formula-pricing-ux-20260814125921`。

## 2026-08-14 gpt-image-2 edit billing smoke test

这次验证的重点是把图片编辑链路的实际扣费和站点余额变化对齐。测试时先确认 dev `/v1/models` 只暴露 `gpt-image-1`、`gpt-image-1.5`、`gpt-image-2`、`grok-imagine-image`、`grok-imagine-video`、`grok-imagine-video-1.5`，`gpt-image-2-enterprise` 不在可用模型列表里，直接请求会报 `No available channel for model gpt-image-2-enterprise under group default (distributor)`。随后改用 canonical `gpt-image-2` 跑同一固定 prompt 和远程图片引用的 `images/edits`。

结果是 one-image 和 three-image 两单都成功；`usage.total_usage` 从 `299.7154` 变到 `348.06`，再到 `415.3414`。该接口以 `0.01` 计费单位返回数值，因此原始增量 `48.3446` 和 `67.2814` 分别对应实际扣费 `0.483446` 和 `0.672814`。容器结算日志记录的实际消耗与这两个换算结果一致，说明当前公式计费与站点扣费匹配。结果包已保存到 `temp/atlascloud-edit-billing-gpt-image-2-20260814.tar.gz`，里面保留了请求体、提交响应、轮询快照和 usage 前后对比，便于后续核查。
