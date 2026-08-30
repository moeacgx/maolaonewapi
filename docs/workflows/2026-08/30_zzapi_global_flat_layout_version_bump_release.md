# zzapi 全局平铺修复版本号更新

日期：2026-08-30

## 目标

为 Classic 控制台全局全宽平铺修复发布新的 zzapi 版本，避免已发布的
`v1.0.0-rc.10.1.10.279` 只包含 Dashboard 单页修复。

## 修改

- 根目录 `VERSION` 从 `v1.0.0-rc.10.1.10.279` 提升到
  `v1.0.0-rc.10.1.10.280`。
- 本版本应包含 `classic-console-page-container` 全宽修复，以及渠道可观测性中心
  接入公共 Classic 控制台外壳的改动。

## 发布边界

- 目标环境：zzapi。
- 仅发布 Classic 前端布局修复，不修改生产 maolaoapi。
- 发布前需确认 tag、GitHub Actions、GHCR 镜像和 zzapi 容器版本一致。

## 验证计划

- `node --test`（`web/classic`）
- `npx prettier@3.6.2 --check ...`
- 受影响文件 ESLint
- `git diff --check`
