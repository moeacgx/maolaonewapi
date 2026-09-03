# Classic 登录审计日志类型展示修复

## 背景

后端成功登录审计会写入普通业务日志表，日志类型为 `LogTypeLogin=7`。Classic 使用日志页的类型映射和筛选项仍只覆盖 `1-6`，导致登录成功记录在类型列显示为“未知”。

## 变更范围

- Classic 使用日志类型列补齐 `type=7` 的“登录”标签。
- Classic 使用日志筛选器补齐“登录”类型筛选项。
- Classic 使用日志详情摘要和展开行基于结构化 `other.op` / `login_method` 展示登录成功、登录方式、IP 与 User Agent。

Default 使用日志页已有 `LOGIN=7` 类型配置和登录详情渲染，本次不修改 Default 模板。

## 契约

- 后端登录日志类型保持 `7`，不改数据模型和接口。
- 历史登录日志只要带有 `type=7`，Classic 页面即展示为“登录”。
- 有结构化 `other.op.params.method` 或 `other.login_method` 时，详情展示本地化登录方式；缺失结构化字段时回退原始 `content`。

## 验证结果

- `node --test src/components/table/usage-logs/__tests__/login-log-presenter.test.mjs`：3 个用例全部通过，覆盖 `password`、`oauth:*` 和旧内容回退。
- `node --check src/components/table/usage-logs/login-log-presenter.js`：通过。
- `git diff --check`：通过。
- Classic 完整构建未执行：当前环境没有可用的 `bun`，且 `web/classic/node_modules` 不存在；本次未安装依赖或修改依赖环境。
