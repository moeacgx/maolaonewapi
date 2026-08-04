# AtlasCloud 图像与视频渠道适配

## 目标

接入 AtlasCloud 提供的图像与视频模型，让运营侧可以在渠道中单独配置 AtlasCloud，并通过
现有 OpenAI 图像接口和视频任务接口转发到 AtlasCloud 上游。

本次范围只覆盖图像和视频，不接入 AtlasCloud 聊天模型，也不把 AtlasCloud 伪装成 xAI
原生协议。AtlasCloud 使用独立渠道类型，避免后续聊天、图像、视频能力混在同一个协议假设下。

## 方案

- 新增 `AtlasCloud` 渠道类型和 API 类型。
- 图像生成走 `POST /api/v1/model/generateImage`，同步轮询
  `GET /api/v1/model/prediction/{id}`，完成后转换为 OpenAI 图像响应结构。
- 视频生成走现有异步任务链路，提交到 `POST /api/v1/model/generateVideo`，任务查询复用
  `prediction/{id}`，完成后把输出 URL 写入任务结果。
- 图像编辑、输入图和视频首帧支持远程 URL；`data:` URL 和 multipart 上传文件会先通过
  `POST /api/v1/model/uploadMedia` 上传，再把返回 URL 传给生成接口。
- 任务日志图片预览统一返回站内 content 代理路径，后端按需从任务数据中的 `b64_json`、data URL
  或远程 URL 获取图片字节，浏览器不直接访问 AtlasCloud 上游媒体 URL。
- AtlasCloud 仍保留内部渠道类型；任务日志额外返回 `display_platform`，按模型归属展示为 `xAI`
  或 `OpenAI`，避免把 AtlasCloud 当作最终模型平台展示。
- 请求体保留 `extra_fields` 和未知扩展字段，用于承载 AtlasCloud 后续新增但本项目尚未显式建模的参数。
- 对外模型名保持 NewAPI 兼容短名；短名到 AtlasCloud `provider/model/task` 上游完整模型名的关系由渠道
  “模型重定向”配置承载，不在 adapter 内硬编码。当前建议配置：
  - `gpt-image-1` -> `openai/gpt-image-1/text-to-image`
  - `gpt-image-1.5` -> `openai/gpt-image-1.5/text-to-image`
  - `gpt-image-2` -> `openai/gpt-image-2/text-to-image`
  - `grok-imagine-image` -> `xai/grok-imagine-image/text-to-image`
  - `grok-imagine-video` -> `xai/grok-imagine-video/text-to-video`
  - `grok-imagine-video-1.5` -> `xai/grok-imagine-video-v1.5/text-to-video`
- 图像编辑属于 AtlasCloud action 差异：当渠道重定向后的上游模型名以 `/text-to-image` 结尾时，
  adapter 会在 `action=edits` 时派生为对应 `/edit` 模型名。OpenAI 图像编辑请求字段使用 `image`，
  Grok/xAI 图像编辑请求字段使用 `image_urls`。
- `grok-imagine-image` 按 AtlasCloud 官方模型页的 `$0.02 / PIC` 补入默认固定价；
  这会让非自用模式下的 `/v1/models` 和请求前置计费校验正常识别该模型。
- `gpt-image-1.5` 与 `gpt-image-2` 按 AtlasCloud 当前公开 guide 的 `$0.008 / PIC`
  补入默认固定价；运营侧如需使用更细的规格/质量价格，可在后台显式覆盖。
- 图片固定价格模型会从请求的 `size` 与 `quality` 写入计费维度，并在预扣费阶段匹配
  `ModelPriceVariants`。命中档位时使用后台配置的绝对单价；未命中时保留旧固定价/倍率路径，避免未知规格
  静默落到低价。

## 兼容与限制

- 不改变现有渠道、计费、分组、重试或鉴权逻辑。
- AtlasCloud 图像模型只暴露 `image-generation` 端点；视频模型只暴露 `openai-video` 端点。
- 不加入流式聊天能力，也不加入 `streamSupportedChannels`。
- 图像同步轮询有超时保护；长时间任务应优先使用视频任务类异步链路。
- 图片 content 代理依赖任务数据保留期；任务结果过期后，旧图片可能无法继续从任务日志预览。
- 上游响应格式以 AtlasCloud 当前文档为准：提交返回 `data.id`，轮询读取 `data.status`，
  完成后读取 `data.outputs[0]`。
- AtlasCloud 官方模型页使用 `provider/model/task` 形式的模型名；渠道必须通过 `model_mapping`
  把 NewAPI 对外模型名映射到完整上游模型名。若未配置映射，adapter 会原样透传模型名，上游可能返回
  `not found`。
- `gpt-image-1`、`gpt-image-1.5`、`gpt-image-2` 的尺寸/质量档位价格不写死在 adapter；
  应由运营侧在 `ModelPriceVariants` 中按模型配置 `resolution_enabled`、`quality_enabled` 和
  `rules`。
- `gpt-image-2` 真实生成与编辑更容易超过默认 120 秒图片 prediction 轮询窗口；adapter 对
  `gpt-image-2`、`openai/gpt-image-2/text-to-image` 和 `openai/gpt-image-2/edit` 单独使用
  300 秒轮询超时，其他图片模型继续使用 120 秒，避免整体拉长失败反馈。

## 测试计划

1. 覆盖渠道类型、API 类型和端点类型映射。
2. 覆盖图像请求转换、图片上传路径和轮询结果转换。
3. 覆盖视频任务提交、任务查询和 OpenAI 视频响应元数据转换。
4. 执行相关 Go 包测试：
   `go test ./common ./controller ./relay/channel/atlascloud ./relay/channel/task/atlascloud ./relay ./setting/ratio_setting`。
5. 覆盖图片固定价格模型的 `ModelPriceVariants` 匹配，确认档位绝对价会覆盖旧图片倍率并仍按 `n` 结算。
6. 部署到 DBM dev 环境后验证 `/api/status`、容器健康状态和 `dev.nu11.me` 访问。

## 验证结果

- 本地相关 Go 包测试通过。
- 本地 `go test ./...` 因缺少 `web/classic/dist` 前端构建产物在根包初始化阶段失败，
  与本次 AtlasCloud 适配代码无关。
- DBM 上误将首轮测试镜像部署到既有 `newapi-gang-dev` 环境后已回滚：
  `newapi-gang-dev-app` 恢复为 `newapi-gang-dev:645082ad8`，误加的 `dev.nu11.me`
  站点域名绑定已移除，repo 恢复干净。
- DBM 因 GitHub 授权暂不可读取 `maolaonewapi` 仓库，独立 dev 环境临时使用本地工作树源码包部署；
  正式 PR 前应恢复 Git-first 流程。
- DBM 独立目录 `/home/paseo/apps/maolao-newapi-dev` 已创建，独立镜像
  `maolao-newapi-dev:atlascloud-20260803095431` 构建成功，并部署到
  `maolao-newapi-dev-app`。
- 独立容器使用独立数据卷 `maolao-newapi-dev-data` 和端口 `127.0.0.1:19082 -> 3000`。
- `dev.nu11.me` 通过专用 Nginx server block 反代到 `127.0.0.1:19082`；
  `main.nu11.me` 仍指向既有 `newapi-gang-dev-app`。
- `https://dev.nu11.me/api/status`、`https://dev.nu11.me/` 和
  `https://main.nu11.me/api/status` 均返回 `200`；带唯一 query 的探针确认
  `dev.nu11.me` 命中 `maolao-newapi-dev-app`。
- 2026-08-03 重新构建并部署 `maolao-newapi-dev:atlascloud-grok-image-price-20260803123016`；
  `/v1/models` 已暴露 `gpt-image-1`、`grok-imagine-image`、`grok-imagine-video`、
  `grok-imagine-video-1.5`、`sora-2` 和 `sora-2-pro`。
- 使用临时 NewAPI 测试 token 完成真实 relay 验收：`gpt-image-1` 图像生成成功，
  `grok-imagine-image` 图像生成成功，`grok-imagine-video` 与
  `grok-imagine-video-1.5` 均提交成功并轮询到 `completed`，输出资源 URL 已脱敏不记录。
- 2026-08-04 dev 验收修复：`maolao-newapi-dev` 必须使用完整 `Dockerfile` 构建，不能使用
  `Dockerfile.dev`，否则根路径只会返回 `use frontend dev server` 占位页。
- 2026-08-04 classic 任务日志视频预览改为优先打开 `/v1/videos/{task_id}/content` 本机代理，
  避免浏览器直接播放 AtlasCloud 上游结果 URL 时触发跨域、防盗链或鉴权限制。
- 2026-08-04 AtlasCloud 默认暴露模型先收敛为已通过渠道重定向并验收的 4 个：`gpt-image-1`、
  `grok-imagine-image`、`grok-imagine-video`、`grok-imagine-video-1.5`。`sora-2`、
  `sora-2-pro`、`grok-imagine-image-pro`、`grok-2-image-1212` 暂不从 AtlasCloud 渠道暴露，
  避免后台配置到尚未确认上游完整模型名或未验收的模型。
- 2026-08-04 任务日志图片预览修复已在 DBM dev 验证：新提交的 AtlasCloud 异步图片任务通过
  `/v1/images/tasks/{task_id}/content/0` 返回 `200 image/jpeg`，确认图片结果走站内 content
  代理，不再让浏览器直连上游媒体 URL。
- 2026-08-04 AtlasCloud 视频任务日志平台展示已改为按模型归属显示：Grok/xAI 模型展示 `xAI`，
  OpenAI/gpt-image/Sora 模型展示 `OpenAI`。classic 与 default 前端均优先使用后端
  `display_platform` 字段。
- 2026-08-04 本地补充 `gpt-image-1.5` 与 `gpt-image-2` 的默认模型列表和默认固定价；
  模型名转换改由渠道 `model_mapping` 配置完成，adapter 不再硬编码短名到完整上游模型名的映射。
  当前为本地单元测试覆盖，尚未在 DBM dev 用真实 AtlasCloud token 完成生成/编辑验收。
- 2026-08-04 AtlasCloud 渠道前端图标改为 lucide `CloudCog`，不再复用 OpenAI 品牌图标。
- 2026-08-04 DBM dev 验证 `gpt-image-2` 生成和编辑首次任务均曾触发 AtlasCloud prediction
  120 秒轮询超时，轻量重试成功；本地已将 `gpt-image-2` 图片 prediction 轮询窗口提升到 300 秒，
  其他图片模型仍保持 120 秒。
- 2026-08-04 本地补充图片固定价格 `ModelPriceVariants` 匹配：`ImageRequest` 会把 `size`/`quality`
  作为 `resolution`/`quality` 计费维度传入 `ModelPriceHelper`，命中后台档位后使用绝对单价重算预扣。
  相关测试 `go test ./dto ./relay/helper ./relay/channel/atlascloud ./relay/channel/task/atlascloud ./relay ./setting/ratio_setting`
  通过。
