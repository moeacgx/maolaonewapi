# Classic 模型详情动态计费分组价格与对比度

日期：2026-09-08

## 问题

Classic 模型广场点开模型详情后，定价卡片整体过淡，动态计费分组只显示
“见上方动态计费详情”，没有按分组倍率算出实际单价。

## 根因

- 详情抽屉沿用 Semi 的 `text-2`/`text-3` 和近白底卡片，标题、折扣徽标和表格
  表头对比不足。
- `ModelPricingTable` 在 `billing_mode === 'tiered_expr'` 时把分组价格列替换成
  提示文案，没有把分档单价乘以分组倍率。

## 修改范围

- 仅 Classic 模型广场详情抽屉（`web/classic`）。Default 模板不在范围内。
- 动态计费基础价格展示首档官方单价；分组区按档位展开
  `档位单价 × 分组倍率`，并跟随页面货币/充值展示与 token 单位。
- 无法解析的特殊表达式显示警告，而不是空表或“见上方”占位。
- 加强详情卡片标题、价格块、表格、折扣徽标和动态计费分区的对比度。
- 标题旁「动态计费」只用琥珀色文字；模型信息区的计费类型保留彩色徽标；分档名使用蓝色档位标签。

## 兼容性与安全边界

- 不改变计费表达式、分组倍率或后端结算。
- 分组价格只用于展示；实际扣费仍走原有预扣费/结算链路。
- 非动态计费模型仍使用原来的分组价格表。

## 验证

- `node --test web/classic/src/components/table/model-pricing/billing/utils.test.mjs`
- `node --test web/classic/src/components/table/model-pricing/model-pricing-visual-contract.test.mjs`
- `node --test web/classic/src/components/table/model-pricing/billing/__tests__/i18n.test.mjs`
- `node --test web/classic/src/group-display-name-integration.test.mjs`

## 发版

- PR：https://github.com/moeacgx/maolaonewapi/pull/194 ，已合并到 `custom-main`。
- 版本号：`v1.0.0-rc.10.1.10.318`。
- 发布提交：`c05667bde`；标签：`v1.0.0-rc.10.1.10.318`。
- GitHub Actions：Linux Release `34243580749`、Docker Multi-arch `34243580746`，均成功。
- GHCR 多架构清单：`sha256:6bfa91b376f9e75c791d4e55412de669dda015081b94958e016d04c265fcc873`。
- 镜像：`ghcr.io/moeacgx/maolaonewapi:v1.0.0-rc.10.1.10.318`。
- 部署目标：仅 CloudSSH `serverId=52` / `hostId=17` 的 zzapi 三应用容器
  （`zzapi-slave-1` → `zzapi-slave-2` → `zzapi`）。
- 明确不更新：`zzapi-postgres`、`zzapi-redis` 和 `maolaoapi`。

### 远端更新证据

- Compose 备份：`docker-compose.yml.bak-v1.0.0-rc.10.1.10.318`，
  SHA-256 `4fbc7b46994af179b3c2c8a242cbacb7e0a96431e5609169091b77aae206c929`。
- 更新后 Compose SHA-256
  `7cb67b0ae4a51b2a004e64407a99fa9acbab972de07109971e371811b4a82cd7`。
- 滚动作业：`zzapi-slave-1` 为 `962d86f6-e7c4-46e4-9405-8a3a5ce335be`，
  `zzapi-slave-2` 为 `476b1193-f07c-412d-bda5-176cafba7908`，
  `zzapi` 为 `3124db33-47cc-48b8-9e1a-6b6b15efb4b1`。
- 终验作业：`dcfca1c4-ef8a-4fe5-bcc0-4560c45390c6`。
  端口 `18097/18098/18099` 与公网 `/api/status` 均返回 `.318`；
  三个应用 `running/healthy`、重启次数 `0`；PostgreSQL 与 Redis 保持原镜像且重启次数 `0`。
