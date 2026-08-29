# Classic 模型广场卡片：页脚只保留计费类型，与性能摘要对齐底部基线

日期：2026-08-29

## 目标

修复 Classic 模型广场卡片页脚的布局问题：页脚不应再显示分组信息，只保留
“按量计费/按次计费/按秒计费/动态计费”等计费类型；计费类型整体向左收拢，
并与右侧的延迟、吞吐、状态性能摘要（`ModelPerformanceBadge`）共享同一底部
基线，避免两侧内容行数不同时出现垂直错位。

（追加，2026-08-30）修复 Classic 模型广场卡片在手机端的另一处视觉问题：
性能摘要徽标（延迟/吞吐/状态，`ModelPerformanceBadge`）在窄屏下带有明显的
灰色填充底色（`background: var(--semi-color-fill-0)`），用户截图反馈非常
突兀；本次追加修复仅去掉手机端该徽标的底色，桌面端（`min-width: 460px`）
既有的透明底色与其余样式保持不变。

## 背景

`26_classic_model_plaza_card_height_alignment.md` 引入的版本在页脚计费类型
前展示了当前计价分组的显示名称（`resolveCardDisplayedGroup` +
`getGroupDisplayName` + `getGroupTextColor`）。用户截图显示该分组文案与
计费类型混排在页脚左侧，且当分组文案存在与否时，左右两栏顶部对齐
（`align-items: start`）导致底部基线随内容行数漂移，与右侧性能摘要不平齐。

## 修改范围

- `web/classic/src/components/table/model-pricing/view/card/PricingCardView.jsx`：
  - 移除页脚分组 `<span className='classic-pricing-card-group'>` 渲染及其
    `displayedGroup` 计算。
  - 移除仅供页脚分组使用的导入：`resolveCardDisplayedGroup`（来自本地
    `./card-display`）、`getGroupDisplayName`（来自 `../../../../../helpers`）、
    `getGroupTextColor`（来自 `../../groupVisuals`），以及仅供分组回退使用的
    `groupNames` prop。
  - 页脚左栏 `classic-pricing-model-card-billing` 现在只渲染
    `renderBillingTag(model)`。
- `web/classic/src/components/table/model-pricing/view/card/card-display.js`：
  仓库内已无任何调用方（`resolveCardDisplayedGroup` 只服务于上面移除的页脚
  分组渲染），确认为死代码后整文件删除。
- `web/classic/src/index.css`：
  - `.classic-pricing-model-card-footer-info`（第二处、真正生效的定义，位于
    `.classic-pricing-model-card:hover` 规则之后）的 `align-items` 由 `start`
    改为 `flex-end`，让左栏计费类型与右栏性能摘要在页脚内共享底部基线。这是
    本次唯一的实际生效 CSS 改动。
  - 未改动第一处 `.classic-pricing-model-card-footer-info`（早于 hover 规则、
    被后者覆盖的旧定义），因为它不生效，改动它不会影响实际渲染，保留原样
    以避免无意义的历史 diff。
  - 曾短暂在 `.classic-pricing-model-performance-badge` 上加过
    `flex-shrink: 0`，但该元素是 `display: grid` 的
    `.classic-pricing-model-card-footer-info` 的直接网格子项，不是 flex 子项，
    `flex-shrink` 对网格项无效，属于死 CSS；第二轮审查指出后已移除，未保留
    在最终改动中。
- `web/classic/src/components/table/model-pricing/view/card/__tests__/card-layout-contract.test.mjs`：
  - 原契约断言页脚渲染分组；改为反向契约：断言页脚源码不包含
    `classic-pricing-card-group`、`resolveCardDisplayedGroup`、
    `getGroupDisplayName(`、`getGroupTextColor(`，且不再从 `card-display`
    导入。
  - 新增契约：分组展示能力（`getGroupDisplayName`/`getGroupTextColor`）仍被
    详情面板（`ModelBasicInfo.jsx`）使用，确认收敛只发生在卡片页脚。
  - 新增契约：`.classic-pricing-model-card-billing` 与
    `.classic-pricing-model-card-footer-info` 的 CSS 规则中，前者保持
    `display: flex` + 左对齐（无 `justify-content: flex-end` 等右移属性），
    后者的生效副本使用 `align-items: flex-end`。不再断言
    `.classic-pricing-model-performance-badge` 的 `flex-shrink`（该声明已确认
    为死 CSS 并移除，没有可保护的行为）。
  - 新增契约：`renderBillingTag` 覆盖按量（`quota_type === 0`）、按次
    （`quota_type === 1` 非秒单位）、按秒（`quota_type === 1` 且
    `model_price_unit` 为秒）、动态（`billing_mode === 'tiered_expr'`）四种
    输入组合，且四个标签文案均经过 `t(...)` 包裹（保持 i18n）。
  - 新增边界契约：未识别的 `quota_type`（既非 0 也非 1）经 `renderBillingTag`
    仍返回占位符 `-`，不会抛出或渲染空白计费标签。

（追加，2026-08-30）
- `web/classic/src/index.css`：新增 `@media (max-width: 459px)` 规则，覆盖
  `.classic-pricing-model-performance-badge` 的 `background` 为
  `transparent`，与既有 `@media (min-width: 460px)` 桌面覆盖规则的断点
  （460px）互补、不重叠。未修改该选择器的基础（无媒体查询）声明本身，也
  未修改 `min-width: 460px` 区块内的任何桌面样式（`width`/
  `grid-template-columns`/`padding`/`font-size`/`text-align` 等均保持
  原样）。
- `web/classic/src/components/table/model-pricing/view/card/ModelPerformanceBadge.test.mjs`：
  新增契约用例，断言 `@media (max-width: 459px)` 区块内
  `.classic-pricing-model-performance-badge` 的 `background` 为
  `transparent`，并同时断言既有 `@media (min-width: 460px)` 区块内的
  `background: transparent` 未被移除，防止后续改动覆盖或误删桌面样式。

## 兼容性与边界

- 仅修改 Classic 模型广场卡片展示层与其直接样式；未改动模型筛选、价格计算
  （`calculateModelPrice`/`getModelPriceItems`）、详情抽屉数据契约、后端接口
  或分组 code 语义。
- 分组显示能力（名称回退、稳定着色）继续保留在详情面板
  （`ModelBasicInfo.jsx` 等），只是不再出现在卡片页脚，未删除
  `getGroupDisplayName`/`getGroupTextColor` 本身。
- 未新增或修改任何 i18n key；计费类型文案沿用既有的
  `按量计费`/`按次计费`/`按秒计费`/`动态计费`，均通过 `t()`。

## 已知限制 / 未验证项

- Classic 当前定向测试环境用 `node --test` 读取 JSX 源码做结构契约校验
  （字符串/正则匹配），未配置 React DOM 渲染测试运行时，因此无法对
  `align-items: flex-end` 在真实浏览器下的像素级基线对齐做端到端断言，
  只能确认生效的 CSS 规则文本符合预期。
- 本机未安装 `bun`，且 `web/classic/node_modules` 不存在，无法执行 Classic 的
  Prettier/Vite 构建或在浏览器中用真实视口宽度核对三列布局下卡片变窄时
  的计费类型与性能摘要是否仍保持底部基线对齐；这一响应式场景未做浏览器
  实测，风险保留待人工或有 Node 环境的后续验证。

（追加，2026-08-30）
- 同样受限于本机无 `bun`/`node_modules`，未能启动 Vite 开发服务器在真实
  浏览器中用 ≤459px 视口截图核对手机端徽标底色是否确实变为透明、是否与
  卡片其余留白协调；本次验证仅限于对 `index.css` 文本的正则契约断言，
  未做像素级或视觉回归验证。
- 未验证 459px/460px 断点附近（例如浏览器缩放导致的次像素宽度）是否存在
  两条媒体查询都不命中的边界情况；两者以相邻整数互补
  （`max-width: 459px` 与 `min-width: 460px`），写法与文件中其他成对断点
  一致，理论上无缝隙，但未做真机验证。
- 未重新核对暗色主题（`html.dark`）下该徽标背景是否也存在同样的手机端
  灰底问题；`background: var(--semi-color-fill-0)` 是主题变量，本次修复
  按 `background: transparent` 覆盖对亮/暗主题应等效生效，但未逐一截图
  确认。

## 验证结果

- `node --test src/components/table/model-pricing/view/card/__tests__/card-layout-contract.test.mjs`：4 项通过。
- `node --test`（对 `model-pricing`、`helpers`、`groupVisuals` 等相关目录下全部
  `*.test.mjs`，共 52 项）：全部通过，未发现分组、性能、计费相关回归。
- `git diff --check`：通过（见下）。

（追加，2026-08-30，性能徽标手机端底色修复）
- TDD RED：实现前运行
  `node --test src/components/table/model-pricing/view/card/ModelPerformanceBadge.test.mjs`，
  4 项中新增用例 1 项按预期失败（缺少 `@media (max-width: 459px)` 覆盖
  规则），其余 3 项通过。
- TDD GREEN：新增 CSS 规则后重跑同一测试文件，4 项全部通过。
- 回归：`node --test`（`web/classic/src` 下全部 37 个 `*.test.mjs`，共 165
  项）：全部通过，未发现回归。
- `git diff --check`（本次改动文件）：通过，无空白符问题。
