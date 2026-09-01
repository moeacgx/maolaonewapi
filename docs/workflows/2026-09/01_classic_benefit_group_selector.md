# Classic 时效额度券绑定分组选择器

## 问题

Classic 营销福利的时效额度券创建/编辑表单将绑定分组暴露为 `group_id` 数字输入，
管理员需要先查找内部 ID，且与令牌编辑时使用的分组名称选择体验不一致。

## 修改范围

- 活动编辑抽屉打开时从 `/api/group/details` 加载分组详情。
- 将“绑定分组 ID”输入改为可搜索的单选分组下拉，显示分组名称、code 和描述。
- 选项内部使用分组数字 ID，保存时继续提交 `group_id`，后端活动、领取和计费契约不变。
- 若编辑活动时当前快照分组已不在详情列表，保留快照名称作为临时选项，避免误清空绑定。

本次只修改 Classic 模板；Default 模板、后端接口、活动状态机和计费逻辑不在范围内。

## 验证

- `node --test web/classic/src/hooks/benefits/__tests__/benefit-contract.test.mjs`
- `git diff --check`
- 发布前执行 Classic 前端构建。
