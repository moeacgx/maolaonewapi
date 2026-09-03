# Classic 安全审计策略来源加载修复

## 问题与根因

`policy_action_sources` 在 Classic 安全审计配置和内置策略接口中以数组返回。
Classic 的归一化逻辑把允许来源定义为 `Set`，随后却调用数组方法
`.filter()`。首次加载配置时因此抛出 `TypeError`，顶层页面无法建立草稿，
所有安全审计标签页都停留在加载失败状态。

## 修复范围

- Classic 将允许来源保留为有序数组，另建 `Set` 只用于成员校验。
- 归一化继续忽略未知值、去重并按 `cyber_policy`、`biological_risk` 的稳定顺序输出。
- 累计列使用新的通用上游策略字段，同时继续读取历史
  `user_cyber_policy_count` 和 `cyber_policy_window_hours` 响应字段。

Default 使用数组常量再调用 `.filter()`，不受本问题影响。

## 验证

- 回归用例以包含 `biological_risk` 的真实数组输入复现了原始
  `TypeError`，修复后通过。
- `node --test web/classic/src/security-audit-*.test.mjs`：33 项通过。
- `git diff --check`：通过。

本地未安装 Classic 前端依赖，未执行本地 Vite 构建；当前提交的 CI 已验证
Classic `bun install --frozen-lockfile` 和 Vite 构建。
