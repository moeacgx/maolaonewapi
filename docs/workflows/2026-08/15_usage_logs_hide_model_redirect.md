# 模型重定向对普通用户隐藏

## 问题与目标

普通用户在使用日志里能看到模型重定向的实际上游模型，包括模型卡片上的跳转提示、详情弹窗里的“Actual Model”，以及任务日志响应中的 `upstream_model_name` / `is_model_mapped` 这类实现细节。管理员仍需要保留这些信息用于排障和审计。

目标：普通用户只看到对外请求模型，不再直接看到重定向到的实际上游模型；管理员视图保持完整。

## 实现契约

- 普通用户的 usage logs 列表不展示实际模型提示，仅保留请求模型名。
- 普通用户的 usage logs 详情弹窗不展示“Model Mapping / Actual Model”分组。
- 普通用户任务日志卡片与详情弹窗不展示实际模型；管理员继续展示请求模型与实际模型。
- 普通用户任务日志和公开任务查询响应在 DTO 转换阶段去掉 `upstream_model_name`，并把展示模型折叠到 `origin_model_name`。
- 日志存储侧的用户可见格式继续清除 `admin_info`、`upstream_error`、`stream_status`，并额外清除模型映射调试字段。

## 兼容性与边界

- 不新增数据库字段，不改管理员路由权限。
- 管理员视图保持原始模型重定向信息，便于审计和故障定位。
- 仅影响用户可见的 usage logs / task logs 展示、任务日志响应与公开任务查询响应的用户响应整形。

## 验证

- `go test ./model ./controller ./relay -run "TestFormatUserLogsHidesIp|TestFormatUserLogsHidesModelRedirect|TestTasksToDtoHidesUpstreamModelForUserView|TestTaskModel2DtoForUserHidesUpstreamModel"`
- `npm run typecheck`
- `npm run build`
- `git diff --check`
