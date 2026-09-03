# 对话归档扩展

## 目标

为线上安全排查提供可勾选的分组、多选用户 ID、按时间浏览和在线预览能力，同时避免普通原始 JSON 导致内存膨胀。

## 方案与契约

认证完成后先读取用户/分组上下文执行筛选；筛选未命中时不物化持久化正文。命中后将 OpenAI Chat、Anthropic、Gemini 和 Responses 的消息统一清洗为紧凑文本结构，删除媒体、base64、工具 schema、请求头和敏感凭据。消息数量、单条文本、正文总大小、筛选项数量均设置上限。常规请求在链路结束时保存，以使用 Distributor 确认后的最终分组编码；阻断请求使用预分配分组上下文。OpenAI Realtime 在连接生命周期内有界累积客户端和上游文本增量，连接结束时生成一条归档，音频和完成事件重复正文不会保存。

配置使用 `config_version` CAS，并在本地缓存 2 秒、成功更新后立即失效。筛选同时设置时采用 AND。列表只返回元数据，详情在 RootAuth 下返回清洗消息，前端按纯文本渲染。

## 兼容性与安全

新增模型通过 GORM AutoMigrate 兼容 SQLite、MySQL 和 PostgreSQL。存储密钥不进入 API 或日志；配置稳定 `CRYPTO_SECRET` 后归档正文使用 AES-GCM 加密存储，详情接口服务端解密。过期记录会从列表、详情接口隐藏，并由主节点每小时按 ID 小批量删除数据库记录。当前实现不使用外部对象存储。未知协议只保存有限摘要并标记不支持，不能借扩展接口暴露旧的原始归档正文。

## 测试计划

覆盖用户/分组 AND 筛选、媒体和工具字段清洗、归档配置 CAS、分页与大小上限、过期列表/详情隐藏与批量清理，以及三种数据库迁移路径。交付前执行 Go 测试、前端构建、Markdown 链接检查和 `git diff --check`。

## Classic 原生入口兼容修复

Classic 对话归档页曾错误复用 Default 入口所需的 React Query、`@/lib/api`、
`@/components/layout` 和 Default UI 模块。Classic `native v1` 宿主不提供这些模块，导致
入口导入阶段直接失败，页面只显示“原生扩展加载失败”，配置、列表和详情 API 均不会发起。

Classic 入口现仅依赖其稳定 SDK 契约中的 `react`、`react/jsx-runtime`、
`react-i18next` 与 `../../helpers.API`，使用本地有界请求状态实现配置读取/保存、分组筛选、
归档分页和详情预览。Default 入口、后端 API、数据模型和权限没有改动。回归测试
`web/classic/scripts/conversation-archive-native.test.mjs` 以 Classic SDK 最小契约导入入口，
防止 Default 专属模块再次混入 Classic 目标。
