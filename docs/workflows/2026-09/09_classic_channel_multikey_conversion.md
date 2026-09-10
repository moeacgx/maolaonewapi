# Classic 编辑渠道启用密钥聚合

## 目标与范围

Classic 编辑已有单密钥渠道时允许显式切换到密钥聚合模式，保留渠道 ID、分组和其他配置。
Default 页面不在本次范围内；两套前端继续共享已有多密钥渠道管理能力。

## 接口与安全契约

- `PUT /api/channel/` 新增可选 `convert_to_multi_key: true`，仅显式启用才转换，缺省或 false 保持原行为。
- 转换需 `ChannelSensitiveWrite` 权限；客户端提供的 `channel_info` 不能替代该开关或覆盖服务端状态。
- `key_mode` 默认 `append`，保留原密钥并追加每行一个的新密钥；`replace` 必须提供非空新密钥列表。
- 转换时清除空行、去除首尾空白、按原顺序去重；`multi_key_mode` 为 `random` 或 `polling`，缺省为 `random`。
- 不提供多密钥转单密钥；重复转换请求不能重置已有密钥的禁用状态。
- 关闭 Classic 编辑抽屉时重置聚合、策略和来源状态，避免随后新建渠道继承本次选择。
- Codex OAuth、Vertex AI 和 io.net 的单密钥转换不在本期支持范围内；同时检查存量类型和请求目标类型，防止省略或切换类型绕过限制。
- 只复用现有渠道更新、GORM 持久化和缓存刷新，不新增数据库结构。响应和管理审计不返回密钥内容。

请求示例（示例值不是实际凭据）：

```json
{
  "id": 123,
  "convert_to_multi_key": true,
  "key_mode": "append",
  "multi_key_mode": "random",
  "key": "example-key-2\nexample-key-3"
}
```

## 实现与验证计划

在 Classic 原有密钥区域增加编辑转换开关，复用多行输入、追加/覆盖、随机/轮询控件。
后端验证转换权限、来源与目标类型、模式和密钥列表，再复用现有更新路径。
回归覆盖追加、覆盖、空白/重复行、留空保留、缺省不转换、权限拒绝、特殊凭据和重复转换。
前端验证编辑入口、提交载荷和开关状态；执行受影响测试、格式检查及 Classic 构建。

## 发布与回滚

需要后端与 Classic 同版本发布才能使用新入口；未升级的 Default 仍可管理转换后的渠道。
旧后端已支持多密钥读取，因此代码回滚无需迁移渠道数据，但会失去编辑转换入口。
本任务不部署线上实例。

## 验证结果

- 后端先运行新增回归得到失败，再实现转换；`go test ./controller -count=1 -timeout=60s` 通过。
- 新增前端 6 项行为/提交契约测试通过：`node --test web/classic/src/components/table/channels/modals/__tests__/multi-key-editing.test.mjs`。
- 修改的 Classic JavaScript/JSX 通过 ESLint；新增模块、测试及 Markdown 通过 Prettier 检查。
  原有 `EditChannelModal.jsx` 全文件存在大量既有格式差异，本次保留其原格式并仅调整修改区域，避免无关重排。
- Classic 生产构建通过；浏览器依赖数据过旧、第三方 `eval` 和大包提示属于现有构建警告。
- 本地 Playwright 使用模拟 API 加载真实 `EditChannelModal`，确认编辑入口出现、勾选后显示多行输入、
  追加提示和随机/轮询选择；未完成浏览器提交到真实后端的端到端验证。
- 独立只读审查发现关闭后的状态残留并已修复；此生命周期补丁经过 lint 和构建，尚未完成浏览器重开验收。
- `git diff --check` 通过；文档索引及 JSON 请求示例检查通过。
- 未修改 Default，也未修改 relaykit；没有数据库迁移或线上部署。
- 共享契约影响：管理 API 增加显式可选转换字段，旧客户端缺省行为不变；推理请求、usage、计费契约不变。
- 并发限制：沿用现有渠道更新的读后写语义，未新增并发追加冲突保护，避免多个管理端同时编辑同一渠道。
