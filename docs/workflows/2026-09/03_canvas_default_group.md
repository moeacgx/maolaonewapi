# 无限画布默认分组

## 目标

管理员可在“模型相关设置 → 全局设置”预设无限画布的默认分组，减少用户首次使用时的配置成本。

## 行为契约

- 配置键为 `global.canvas_default_group`，默认值为空字符串。
- 该配置只影响无限画布页面首次加载时下拉框的初始选中项，不锁定分组，用户仍可手动切换。
- `GET /api/user/self/groups` 仅在配置分组属于当前用户可用分组时返回 `canvas_default_group`；否则返回空值。
- 未使用预设时，前端继续按 `default`、首个可用分组的顺序回退。
- Canvas 请求仍必须携带显式 `group`，原有会话、来源和分组权限校验不变。

## 模板范围

- Default：模型设置与无限画布入口均支持该配置。
- Classic：模型设置与无限画布入口同步支持该配置。
- 两套模板共享后端配置和用户分组接口，但页面实现与验证链路独立。

## 验证

- Go：`go test ./setting/model_setting -run CanvasDefaultGroup -count=1 -timeout 60s`。
- Go：`go test ./controller -run CanvasDefaultGroup -count=1 -timeout 60s`。
- 前端：运行受影响的 Canvas 选择逻辑测试、Default typecheck/lint；环境缺少 Bun 或依赖时记录为未执行。
