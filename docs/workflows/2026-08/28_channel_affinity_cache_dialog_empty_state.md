# 渠道亲和性上游缓存命中弹窗空态修复

## 问题

`.267` 线上 Classic 使用日志中的“渠道亲和性：上游缓存命中”标记可以显示，点击后却直接显示“暂无可展示数据”。该弹窗通过 `key_fp` 查询请求结算期间写入 Redis/内存混合缓存的统计数据。

## 根因

relaykit 协议迁移提交 `c0d3be5265` 重写 `service/channel_affinity.go` 时，正常成功路径 `MarkChannelAffinityUsed` 删除了日志 `other.admin_info.channel_affinity` 中的 `key_hint` 和 `key_fp`。后端详情接口仍要求 `rule_name` 与 `key_fp`，Classic 弹窗在 `key_fp` 缺失时按设计不发起请求，于是进入真实空态。历史 v243 及更早实现仍保留这两个字段；模板覆盖路径也未受影响，因此问题只出现在正常亲和性命中日志。

## 修改范围

- 恢复 `key_path`、`key_hint`、`key_fp` 的管理员日志字段。`key_hint` 是脱敏摘要，`key_fp` 是不可逆指纹，不写入原始会话标识。
- Classic 弹窗复用可测试的目标参数映射；支持 OpenAI/Claude 统计时保留后端明确返回的合法零值，并展示 `prompt_cache_hit_tokens`。
- Default 弹窗同步展示 `prompt_cache_hit_tokens` 和明确返回的零值；无支持统计口径或无详情记录时继续显示真实空态。

## 兼容性与安全边界

- 详情接口继续强制要求 `key_fp`，不通过放宽权限、模糊匹配或默认值掩盖历史数据缺失。
- 已落库但缺少 `key_fp` 的历史日志无法从脱敏摘要反推出原始身份，只能保持空态；新请求会生成完整索引字段。
- 统计仍按请求结算时的 OpenAI `cached_tokens`、Claude `cache_read_input_tokens`/`prompt_cache_hit_tokens` 归一化结果聚合，多实例继续共享现有 Redis 命名空间。

## 生产只读核验

通过 cloudssh-agent 精确检查 serverId `38`（美国 maolaoapi.com）：线上镜像为 `.267`，运行 `maolaoapi`、`maolaoapi-slave-1`、`maolaoapi-slave-2` 三个实例。168 小时脱敏字段计数显示主实例日志出现 `cached_tokens` 与 `cache_read_input_tokens`，未出现 `prompt_cache_hit_tokens`；两个从实例三类字段均未出现。三实例均出现 `channel_affinity` 与 `key_fp` 字段名，但 `key_hint` 未出现，和 `.267` 正常命中路径删除脱敏摘要字段的代码证据一致。仅输出字段名和计数，未读取凭据、完整日志、请求体或用户标识，未创建数据、修改数据库或重启容器；未构造具体用户的生产管理员会话。

## 测试计划与结果

- Go：覆盖 OpenAI `cached_tokens`、Claude `prompt_cache_hit_tokens`、合法零值与缺失 usage 的聚合语义，以及日志字段契约。
- Classic Node 测试：覆盖详情 API 目标字段映射、缺失 `key_fp` 的真实空态条件、显式零值与缺失字段区分。
- 发布前执行受影响 Go 测试、Classic/Default 前端类型检查与构建、`git diff --check`。
