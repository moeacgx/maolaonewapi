# 使用日志 WebSocket 徽标

日期：2026-09-02

## 变更目标

使用日志此前只记录 `other.ws: true`，界面没有明确的 WebSocket 标识。现在在
Default 和 Classic 两套模板的日志列表中显示 `WS` 徽标，并在详情标题中保留该标识。

## 实现契约

- 仅当日志 `other.ws` 严格等于 `true` 时显示 `WS` 徽标。
- `is_stream` 只表示普通流式响应，不参与 WebSocket 判断。
- Default 桌面列表、移动端卡片和详情弹窗均显示徽标。
- Classic 使用日志的用时/首字列和消费/错误详情摘要显示 WebSocket 标识。
- 后端日志字段、权限和计费逻辑不变；历史日志没有 `ws` 标记时保持原有展示。

## 验证

- 新增 Default `isWebSocketLog` 判定测试和 `WebSocketBadge` 可访问性测试。
- Classic 保留纯函数测试，覆盖 `other.ws` 的 WebSocket 标识。
- 执行受影响前端测试、类型检查、Lint 和 `git diff --check`；若环境缺少 Bun，需在交付说明中记录。

## 范围与回滚

本次只改前端展示和开发文档，无数据库迁移、接口变更或生产部署。回滚对应前端提交即可，
不会影响后端日志写入或已有日志数据。
