# Classic 操练场图片附件与分组模型筛选

日期：2026-08-21

## 变更目标

简化 Classic 操练场的图片入口，并让分组与模型选择保持一致：

- 移除侧栏的图片地址输入和启用开关。
- 输入框直接支持粘贴图片，并提供手动选择图片按钮。
- 发送前将本地图片附件转换为 data URL，沿用现有多模态请求格式。
- 选择分组后只请求并展示该分组可用模型。
- 当前选中的分组在分组下拉列表中置顶；无有效选择时保留用户分组优先级。

## 范围与数据契约

本次只修改 `web/classic` 操练场界面和前端请求编排。模型筛选复用后端
`GET /api/user/models?group=<code>` 契约；后端已按请求分组返回启用模型，
`auto` 仍由服务端展开为配置顺序中的分组。
分组尚未确定时清空模型列表，不请求无 `group` 参数的全量模型。

图片附件只在输入组件中暂存，发送时转换为 `image_url.url` data URL，
发送后由 Chat 输入组件清理附件。图片不会写入操练场的 `inputs` 配置；
发送后的消息仍按原有消息历史机制保存，因此消息导出可能包含图片 data URL。

自定义请求体模式不接收图片附件，避免附件被静默丢弃；该模式仍按原逻辑
发送用户编辑的请求体。

## 安全与兼容性

- 上传控件限制为 `image/*`，并使用 `uploadTrigger="custom"`，不会向未知
  地址发起浏览器上传请求。
- 只接受 `FileItem.fileInstance` 中的图片文件，其他附件不会进入请求体。
- 快速切换分组时，旧模型请求的响应不会覆盖最新分组结果。
- 不改变后端模型、分组、计费或认证接口；旧配置中的图片字段会被忽略。

## 验证计划

- Classic 分组排序纯函数测试：已选分组、`auto` 和用户分组回退。
- Classic 前端格式检查与生产构建。
- 后端已有的 `TestGetUserModelsFiltersByRequestedGroup` 定向测试。
- 浏览器手工验证：粘贴/按钮选择 PNG 后发送，多分组切换时模型列表与请求
  参数同步，图片附件不写入配置。

## 验证结果

- `node --test src/helpers/groupDetails.test.mjs`：18 项通过。
- Classic 相关 Prettier 检查通过。
- Classic 相关 ESLint 检查通过。
- `go test ./controller -run "TestGetUserModels" -count=1 -timeout 60s`：通过。
- `node_modules/.bin/vite.cmd build`：生产构建成功；仅有既有的 Browserslist、
  `lottie-web` eval 和 chunk 体积提示。
- `git diff --check`：通过（仅提示测试文件的换行符转换警告）。
- Playwright 本地浏览器回归：使用拦截后的本地分组、模型和对话接口，验证按钮
  选择图片与 Ctrl+V 粘贴图片都会发送为 `image_url` data URL；切换 `VIP` 与
  `Default` 时，请求带对应 `group` 参数，模型列表分别只显示对应模型，且当前
  分组位于下拉列表第一项。

未连接真实后端执行浏览器回归，未执行生产部署或数据库变更。
