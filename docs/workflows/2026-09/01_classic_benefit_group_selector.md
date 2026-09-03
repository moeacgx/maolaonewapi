# Classic 时效额度券绑定分组选择器

## 问题

Classic 营销福利的时效额度券创建/编辑表单将绑定分组暴露为 `group_id` 数字输入，
管理员需要先查找内部 ID，且与令牌编辑时使用的分组名称选择体验不一致。

## 修改范围

- 活动编辑抽屉打开时从 `/api/group/details` 加载分组详情。
- 将“绑定分组 ID”输入改为可搜索的单选分组下拉，以分组名称为主；名称重复时追加
  code，不展示长描述或内部 ID。
- 选项内部使用分组数字 ID，保存时继续提交 `group_id`，后端活动、领取和计费契约不变。
- 若编辑活动时当前快照分组已不在详情列表，保留快照名称作为临时选项，避免误清空绑定。

本次记录的初始修复只修改 Classic 模板；Default 模板随后在福利活动表单收敛工作中同步了
相同的分组选项规则。后端接口、活动状态机和计费逻辑均未改变。

## 验证

- `node --test web/classic/src/hooks/benefits/__tests__/benefit-contract.test.mjs`
- `git diff --check`
- 发布前执行 Classic 前端构建。
