# 用户搜索类型契约恢复

## 变更目标

修复 GitHub Issue #109：恢复 `GET /api/user/search` 的 `search_type` 契约，
使 Classic 用户管理页选择“用户 ID”时只按用户 ID 精确匹配。

## 范围

- `search_type=id`：关键字必须是正整数且在用户 ID 数据库整数范围内，执行
  `id = ?` 精确查询；非法、负数、零或超大关键字返回空结果。
- `search_type=username`：仅对 `username` 执行原有模糊匹配。
- `search_type=all`（缺省及未知值）：保留用户名、邮箱、显示名模糊匹配，
  对合法正整数关键字额外匹配同 ID 用户。
- 保留现有分组、角色、状态、排序和分页行为，不改变管理员权限边界。

## 接口与安全边界

控制器读取并规范化查询参数 `search_type`，模型层负责最终筛选。ID 使用
32 位有符号整数解析，避免非法或超大输入进入 SQL 参数绑定；查询条件使用
GORM 参数绑定，兼容 SQLite、MySQL 和 PostgreSQL。

## 测试计划

- 模型行为测试覆盖 ID 精确、用户名模糊、all 综合搜索及非法/超大 ID。
- 控制器回归测试确认 `search_type=id` 从 HTTP 查询参数传递到模型并只返回目标用户。
- 执行相关 Go 测试、Classic 前端测试/类型检查（若环境允许）、gofmt 和
  `git diff --check`。

## 验证结果

- `go test ./model ./controller -count=1`：通过。
- `go test ./model -run 'TestSearchUsers' -count=1`：通过。
- `go test ./controller -run 'TestSearchUsersController' -count=1`：通过。
- `gofmt`、`git diff --check`：通过。
- `docs/openapi/api.json`：PowerShell JSON 解析通过。
- `go test ./...`：根包因工作树缺少 `web/classic/dist` 的 Go embed 资源而无法启动，
  其余已启动的 common 包测试通过；未为验证生成或提交构建产物。
- Classic 前端测试未执行：当前环境未安装 Bun，且 `web/`、`web/classic/`
  均无依赖目录。
