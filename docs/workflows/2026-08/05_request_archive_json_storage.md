# 请求归档改用可审阅 JSON 存储

## 变更目标

完整请求归档是 Root 管理审计数据，不再需要为新正文生成 AES-GCM 信封。新任务应写成
可以直接使用文本工具审阅的标准 JSON 文件，同时保持 HTTP、Realtime 文本和二进制帧
的原始字节可精确还原。

## 范围

- 新请求归档任务固定使用 `request_cipher_format=json_v1`，不再根据
  `CRYPTO_SECRET` 切换正文格式。
- 本地和 S3/R2 对象使用 `.json` 后缀；S3/R2 Content-Type 为
  `application/json`。
- Guard 节点令牌、S3/R2 访问凭据和删除确认令牌仍依赖稳定的
  `CRYPTO_SECRET`，本次不降低这些凭据和管理操作的保护。
- 历史 `plain_ra1`、`ra1`、`ra2`、`ra3` 数据不迁移、不改名，继续通过原 `.enc`
  对象键读取和清理。没有原密钥时，历史加密任务继续等待密钥恢复。

## JSON 契约

对象顶层格式标识为 `request_archive_json_v1`，包含：

- 归档 ID、可选审计事件 ID、目标 ID、配置版本和创建/过期时间；
- 去除 query/fragment 的请求路径、请求 ID、方法和 Content-Type；
- 用户、令牌和分组的管理元数据；
- 原始正文 SHA-256 和字节数；
- `body_encoding` 及对应正文。

可直接审阅且不超过 64 MiB 的 UTF-8 正文使用 `body_encoding=utf8` 和 `body` 字符串。
二进制、包含不适合直接展示的控制字符、超过可读阈值，或 JSON 转义后明显大于
Base64 的正文使用 `body_encoding=base64` 和 `body_base64`。编码转换不得改变前后空白、
JSON 标量、文本帧或二进制帧的原始字节。

归档仍禁止保存 Authorization、Cookie、除 Content-Type 外的请求头，以及 URL query
和 fragment。JSON 文件是明文管理数据，部署方必须通过本地文件权限、S3/R2 IAM、保留期
和备份策略限制访问，不能公开对象地址。

## 兼容与清理

Worker 在没有 `CRYPTO_SECRET` 时可以处理本地目标的 `json_v1` 和历史
`plain_ra1`。S3/R2 凭据仍受密钥保护，因此无密钥时这些目标的任务与历史加密任务都
保持排队且不消耗重试次数。Worker 会在数据库查询层筛选可投递任务，队首 S3/R2 或
历史密文积压不会遮挡后续本地 JSON。对象摘要解析同时接受 `.json` 和 `.enc`，以维持
S3 版本协调、幂等重试和过期对象精确删除。未知格式继续按历史加密数据处理，不能猜测为
JSON。

## 内存与部署

UTF-8 正文直接流式转义到最终 JSON Builder，Base64 正文也直接流式编码，避免先生成
完整 Marshal 缓冲再复制成字符串。请求入队和 Worker 按正文四倍加固定余量共享
384 MiB 并发预算，覆盖最终 JSON、解码和数据库驱动短暂副本。约 95 MiB 以上正文会
占满预算并独占处理；该预算约束并发，不是 Go 堆峰值的严格上限。

`json_v1` 是新的队列格式，旧 Worker 不识别。滚动升级前必须先停止旧 Worker，再整体
升级所有实例；回滚前必须先关闭归档，由当前版本 Worker 排空 `json_v1`，确认没有待处理
任务后再停止 Worker 并恢复旧版本。已经写出的 JSON 与历史 `.enc` 对象均不自动删除或迁移。

## 验证

- 有无 `CRYPTO_SECRET` 时，新任务都写成 `json_v1` 和 `.json`。
- UTF-8 正文、前后空白和 Realtime 二进制正文可精确往返；HTML 字符不发生六倍转义，
  控制字符和转义膨胀过大的正文回退到 Base64。
- 历史 `plain_ra1` 仍投递为 `.enc`，历史加密任务无密钥时仍保持排队。
- 无密钥时本地 JSON 正常投递，S3/R2 JSON 任务保持排队且不增加尝试次数。
- JSON 对象键、摘要解析、幂等写入和 S3 Content-Type 保持一致。
- 路径 query、Authorization、Cookie 和其他原始请求头不会进入对象。
- 执行请求归档定向测试、Go 格式化和 `git diff --check`。
