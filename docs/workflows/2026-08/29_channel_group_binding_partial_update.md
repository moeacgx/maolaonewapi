# 渠道分组绑定部分更新保留修复

## 根因

`v1.0.0-rc.10.1.10.243`（提交 `2b538c8e98a14004b8948b6e5fb7482cfeb32941`）的
`model/channel.go` 通过
`groupSelectionProvided` 区分“请求包含分组字段”和“仅更新其他字段”。
`v1.0.0-rc.10.1.10.244`（提交 `d182efadc57279807eeb8807bbc66b886e2458d5`）
合并官方基线时删除了这段逻辑。当前版本的
`Channel.Update()` 因而无论请求是否包含分组，都先以空的 `GroupIds`/`Group`
准备绑定，再重写 `channel_groups` 和 `abilities`。

Classic `useChannelsData.jsx` 与 Default `channel-actions.ts` 的快捷编辑只发送
`{id, priority}`、`{id, weight}` 或 `{id, concurrency_limit}`。这些合法的部分
PUT 会因此清空已有渠道分组，迁移完成后 `channel_groups` 又是 API 的权威来源，
最终表现为接口返回空分组或前端显示“用户分组”。

## 修复

- `Channel.Update()` 恢复事务内 presence-aware 逻辑：分组字段省略时从当前
  `channel_groups` 重新加载并保留 `Group`、`GroupIds`、`GroupDetails`。
- 显式提交非空 `group_ids` 或 `group` 仍经过现有解析、禁用分组校验和绑定替换。
- 显式 `group_ids: []` 或仅提交 `group: ""` 由 controller 拒绝；JSON `null`
  按未提供处理。部分更新不会因为 DTO 的零值切片误判为显式空分组。
- 分组行和渠道行在同一事务内使用现有 GORM `lockForUpdate` 规范锁定；SQLite
  跳过 `FOR UPDATE`，依靠单写者模型，MySQL/PostgreSQL 使用行锁。
- multi-key 更新复用同一事务路径，重新计算 `MultiKeySize` 不会改变渠道分组
  绑定或能力记录。

## 接口契约

对 `PUT /api/channel/`：

- 省略 `group` 与 `group_ids`：保留当前渠道分组、`channel_groups` 记录、API
  返回的 `group`/`group_ids`/`group_details` 以及 abilities 的 `Group`/`GroupId`。
- 提交非空 `group_ids` 或非空 `group`：替换当前绑定并重建对应 abilities。
- 提交 `group_ids: []` 或仅提交空字符串 `group: ""`：返回失败，不修改旧绑定。
- 提交 `group: null` 或 `group_ids: null`：视为未提供，保留旧绑定。

## 兼容性

修复只使用 GORM 查询、事务和现有锁辅助函数，兼容 SQLite、MySQL 和
PostgreSQL。迁移完成后 `channel_groups` 继续作为分组权威来源；本次不修改
迁移脚本、表结构或 Classic/Default 前端请求格式。

## 验证

- 模型测试覆盖普通 priority 部分更新、multi-key weight 部分更新、显式替换和
  显式空分组拒绝，并检查绑定与 abilities。
- controller 测试直接发送 Classic/Default 等价的三种单字段 PUT，检查响应分组
  字段、`channel_groups` 和 abilities；另测 multi-key、显式替换、空数组和空字符串。
- 已执行 `gofmt`、`git diff --check`、`go test ./model -count=1 -timeout 60s`
  和 `go test ./controller -count=1 -timeout 60s`，均通过。
- `go test ./... -count=1 -timeout 60s` 的其他包均通过；根包 setup 因当前工作树
  缺少 `web/classic/dist` embed 目录失败，未伪称为完整根包通过。
