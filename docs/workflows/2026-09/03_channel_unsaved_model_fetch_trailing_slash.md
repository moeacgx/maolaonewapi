# 未保存渠道模型获取尾部斜杠兼容

日期：2026-09-03

## 问题

Classic 渠道新建弹窗会在保存前将当前 `base_url`、`type` 和临时
`key` 提交到 `POST /api/channel/fetch_models`。当 API 地址以 `/` 结尾时，
后端继续拼接 `/v1/models`，实际请求路径变成 `//v1/models`。部分兼容上游
不会规范化该路径，导致模型获取失败。

## 根因

v243 的未保存渠道取模处理会先去除 API 地址末尾的 `/`。后续将模型探测收敛
到 `fetchChannelUpstreamModelIDs` 后，这个规范化步骤没有迁移到共享入口。
Classic 前端仍会正确提交未保存表单中的令牌和地址，因此无需修改页面请求逻辑。

## 修改范围与契约

- 在 `fetchChannelUpstreamModelIDs` 选定渠道 API 地址后去除末尾 `/`，再按渠道
  协议构造模型列表请求。
- 输入 `https://provider.example/` 时，OpenAI 兼容模型请求固定为
  `https://provider.example/v1/models`。
- 该处理只影响模型探测请求地址，不改写已保存渠道的 `base_url`，也不持久化
  新建表单的临时令牌。
- Classic 模板（`web/classic`）是本次复现范围；Default 模板未修改，依据是其
  渠道抽屉使用不同的“从上游获取”交互和组件入口。

## 安全与兼容性

- `POST /api/channel/fetch_models` 继续使用既有敏感渠道写权限；本次不放宽
  权限边界。
- 模型获取仍复用现有鉴权头构造和错误脱敏逻辑。
- 已保存渠道和未保存渠道均走共享取模器，因此两种路径均获得尾部斜杠兼容。

## 验证

- 控制器回归用例以未保存 DeepSeek 渠道、带末尾 `/` 的地址和临时令牌调用
  `POST /api/channel/fetch_models`，断言上游路径为 `/v1/models` 且 Bearer
  令牌正确透传。
- 执行：`go test ./controller -run '^TestFetchModelsUnsavedDeepSeekTrimsTrailingBaseURLSlash$' -count=1 -timeout 60s`
- 执行：`go test ./controller -run 'TestFetchModels(UsesSharedChannelFetchBehavior|UnsavedDeepSeekTrimsTrailingBaseURLSlash)$' -count=1 -timeout 60s`
- 执行：`git diff --check`
