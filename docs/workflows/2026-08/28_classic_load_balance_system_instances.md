# Classic 负载均衡多节点实例展示

## 背景

zzapi 已按多节点负载均衡方式部署，Default 前端已经可以在系统信息页通过
`GET /api/system-info/instances` 查看各应用实例的心跳、角色和资源占用。
Classic 后台的性能设置页只展示本节点 `/api/performance/stats`，缺少跨节点视图。

## 变更范围

- Classic `/console/setting?tab=performance` 新增“多节点实例”面板。
- 面板复用已有系统实例接口，不新增后端接口或数据库字段。
- 展示节点名称、hostname、在线/失联状态、master/worker 角色、CPU、内存、磁盘、
  版本、运行环境、启动时间和最近心跳。
- 每 30 秒自动刷新；仅允许通过确认弹窗删除后端判定为失联的实例记录。

## 数据契约

- 列表接口：`GET /api/system-info/instances`
- 删除全部失联实例：`DELETE /api/system-info/stale-instances`
- 删除单个失联实例：
  `DELETE /api/system-info/instances/{node_name}`
- Classic 只消费 `info.resources` 中的 CPU、memory、storage 字段；缺失时显示 `-`，
  不伪造性能数据。

## 验证计划

- 运行 Classic 原生 Node 回归测试，确认面板使用系统实例接口且不再引用模型性能摘要。
- 运行 Classic 构建，确认新增 JSX 与 i18n 文案可编译。
- 运行 `git diff --check` 检查空白。
