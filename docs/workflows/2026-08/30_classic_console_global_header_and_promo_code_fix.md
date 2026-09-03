# Classic 控制台全局顶栏统一与营销福利 404 修复

日期：2026-08-30

## 问题

Classic 控制台只有数据看板首页复用了模型广场同款模板顶栏，其他
`/console/*` 页面以及通知中心仍然走旧顶栏，导致控制台全局视觉不统一。与此同时，
“营销福利”页的优惠码列表、搜索、保存、状态切换和删除请求仍调用
`/api/promo-code/...`，而后端实际注册的是 `/api/promo_code/...`，页面因此报
404。

## 修改

- `web/classic/src/components/layout/headerbar/index.jsx`：将经典版模板顶栏的控制台
  判定扩展为所有 `/console` 段路径和通知中心壳页面，让控制台共享数据看板同款头部壳。
- `web/classic/src/components/layout/PageLayout.jsx` 与
  `web/classic/src/hooks/common/useHeaderBar.js`：统一使用路径段判断，避免
  `/consolex` 等相邻路径误套用控制台布局。
- `web/classic/src/components/layout/headerbar/consoleHeaderBehavior.js`：桌面侧栏
  入口固定不渲染，移动端继续保留抽屉入口，不再支持 `showOnDesktop`。
- `web/classic/src/components/layout/headerbar/__tests__/console-header-contract.test.mjs`
  与 `pricing-template-header.test.jsx`：收紧契约，明确所有控制台页面都走模板顶栏，
  且桌面不渲染侧栏按钮。
- `web/classic/src/hooks/promo-codes/usePromoCodesData.jsx`：将优惠码接口路径改回
  后端实际的 `/api/promo_code/...`。
- `web/classic/src/hooks/promo-codes/__tests__/usePromoCodesData-contract.test.mjs`：
  以 source-contract 形式锁定优惠码请求路径，防止再次写成连字符版本。

## 范围

- 仅修改 Classic 模板。
- 不修改 Default 模板、后端路由、数据库、权限、计费或生产配置。

## 兼容性

这是展示层和前端请求路径修复，不改变优惠码业务模型、接口返回结构或控制台
权限边界。桌面控制台仍保留默认展开侧栏，移动端仍只显示抽屉按钮；通知中心
继续沿用同一壳结构。

## 验证结果

- 通过 `node --test` 运行控制台顶栏和优惠码路径契约测试：6 项通过。
- 通过 `go test ./router -run '^TestPromoCodeAdminRoutesAreRegisteredAndRejectNonAdmins$'
  -count=1 -timeout 60s`，确认后端路由存在且非管理员仍被拒绝。
- 通过 `git diff --check`，并确认新版 `web/src` 没有差异。
- 未运行 Classic 的 Bun 测试、格式检查和构建：当前环境没有 Bun，且
  `web/classic/node_modules` 不存在。
