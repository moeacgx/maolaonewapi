# 分组图标选择与展示

## 变更目标

为用户分组增加可持久化的 AI 厂商图标，使管理员可以在分组设置中从精选
`@lobehub/icons` 图标库选择图标，并在 Default 与 Classic 的分组选择器中显示。

## 范围与契约

- `groups.icon` 使用与供应商相同的图标名称字符串，最长 128 个字符；空值表示未设置。
- 管理端 `GET|PUT /api/group/details` 的分组对象和保存请求增加 `icon`。
- 用户 `GET /api/user/groups`、`GET /api/user/self/groups` 的分组投影增加 `icon`；虚拟
  `auto` 使用 `Layers` 作为默认展示图标，不写入实体分组表。
- 渠道、令牌的 `group_details` 引用增加 `icon`，旧客户端仍可只使用 `id/code/name`。
- 旧管理客户端省略 `icon` 时保留数据库原值；显式传空字符串才会清除图标。

## 实现

- 后端在 `Group`、`GroupConfig`、`GroupReference` 中传递图标，现有 GORM `AutoMigrate`
  会为 SQLite、MySQL、PostgreSQL 补齐 `icon` 列。
- Default 在分组倍率设置表中提供可搜索的精选图标弹层，并在 API Key、Playground、模型组
  选择器、渠道分组和渠道列表徽标中渲染图标。API Key/福利活动等共享分组选择器中，已选项
  和待选项均将图标与分组名称放在同一行，并统一使用紧凑的 14px 图标；描述保持在名称下方。
- Classic 在分组倍率表中提供可搜索图标弹层，并在令牌、渠道、标签和操练场分组选择器中
  渲染图标。未设置时图标列显示带加号的“选择图标”按钮；点击后从精选库选择并点击页面
  底部“保存分组相关设置”持久化。已设置时该按钮显示实际图标，仍可点击更换或清除。
- 图标仍通过各模板已有的 LobeHub 图标渲染器解析；未知名称使用既有兜底，不加载外部图片。

## 安全与兼容性

- 图标只保存名称，不接受远程 URL、HTML 或 SVG 内容。
- 分组授权、计费、渠道选择和稳定 ID 绑定逻辑不变；图标仅为展示属性。
- `auto` 是虚拟分组，不创建实体记录。

## 验证

- Go：分组图标持久化、请求字段省略/显式清除语义和现有 Group/Controller 回归测试。
- Classic：`node --test src/helpers/groupDetails.test.mjs src/group-icon-integration.test.mjs`。
- Default：新增图标目录/分组转换测试；当前环境没有 Bun，无法运行 `bun test`、typecheck
  或构建，应在具备 Bun 与依赖的环境补跑。
- 通用：`git diff --check`。
