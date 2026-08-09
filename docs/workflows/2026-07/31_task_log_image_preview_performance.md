# 任务日志图片预览性能优化记录

## 目标

解决打开任务日志或点击图片预览后后台整体卡顿的问题，同时保留任务日志
中的基本图片查看能力。

## 根因

- 任务日志列表查询会把图片任务的完整 `data` 字段从数据库读入内存。
- 列表序列化前还会解析完整 Base64 JSON，仅为了生成图片地址。
- 预览页面会对同一任务的全部图片并发发起内容请求。
- 每个内容请求都会重新读取并解析包含全部图片的任务正文，造成重复放大。

## 修改范围

- 新增任务日志专用查询路径：图片任务省略 `data` 大字段，非图片任务继续
  补回原有 `data`，不改变 Suno 音频预览契约。
- 图片日志列表只返回首张图片的按需内容地址。
- Default 与 Classic 预览均只加载首张图片。
- Classic 增加请求取消、重复点击保护和 Blob URL 清理；Default 保留
  AbortController 清理路径。

## 接口与兼容性

- 任务日志接口路径、鉴权和分页参数不变。
- 图片内容接口 `/api/task/:task_id/content/:index` 不变。
- 任务日志的图片预览从“全部图片”调整为“首张图片”，避免一次操作
  加载整批原图；异步图片任务 API 本身仍可按索引读取完整结果。
- 非图片任务的 `data` 仍返回给现有日志消费者。
- 过期判断仍由图片任务保留时间配置控制；图片正文不在列表响应中返回。

## 安全边界

图片内容读取继续复用原有用户归属和管理员权限校验；前端只创建当前
预览所需的 Blob URL，并在关闭、取消或卸载时释放。

## 验证结果

- `go test ./model -run 'TestTaskLog' -count=1` 通过。
- `go test ./controller -run 'TestPrepareImageTaskLog|TestGetTaskImageContent|TestReadCanvasImageTaskContent' -count=1` 通过。
- `go test ./controller -count=1` 通过。
- Default 类型检查、生产构建和两处目标文件格式检查通过。
- Classic 生产构建和目标文件格式检查通过。
- 本次改动文件的 `git diff --check` 通过。
- 固定本地测试脚本已检查 3000/3001；切换启动时被工作区已有的
  SQLite 迁移错误阻断：`request_archive_jobs` 尝试添加 `UNIQUE`
  列，导致后端在 `/api/status` 验证前退出。该迁移不属于本次任务，
  未修改固定测试库或相关迁移代码；失败后两个固定端口均未监听，
  没有遗留半启动测试进程。
