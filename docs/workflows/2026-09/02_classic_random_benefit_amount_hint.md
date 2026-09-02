# Default/Classic 随机福利额度边界提示

## 问题

Default 和 Classic 营销福利的随机额度活动会在最低面额和最高面额约束下，把总预算完整
拆分到所有份额。预算越接近服务端计算的可分配上限，随机空间越小；达到上限时，
每张券必然都是最高面额。例如 100 美元预算、10 份、最高面额 10 美元会得到 10 张 10 美元券。这是预算
约束下的正确结果，但原表单没有解释该边界，容易被误认为随机拆分失效。

## 修改范围

- 修改 Default 模板 `web/src` 和 Classic 模板 `web/classic` 的随机额度活动表单。
- 在两套模板的“可行总预算范围”下增加说明：随机额度受总预算约束；预算越接近
  可分配上限，随机空间越小，达到上限时每张券都是最高额度。
- 同步 Default 和 Classic 的 `en`、`zh`、`zh-CN`、`zh-TW`、`fr`、`ru`、`ja`、`vi` 文案。
- 不修改后端拆分算法、金额校验或数据库字段。

## 兼容性与安全边界

提示只解释既有的总预算约束，不改变提交 payload、活动状态机或券额度。后端仍以
`count * min <= total <= count * max` 校验并保证拆分后的份额总和等于活动预算。

## 验证

- `node --test web/classic/src/hooks/benefits/__tests__/benefit-contract.test.mjs web/classic/src/components/table/benefits/__tests__/activity-management-contract.test.mjs`
- `git diff --check`
- Classic 组件、测试、文档和 locale 文件的 `npx --no-install prettier --check`：通过。
- Default 组件、测试和静态 key 使用项目现有单引号、JSX 单引号、无分号参数执行
  `npx --no-install prettier --check`：通过。
- Classic `npm run build` 未执行成功，当前环境缺少 `vite` 依赖；未修改依赖或构建配置。
- Default Vitest 未执行成功，当前环境缺少 `vitest` 依赖；已先补充测试用例保护提示契约。
