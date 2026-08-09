# Classic 屏蔽词指定分组保存校验修复

## 问题

Classic 安全审计页面的屏蔽词规则选择“指定分组”并选中有效分组后，规则摘要已经显示
一个分组，但页面仍提示启用规则缺少目标并禁止保存。Default 页面不受影响。

## 根因

Classic 的目标校验先确认指定分组不为空，随后仍继续执行渠道目标校验。因为分组规则
本来不需要填写 `channelIds`，空渠道列表被错误识别为校验失败。该问题只影响前端保存
按钮状态，不改变已保存规则、后端配置结构或运行时匹配行为。

## 修改范围

- 渠道列表非空校验仅在 `targetType=channels` 时执行。
- `targetType=groups` 只校验 `groupCodes`，选择一个或多个有效分组后允许保存。
- `targetType=all` 继续无需选择具体渠道或分组。
- 保留旧 `channel_tags` 配置的兼容校验，不新增此类配置入口。
- Default 已使用互斥目标判断，无需修改。

## 兼容性与安全边界

接口、数据库、`SensitiveRules` JSON 和权限边界均不改变。后端仍会独立验证启用规则的
目标范围，因此前端修复不会降低配置安全性。

## 验证计划

- Classic 专项测试覆盖空分组被拦截、非空分组通过、空渠道被拦截、非空渠道通过及
  全部渠道通过。
- 执行 Classic 目标文件格式检查和生产构建。
- 执行 `git diff --check`。

## 验证结果

- Classic 屏蔽词与安全审计专项测试 9 项全部通过。
- Default 对应目标校验测试 7 项全部通过，确认无需同步代码修改。
- Classic 目标文件 ESLint、Prettier 检查和生产构建通过；构建仅包含仓库既有的
  Browserslist、`lottie-web` 和大分块警告。
- `git diff --check` 通过。
- 固定本地环境未能启动：既有 `tmp-local-v10101.db` 在迁移
  `request_archive_jobs.dedupe_key` 时触发 SQLite
  `Cannot add a UNIQUE column`。按照固定数据源约束未改用临时数据库；该迁移问题与
  本次纯前端校验修复无关。
