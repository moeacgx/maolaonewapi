# 图片编辑路由规格计费

## 背景

部分图片模型的生成和编辑接口共用同一个对外模型名，但上游会按不同路由或任务类型计费。
`gpt-image-2` 的编辑接口就是这种场景：生图可以按尺寸和质量给出固定档位价，编辑还会受输入图和
prompt 等因素影响，不能简单复用生图价格。

## 方案

- 新增 `ModelRoutePriceVariants` option，结构为 `model -> route -> specification price config`。
- 首期内置路由名为 `image.edit`，用于覆盖图片编辑接口。
- `/v1/images/edits` 和 `/canvas/v1/images/edits` 都归一为 `image.edit`；异步图片编辑任务内部也会走
  `/canvas/v1/images/edits`，因此复用同一条计费路径。
- 生图仍使用现有 `ModelPrice` 和 `ModelPriceVariants`，不需要单独路由配置。
- 计费优先级：
  1. 固定价格模型进入预扣费后，先判断当前请求是否命中可识别路由。
  2. 若命中 `ModelRoutePriceVariants[model]["image.edit"]` 的规格规则，使用该规则的最终单价。
  3. 若路由已配置但规格未命中，回落到原 `ModelPriceVariants` 或 `ModelPrice`。
  4. 若未配置路由价，保持既有模型级按次计费行为。
- 图片编辑请求缺少规格参数时，如果该模型已启用图片编辑路由规格价或模型级规格价：
  - 缺少 `size` 时默认补为 `1024x1024`。
  - 缺少 `quality` 时默认补为 `medium`。
  - 已显式传入的参数不覆盖；未启用规格价的模型不额外补参数。

示例：

```json
{
  "gpt-image-2": {
    "image.edit": {
      "resolution_enabled": true,
      "quality_enabled": true,
      "rules": [
        {
          "resolution": "1024x1024",
          "quality": "medium",
          "price": 0.12
        }
      ]
    }
  }
}
```

## 实现范围

- 后端：
  - `setting/ratio_setting` 新增路由规格价解析、校验、复制和匹配函数。
  - `ModelPriceHelper` 在固定价格图片编辑请求中优先匹配 `image.edit` 路由价。
  - 图片编辑请求在命中规格价配置时补齐缺省 `size=1024x1024` 和 `quality=medium`，避免缺参导致档位价不可命中。
  - `ModelRoutePriceVariants` 接入 option 初始化、保存校验、公开 ratio 数据和 pricing cache 失效。
  - 计费日志 metadata 增加 `price_route` 和 `route_price_status`，用于区分路由价命中、回落或禁用。
- 前端：
  - classic 模型定价可视化编辑器在按次计费下新增“图片编辑路由计费”。
  - classic JSON 编辑器新增 `ModelRoutePriceVariants` 字段。
  - default 设置页新增字段类型、JSON 校验、默认值、表单保存链路和可视化编辑链路；可视化列表会保留已有路由价配置。
  - default 模型创建/编辑抽屉和上游同步页都补齐了 `ModelRoutePriceVariants` 透传，避免保存或同步时丢失路由价。
  - classic 与 default 的规格差异计费、图片编辑路由计费都支持“表达式编辑”快捷录入；点击后打开模态窗口，
    每行一条规则，可使用 `resolution quality price`、`resolution price` 或 AtlasCloud `sku_out_*` 行。
    模态窗口会实时解析并用表格预览即将应用的分辨率、质量档位和价格；应用后仍保存为
    `ModelPriceVariants` / `ModelRoutePriceVariants` JSON，不改变后端计费语义。
  - classic 表达式编辑模态使用 Semi UI 独立导出的 `TextArea` 组件；不能使用 `Input.TextArea`，否则生产构建
    点击按钮时会因为渲染 `undefined` 组件触发 React #130。

## 验证

- `go test ./setting/ratio_setting ./relay/helper ./model` 通过。
- `web/default` 的 `bun run typecheck` 通过。
- `web/default` 修改文件的 `bunx prettier --check` 通过。
- `web/classic` 修改文件的 `bunx prettier --check` 通过。
- `web/classic` 的 `bun run build` 通过。
- `git diff --check` 通过。

## 限制

- 本补丁只提供路由级按次规格价，不实现上游 token 公式自动计算。
- 当前可视化编辑只开放 `image.edit`；以后需要覆盖更多路由时，应继续复用同一个
  `ModelRoutePriceVariants` 结构，而不是为单个模型新增专用字段。
- “表达式编辑”只是前端批量录入规格价格规则，不是 `billingexpr` token 动态计费；它不能按输入图实际像素、
  prompt token 或上游实时费用公式结算。
