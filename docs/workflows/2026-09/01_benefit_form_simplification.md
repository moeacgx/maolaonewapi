# 福利活动表单字段收敛

## 目标

降低福利活动创建时的理解成本，避免总预算、固定面额、最低/最高面额同时出现造成
重复填写或互相冲突。

## 交互契约

- 固定面额：填写每份金额和总份数；总预算只读显示为两者乘积，并作为 `total_amount`
  提交。
- 随机面额：填写总预算、总份数、最低面额和最高面额；固定面额字段隐藏，并显示
  `最低面额 × 总份数` 到 `最高面额 × 总份数` 的可行预算范围。
- 固定模式提交的 `min_amount`/`max_amount` 置零；随机模式提交的 `fixed_amount` 置零，
  避免无关字段干扰后端校验。
- Default 和 Classic 的绑定分组选择都以名称为主，重复名称才追加 code；内部仍提交稳定
  `group_id`，不改变后端绑定契约。

## 兼容性

- 后端 API、数据库字段和金额元/quota 换算逻辑不变。
- 旧活动的金额兼容字段仍可回显；编辑草稿时根据当前面额模式显示对应字段。
- 分组列表仅过滤禁用分组；编辑已绑定但当前不可用的分组仍保留快照回显能力。

## 验证

- Default 活动表单回归测试覆盖固定总预算自动计算、随机字段显隐和范围错误。
- Classic 福利契约测试覆盖分组名称、固定/随机字段拆分。
- Default 使用 `oxfmt` 检查；Classic 使用 Prettier 检查。
- 后端金额和小时有效期契约沿用既有 `go test ./controller ./model`。

## zzapi 部署

- 目标：CloudSSH 项目“API中转站”的 `serverId=52`，Compose 目录 `/home/docker/zzapi`；
  未操作受保护的 `maolaoapi`。
- 源码归档：`maolaonewapi-benefit-form-20260901.tar.gz`，本地与远端 SHA-256 均为
  `ff062c94424b2b77ebeb1fc22c72c63ac9a75355df0ccdc913174277e64378f3`。
- 远端构建镜像：
  `ghcr.io/moeacgx/maolaonewapi:zzapi-benefit-form-20260901`，镜像 ID
  `sha256:65cb5f7306071c400ea38ef69d6abd2e062946be5adf4e84e169ffcbaf51a264`。
- Compose 备份：`docker-compose.yml.bak-benefit-form-20260901`。按
  `zzapi-slave-1`、`zzapi-slave-2`、`zzapi` 顺序逐个重建应用容器，PostgreSQL 和 Redis
  未重建。
- 三个应用容器最终均为 `running/healthy`，重启计数为 `0`；本地端口 `18097`、`18098`、
  `18099` 和公网 `https://zzapi.maolaoapi.com/api/status` 均返回
  `success=true`，业务版本为 `v1.0.0-rc.10.1.10.287`。
