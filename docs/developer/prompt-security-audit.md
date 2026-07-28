# 安全审计

## 目标与边界

安全审计是 new-api 的内置 Root 管理能力。它不是扩展模块，也不属于系统设置；
Default 与 Classic 前端均提供独立的“安全审计”一级菜单。页面统一管理四条相互
独立的审计链路：本地屏蔽词过滤、上游安全策略事件、可选的 Qwen3Guard 主动
提示词分类，以及可选的完整客户端请求异步归档。Guard 关闭或没有配置节点时，
本地屏蔽词、上游策略事件和请求归档仍可独立工作。

本能力参考 `Wei-Shaw/sub2api` 的 Content Moderation 与 Prompt Audit 行为重新
实现。实施时固定的上游 `main` HEAD 为
`2e432173f76c351375d18bbdd9e748cce998891c`，当时最新发布版本为
`v0.1.166`；上游许可证为 LGPL-3.0。new-api 适配采用 GORM、跨数据库 CAS、
加密持久化和双 React 前端，不引入上游的 PostgreSQL 专用 SQL 或 Vue 组件。

主动 Guard 只审核客户端可控的文本提示词。图片、音频和其他二进制内容本身不做
内容识别，但相关文本提示、说明、转写文本和 Realtime 文本帧仍会审核。任务类文本
字段包含音频 `ref_text`、Suno 风格 `tags` 和图片水印文字；聊天历史中的兼容
`reasoning_content` / `reasoning`、Claude thinking 块以及 Gemini 可执行代码上下文
同样按客户端可控文本处理。

上游安全策略不是本地语义模型。它只在上游已经返回明确的
`error.code=cyber_policy` 或 `response.error.code=cyber_policy` 后生成事后事件，
不得把普通 400、关键字包含或模糊错误文案误判为 `cyber_policy`。本地屏蔽词继续
沿用现有规则、屏蔽词组、请求/响应作用域以及 block/mask 行为，并支持每条规则
独立选择具体渠道或整个渠道分组。这里的“渠道分组”严格指
渠道管理用于聚合渠道的 `Channel.Tag`，不是用户或令牌的所属分组；持久化使用
规范化后的标签字符串。规则选择整个渠道分组后，后续新增的同标签渠道自动进入规则
范围；已经没有真实渠道使用的标签不会生成可选分组，也不会参与运行时匹配。
既有 Option 存储不做破坏性迁移。

## 安全模型

- Guard 模式为 `off`、`async_audit`、`blocking`，默认 `off`；该模式不再充当
  整个安全审计的总开关。
- `upstream_policy_enabled` 和 `sensitive_word_audit_enabled` 默认开启，分别控制
  上游 `cyber_policy` 与屏蔽词命中是否写入统一事件。二者不要求 Guard 节点或
  审核 API。
- 上游 `cyber_policy` 自动封禁默认关闭。开启后只统计已经成功持久化且
  `source=upstream_policy`、`error_code=cyber_policy` 的精确事件；默认阈值为
  10 次、滚动窗口为 720 小时，阈值设为 1 表示首次命中即封禁。只允许禁用
  普通用户，管理员和 Root 永不自动封禁。
- 管理 API、页面入口、配置、节点探测、原文查看和删除均仅限 Root。
- 完整审计文本和 Guard 节点令牌使用 AES-256-GCM 版本化密文保存；列表只返回
  脱敏预览、哈希、字符数和技术元数据。
- 完整请求归档保存鉴权后的客户端 HTTP 原始正文，以及 Realtime 客户端的文本 JSON、
  二进制 JSON 和原始二进制音频帧。它不保存 Authorization、Cookie、除
  Content-Type 外的其他请求头或 URL query/fragment；归档发生在屏蔽词 mask、
  Guard、协议转换和上游写入之前。
- 请求正文先以独立 AES-256-GCM 密钥域和任务绑定 AAD 写入持久队列，再由 Worker
  异步投递到本地目录或 S3 兼容对象存储。Cloudflare R2 按 S3 兼容 endpoint 配置，
  不需要单独的存储类型，但版本探测和删除会采用 R2 兼容路径。对象内容始终是密文
  信封，密文不能交换到其他任务解密。
- 每个归档目标有稳定 ID；切换活动目标只影响新任务。保留期内存在队列任务或对象时
  禁止复用同一目标 ID 修改路径、bucket、prefix、endpoint 或访问密钥，必须新增目标
  后再切换，避免精确清理误删或遗留旧对象。
- 启用 Guard、完整请求归档或保存 Guard/对象存储令牌前必须显式配置稳定的
  `CRYPTO_SECRET`。密钥丢失或
  更换后，已有密文无法恢复；轮换前必须先完成数据重加密。本地/上游策略事件在
  没有密钥时仍记录不可逆哈希、长度、来源、处置和技术元数据，但不保存可还原的
  正文或正文预览，详情明确返回“正文未保存”。
- 密钥丢失时管理页仍可加载，并把旧节点令牌标记为 `unreadable`，便于 Root
  关闭审计、清除旧令牌或替换令牌；请求热路径不会因此降级放行。
- 节点启用状态显式持久化。禁用节点不参与请求门禁；其旧令牌即使不可读，也只
  影响该节点自身的 `keep` 和探测操作，不会拖垮其他启用节点或异步 Worker。
- 默认保留事件和含密文任务 30 天，后台分批清理；即使审计切回 `off`，超过
  保留期的排队或重试任务也会清除并同步修正队列容量。默认不保存 Safe 事件。
- 管理日志只记录配置版本、节点 ID、删除数量等脱敏摘要，禁止记录提示词、
  Authorization、节点令牌、密文或 Guard 完整响应。
- Guard HTTP 客户端不继承环境代理、不跟随重定向，并限制响应体；允许 Root
  配置本机和内网节点，但拒绝云元数据、link-local、userinfo、query 和 fragment。

## 请求流程

HTTP 请求固定按认证与可复用正文快照、现有敏感词规则预检、可选 Prompt Guard、
渠道分配、计费、上游请求的顺序执行。内置敏感词规则不依赖 Guard；即使 Guard
关闭或没有配置节点，也会在渠道分配前独立执行。固定渠道令牌按该渠道精确选择
规则；动态选渠时，指定渠道规则只要目标渠道属于当前路由候选集并支持请求模型，就按
安全优先语义在分配前执行，避免为了确定最终随机渠道而先占用并发。渠道分组规则按
候选渠道的 `Channel.Tag` 匹配，再与当前路由候选集和模型能力求交；不会把用户分组、
令牌分组或请求选择的路由分组直接当成规则目标。数据库预检同时要求渠道真实存在且
处于启用状态，孤儿能力记录和禁用渠道均不能触发规则。候选集或模型无法可靠解析时采用
fail-safe 预检。响应、SSE 和 Realtime 上游帧按当前尝试实际选中的渠道及该渠道的
`Channel.Tag` 精确选择规则；跨渠道重试必须同步刷新选中渠道快照。
正文快照在敏感词规则执行前生成；即使规则采用 mask 修改了实际转发正文，Guard、
提示词哈希和加密事件仍基于客户端提交的原始文本，数据库中不会写入其明文。
Chat、Claude Messages、Responses 和 Gemini 请求中的未知、缺失或类型非法 `role`
按客户端用户输入审核并参与最新输入优先级，避免协议新增角色或伪造角色绕过文本门禁。
`blocking` 模式下，风险阻断或 Guard 故障发生时不得选择渠道、占用渠道并发、
预扣费或调用上游。

屏蔽词的 request/response、block/mask 命中均写统一事件，事件来源为
`sensitive_word`，阶段区分 `request`、`response`、`response_stream`、
`realtime_request` 和 `realtime_response`；同一请求同一规则与阶段只记录一次，避免
流式分片重复刷屏。屏蔽词命中仍保持既有 HTTP 状态码和响应格式，不因新增审计记录
改变转发语义。命中元数据只保存规则 ID（缺失时保存规则名）和动作；提示词正文仍
遵循统一加密策略，列表不回传命中正文或 Authorization 等敏感字段。

上游 HTTP 错误体、SSE 事件及 Realtime 上游帧在写给客户端前精确检查
`cyber_policy`。命中时沿用上游原始响应，不新增本地二次阻断；异步写入来源为
`upstream_policy`、分类为 `cyber_policy`、分数为 `1.0` 的事后事件。该事件只说明
上游已经拒绝请求，不代表 new-api 在请求前完成了本地语义识别。

可选的自动处置在事件持久化成功后执行，事件写入失败时不得封禁用户。计数只使用
同一普通用户在配置滚动窗口内的精确 `cyber_policy` 事件，不统计 Guard、屏蔽词、
普通上游错误或模糊文案。达到阈值后使用带当前状态和普通用户角色条件的数据库更新
禁用用户，保证重复响应、并发命中或多实例执行不会重复处置；成功禁用后立即失效用户
缓存和该用户全部令牌缓存，并写入不含提示词、上游响应或凭据的管理日志。已经禁用的
用户不重复写处置日志。用户缓存的数据库回填使用 Redis generation fencing，自动禁用
期间已经在途的旧用户快照不能在失效后重新写回缓存。该动作不删除历史事件，也不改变
本次上游原始拒绝响应。

分组范围使用请求分配前的真实候选集，而不是只读取用户默认分组。显式单分组
按稳定分组 ID 判断；显式多分组任一候选命中策略即审核；`auto` 或旧数据无法
可靠解析时采用 fail-safe 审核。事件中的 `pre_allocation:*` 表示记录的是分配前
候选或策略命中的分组，而不是事后声称已经选中的渠道分组。滚动升级期间旧用户
缓存缺少 `group_id` 时同样按 fail-safe 审核；用户分组变化会失效整条缓存，避免
分组名称和稳定 ID 不一致。

Realtime 对 `session.update`、用户 `conversation.item.create`、
`response.create` 等文本字段逐帧审核。客户端帧在通过前不得写入上游，缓冲与
转发必须保持原顺序；二进制音频帧不解析、不送 Guard，并按原帧类型和字节内容
转发。WebSocket 的二进制消息类型本身不代表音频：如果二进制负载是合法 JSON
对象，仍按普通 Realtime 事件提取和审计，防止通过帧类型绕过文本门禁。

完整请求归档启用时，每个 Realtime 客户端帧会在任何解析、屏蔽词改写、Guard
判断或上游写入之前，以客户端提交的原始字节单独加密入队。当 Guard 或屏蔽词
门禁本来就需要首轮缓冲时，缓冲帧由门禁中间件归档，后续帧由 Relay 归档，避免重复写入。文本 JSON、二进制 JSON
和原始二进制音频都使用同一连接的 `request_id`，并可按数据库任务 ID 还原接收
顺序；归档原始音频只用于留存，不代表 Guard 会识别其音频内容。

Realtime 屏蔽词和上游 `cyber_policy` 事件按帧记录。连接内后续帧即使命中相同
规则或相同上游错误码，也会生成独立审计事件；HTTP/SSE 的上游策略事件统一按请求
去重，不区分 `response`、`response_stream` 或任务错误转换阶段，避免同一拒绝经过
多层解析或重复流式片段时重复计数和触发自动处置。

为了保证风险首帧不产生渠道或上游连接副作用，启用审计且命中分组范围的客户端
必须先发送 `session.update`、`conversation.item.create`、`response.create`
或其他首个控制帧；实现不会为了等待上游 `session.created` 而提前选择渠道。
首个 JSON 控制帧之前到达的原始二进制音频会有界缓冲，控制帧通过后再按接收顺序
写入上游。首轮缓冲上限取部署请求体上限与 16 MiB 中较小值，且最多 1024 帧；
超过上限会以 Realtime 错误事件和 1009 关闭码拒绝，避免空帧或音频流耗尽内存。
客户端必须在 WebSocket 升级后 30 秒内送达首个 JSON 控制帧，超时返回错误事件并以
1008 关闭，防止连接在现有渠道与模型限流之前无限占用资源。
首轮通过后的畸形客户端 JSON 帧会返回标准 Realtime 错误事件并以 1007 关闭，
不会写入上游；Guard 阻断或 Guard 故障仍分别使用 4403 或 1013。
未命中审计分组或关闭模式的连接会显式跳过后续逐帧审计，避免渠道分配完成后
重新启用门禁。

稳定错误码如下：

- `prompt_guard_blocked`：HTTP 403，Realtime 关闭码 4403。
- `prompt_guard_unavailable`：HTTP 503，Realtime 关闭码 1013。
- `prompt_guard_invalid_response`：HTTP 503，Realtime 关闭码 1013。

异步模式把加密提示词和任务原子写入主数据库，Worker 直接轮询数据库；当前
实现通过数据库轮询运行，不依赖 Redis 保存任务、明文或正确性状态；Redis 唤醒
和配置失效通知只是未来可选优化。异步提取、入队或配置解密失败只增加丢弃指标，
不得改变主请求结果。

## 数据与任务契约

配置、节点、任务、事件和队列容量使用专用数据表。事件新增 `source`、`stage` 与
`prompt_available`，来源固定为 `prompt_guard`、`sensitive_word` 或
`upstream_policy`。JSON 字段统一以 TEXT 保存，
通过 `common.Marshal`、`common.Unmarshal` 等封装读写。大密文字段在 MySQL
使用 LONGTEXT，在 SQLite 与 PostgreSQL 使用 TEXT，以容纳 65536 个 Unicode
字符加密和 Base64 编码后的数据。任务状态为
`queued`、`processing`、`retry`、`done`、`failed`，领取和完成均校验
`claim_version` 与租约；多分片评估期间定时续租，续租丢失即取消旧 Worker 的
Guard 调用。队列、分页和删除兼容 SQLite、MySQL 5.7.8+、PostgreSQL 9.6+。
事件使用内部 `prompt_cipher_kind` 区分“直接提示词密文”和“失败任务负载密文”；
任务负载还携带固定格式标识。详情读取只按这两个显式标记解包，禁止通过尝试解析
用户正文 JSON 来猜测密文用途。

默认参数：Worker 4 个、队列容量 32768、Guard 超时 3000ms、Unicode 单片
4000 字符、最多重试 3 次、同步全局并发 64、单节点并发 16。持久化文本最多
65536 个 Unicode 字符，超限时记录截断标识、完整字符数和完整 SHA-256。
异步 Worker 仅对 Guard 明确标记为可重试的故障执行有界退避；非法响应及其他
不可重试错误在首次领取后直接终结为 `failed`，并在任务和事件中记录稳定错误码。
内置策略 CAS 配置同时保存 `cyber_policy_auto_ban_enabled`、
`cyber_policy_ban_threshold` 和 `cyber_policy_violation_window_hours`；默认分别为
`false`、`10` 和 `720`。阈值范围为 1 到 1000000，窗口范围为 1 到 87600 小时，
配置更新与其他内置策略字段共享 `expected_version` 冲突检测。自动禁用依赖精确
事件成功落库，因此启用该动作时必须同时开启上游安全策略事件记录；两套页面会
联动这两个开关，服务端也拒绝不一致配置。

同步模式会先创建带加密正文的 `pending` 事件，再调用 Guard；通过且策略未开启
Safe 事件保存时删除该待审事件，阻断或故障事件则保留结果。异步任务失败也按当前
配置的保留天数清理，不使用固定的默认 TTL。删除筛选会拒绝负数 ID 或时间参数，
模型层对空条件删除同样拒绝，防止错误请求退化成全表删除。

Qwen3Guard 结果严格解析唯一的 `Safety:` 和 `Categories:` 行，支持暴力、
非暴力违法、性内容、PII、自杀自残、不道德行为、政治敏感、版权侵权、越狱
攻击九类风险。一次评估使用首个启用节点的超时作为总预算，所有分片和节点故障
切换共享该期限；只有可重试故障才按优先级切换，非法 Guard 响应不切换。未知安全
状态、重复字段和额外正文仍返回 `prompt_guard_invalid_response`；未知分类会被
归一化后以不可逆的 `unknown:<sha256 前缀>` 形式写入 `UnknownCategories`，不保存
原始未知文本。`Unsafe` 且包含未知分类（或没有可识别分类）按 fail-closed 阻断，
避免新版本 Guard 增加风险类别后绕过门禁。

事件详情中的 `UnknownCategories` 仅用于 Root 排查模型版本差异，永不回传原始分类
文本；其哈希输入为规范化后的类别标识。

## 完整请求归档

完整请求归档是一条独立于屏蔽词、上游策略事件和 Prompt Guard 的异步链路。
Relay 完成鉴权并取得可复用正文快照后，在屏蔽词改写、Guard 判断、渠道分配、
计费和协议转换之前把客户端原始 HTTP 正文加密入队。归档是否成功不会改变 Relay
结果；队列已满、正文超限、密钥不可用、目标不可用或数据库写入失败时只增加
`dropped` 运行指标，并记录稳定错误码，不向客户端暴露存储细节。

WebSocket Realtime 完成鉴权后，同样把每个客户端帧作为独立任务异步归档：文本
JSON、二进制 JSON 和原始二进制音频均保留原始字节。首轮与后续帧沿用同一
`request_id`，按数据库任务 ID 表示接收顺序。单帧入队失败只增加 `dropped` 指标，
不会阻断或重排后续帧。仅启用请求归档时，中间件会在渠道分配前读取、归档并缓存
首个原始帧，但不要求它是 JSON 控制帧，也不增加 30 秒首个控制帧期限；首帧之后的
内容由 Realtime Relay 在写上游前逐帧归档。Guard 或屏蔽词门禁启用时，则沿用上一节
一直缓冲到首个 JSON 控制帧的行为。

正文加密和持久队列入队位于请求路径内；加密前容量预检与正式数据库事务使用同一
动态期限：以 250 毫秒为基础，每个开始的 1 MiB 正文增加 50 毫秒，最长 8 秒，以兼顾
小请求延迟和大密文跨数据库写入。
真正的本地或对象存储写入由后台 Worker 完成。入队超时或失败不会拒绝客户端请求，
但会产生有界延迟并增加 `dropped` 指标，因此不能把该链路理解为完全零延迟。

HTTP 归档元数据包含请求 ID、HTTP 方法、去除 query/fragment 的路径、
Content-Type，以及用户、令牌和分组的内部标识或显示名称。Realtime 文本帧使用
`WS_TEXT` 与 `application/json`，二进制帧使用 `WS_BINARY` 与
`application/octet-stream`，并保存相同范围的脱敏连接元数据。Authorization、
Cookie、其他请求头和 URL 查询参数不进入数据库或外部存储。

完整请求和原始音频可能包含个人信息、密钥、业务数据等高敏感内容。启用前必须按
部署地区和业务合规要求确认采集依据、最小化保留期、存储访问控制及删除流程；
Realtime 音频会显著增加数据库暂存、队列和最终存储容量，应据实际吞吐调整双容量
上限并持续监控 `dropped` 指标。

### 数据模型与生命周期

请求归档使用四张跨数据库 GORM 表：

- `request_archive_configs`：单例配置、CAS 版本、活动目标和容量参数。
- `request_archive_targets`：多个存储目标及加密后的访问凭据。
- `request_archive_jobs`：持久任务、加密正文、请求元数据、目标快照、租约、重试、
  对象键和保留期限。
- `request_archive_queue_states`：数据库中仍保留密文的任务数和正文总字节数；与任务
  状态在同一事务更新，保证多进程容量限制正确。最终失败且仍保留密文的任务会继续
  占用两类容量，只有密文成功投递并清空或任务实际删除后才释放。

任务状态为 `queued`、`processing`、`retry`、`done`、`failed`。入队事务会再次校验
配置版本、启用状态、活动目标，以及任务数和字节数上限；加密前还会做无副作用容量
预检，避免队列已经饱和时继续消耗大正文加密资源，最终是否可入队仍以事务内条件更新
为准。正文从首次落库起就是 AES-256-GCM 版本化密文。新任务使用 `ra3` 分片信封，
每片 1 MiB；以随机 `archive_id` 作为 HKDF-SHA-256 盐从稳定 `CRYPTO_SECRET` 派生
任务独立的 256 位 AES 密钥，每片 nonce 由 64 位随机前缀和 32 位分片序号组成。数据库
`sha256` 兼容列在 `ra3` 中保存另一独立 HKDF 密钥计算的 HMAC-SHA-256，不保存可对常见
请求做离线字典猜测的明文 SHA-256；该 keyed digest 与随机 `archive_id`、字节数、目标和
配置版本，以及用户、令牌、分组、请求 ID、
路径和时间等关键任务元数据绑定到 v3 AAD。移动密文或篡改任一绑定字段都会导致校验失败。
旧 `ra2` 保持原主密钥、64 位随机前缀加 32 位分片序号及 v2 AAD 的只读兼容；`ra1`
单块信封也只保留解密兼容，二者随保留期结束自然清理。

Worker 使用 `claim_version + lease` 条件更新领取任务，不依赖 Redis、`SKIP LOCKED`
或数据库专用锁。存储暂时不可用时最多尝试 3 次，并按 5 秒、30 秒、2 分钟退避；
过期的 `queued` 或 `retry` 任务会直接转为 `failed`，不会在存储恢复后补传过期正文。
关闭归档后不再接收新任务，但仍保留一个 drain Worker 处理已入队任务。缺少稳定
`CRYPTO_SECRET` 时 Worker 不领取任务，保持 `queued` 或 `retry` 等待配置恢复，避免把
健康密文误标记为最终失败。Worker 对 `ra2`、`ra3` 逐片认证并累计校验长度和对应摘要，
不重新组装完整明文；请求路径中的磁盘正文
物化与加密、Worker 的密文加载和逐片校验共享 384 MiB 内存预算。磁盘型 BodyStorage
会先取得预算再读取完整正文；预算不足按异步丢弃处理，不阻塞主请求。

对象键为
`<prefix>/requests/YYYY/MM/DD/<job-id>-<ciphertext-sha256>.enc`；没有 `prefix`
时从 `requests/` 开始。哈希只基于密文，不暴露请求明文哈希。外部对象保存的内容仍
是密文信封，不是解密后的请求正文。写入成功
后任务转为 `done`，数据库中的正文密文立即清空，只保留对象位置和技术元数据；
写入最终失败的任务在保留期内保留密文并继续占用任务数和字节容量，便于后续精确
处理，同时防止持续故障绕过队列上限造成数据库无限增长。

后台每分钟启动一次清理，并在 45 秒 Context 截止时间内持续按最多 500 条一批排空过期终态
任务。已上传对象先按任务记录的精确 `object_key` 删除；对象删除状态分为 `exact`、
`unversioned` 和 `absent`：`exact` 必须携带已固化的 `VersionId`，`unversioned` 明确
表示删除时不得携带 `VersionId`，`absent` 表示静默期后协调并确认对象不存在。空状态是
尚未确认的 `unknown`，绝不能退化为不带版本号的删除。

AWS 或标准 S3 上传未返回版本号时固化为精确 `VersionId=null`，避免 bucket 之后开启
版本控制时只创建删除标记、遗留原来的 null 版本。仅官方
`*.r2.cloudflarestorage.com` endpoint 按 R2 明确无版本能力固化为 `unversioned`，不调用
R2 未实现的版本枚举接口。自定义域名代理 R2 无法可靠识别时按普通 S3 处理；版本无法
唯一确认就保留任务并重试，不冒险删除。

Worker 距 `expires_at` 不足 5 分钟时不再领取新任务。终态任务已有 `object_key` 但版本
状态仍为 `unknown` 时，无论对象首次看起来存在还是不存在，都必须先持久化 10 分钟清理
静默期；静默期内不执行 HEAD、版本查询或删除，使客户端超时后仍在服务端处理的多个
晚到 Put 有机会全部落定。静默期结束后才恢复最终版本或确认不存在，并以
`object_key + object_version_mode + object_version_id + reconcile_started_at` 条件快照
删除数据库行。版本固化与 `absent` 确认使用互斥 CAS，多实例不能按旧结论删除已经变化
的任务。单个对象清理失败只写稳定错误码并在下一轮重试，游标继续扫描后续任务，不会
阻塞整批清理。

### 配置多个存储目标

配置最多保存 64 个稳定 ID 的目标，ID 统一为不超过 64 字节的小写 ASCII
字母、数字、连字符或下划线，类型仅支持 `local` 和 `s3`。`active_target_id`
只决定新任务写入哪个目标；任务入队时会保存 `target_id`，因此切换活动目标、禁用
旧目标或关闭归档都不会改写已入队任务的目的地。旧任务仍可由其原目标继续投递和
清理。

保留期内只要目标仍有排队、处理、重试任务，或终态任务仍记录外部对象，就不能
使用同一目标 ID 修改存储类型、本地路径、endpoint、bucket、region、prefix、
寻址方式或访问凭据，也不能删除该目标。迁移存储时应新增目标、探测连通性、保存
配置，再把 `active_target_id` 切换到新目标。

目标配置如下：

- 本地存储使用服务独占的绝对目录。配置保存只校验已有路径层级，不创建目录；探测或
  首次写入时才通过 Go `os.OpenRoot`/`os.Root` 逐级按 `0700` 创建和打开目录，并在受限
  目录句柄内以 `0600` 临时文件原子重命名。卷根目录、Windows UNC/网络共享、任意父级
  或子级中的符号链接、目录联接、重解析点和非目录节点都会被拒绝。Windows 8.3 短路径
  会解析并按同一真实目录逐层验证，名称本身合法包含 `~` 的普通目录也不会被误判。
  同时校验相对路径，创建、打开、重命名和删除都只能发生在已打开的配置根目录内，禁止
  对象键或路径替换竞态逃逸。
  本地目标不接受也不保存访问凭据；运维侧不得把该专用目录交给其他进程改写目录结构。
- AWS S3 或其他 S3 兼容存储填写 `bucket`、`region`，可选 `prefix`、自定义
  `endpoint` 和 `path_style`。endpoint 只接受 HTTP/HTTPS URL，禁止 userinfo、
  query、fragment 和相对路径；留空时使用 AWS 默认终端。HTTPS endpoint 可以是公网
  或私网地址，但仍拒绝云元数据和 link-local 地址；明文 HTTP endpoint 的每一个 DNS
  解析结果都必须属于回环或私网，避免凭据和归档对象通过公网明文传输。拨号前会重新
  解析并校验全部地址，防止混合 DNS 结果或重绑定绕过限制。
- Cloudflare R2 使用 `s3` 类型，把 endpoint 配置为账户的 R2 S3 API 地址，region
  通常填写 `auto`，并填写 R2 Access Key ID 与 Secret Access Key。R2 不使用独立
  存储类型；官方 endpoint 会启用上述明确无版本的删除语义。

对象存储客户端使用独立且复用的连接池，不继承环境代理和全局跳过证书校验设置，
强制 TLS 1.2 及以上、不跟随重定向，响应体最多读取 64 KiB。Worker 写入超时以
20 秒为基础，每开始 1 MiB 密文增加 2 秒，最长 5 分钟；目标探测由管理接口限制为
15 秒，后台精确清理每轮总预算为 45 秒。
Root 可以显式配置本机或内网的 S3 兼容服务。访问密钥分别使用
`access_key_action` 和 `secret_key_action` 执行 `keep|replace|clear`；接口只返回
`access_key_configured`、`secret_key_configured`，永不返回明文或密文凭据；只有
`replace` 可以同时携带非空凭据，`keep` 和 `clear` 携带凭据会被拒绝。两项凭据必须
同时清除，且清除前必须先停用目标，避免留下无法使用的活动 S3/R2 配置。

### 默认值与约束

- 默认关闭，保留期 30 天，Worker 4 个。
- 默认队列容量 32768 个未完成任务，等待队列正文总量 1 GiB。
- 默认单个 HTTP 请求正文或 Realtime 客户端帧上限 64 MiB。
- 保留期允许 1 到 3650 天，Worker 允许 1 到 32 个，队列容量允许 1 到
  1048576 个任务。
- 单请求上限允许 1 KiB 到 128 MiB；队列字节上限不得小于单请求上限，且不得超过
  64 GiB。
- 自定义 endpoint 最多 2048 字节，本地绝对路径最多 4096 字节，每个对象存储凭据
  最多 4096 字节；超限或包含 NUL 的输入在加密和落库前拒绝。
- 新任务使用 1 MiB 分片的 `ra3` AES-256-GCM 信封并经 Base64 写入数据库；分片降低
  加解密峰值并允许 Worker 逐片校验，但数据库字段仍保存完整信封。MySQL 部署使用
  默认 64 MiB 正文上限时，Base64 后的单条密文会超过 85 MiB，必须把
  `max_allowed_packet` 配置为至少约 128 MiB；若提高单任务上限，还需按约 4/3 的
  Base64 膨胀比例及协议余量同步提高该值。

启用归档、替换对象存储凭据和解密已有任务都依赖显式、稳定的 `CRYPTO_SECRET`。
没有稳定密钥时配置读取和运行状态仍可用，但不能启用归档或保存新凭据。密钥丢失或
直接更换会使数据库任务和外部对象中的既有密文无法解密；轮换必须先完成数据
重加密。

### 请求归档管理 API

以下接口位于 `/api/security-audit/request-archive`，继承安全审计路由的
`DisableCache` 和 `RootAuth`：

- `GET /config`：返回脱敏配置。字段包括 `config_version`、`enabled`、
  `active_target_id`、`retention_days`、`worker_count`、`queue_capacity`、
  `max_body_bytes`、`queue_max_bytes` 和 `targets`。
- `PUT /config`：保存完整配置。请求使用 `expected_version` 做 CAS，冲突返回
  HTTP 409；该接口仅要求 Root 权限和限流，不要求 Passkey 或两步验证。
- `GET /runtime`：返回 Worker、心跳、最近处理时间、最近错误、排队延迟、入队与
  丢弃计数。`queue` 包含各状态计数、活动任务数、任务容量、活动正文总字节、
  字节容量和最早待处理时间。
- `POST /targets/probe`：探测一个待保存或已保存目标，仅要求 Root 权限和限流，
  不要求 Passkey 或两步验证。本地目标只创建再删除一个零字节临时文件；S3/R2
  只调用 `HeadBucket`，
  不上传请求正文。结果只返回 `healthy`、`latency_ms`、`status`、稳定错误码和通用
  文案。

`PUT /config` 与 `POST /targets/probe` 中的每个目标使用 `id`、`name`、`type`、
`enabled`、`local_path`、`endpoint`、`bucket`、`region`、`prefix`、`path_style`
及两组凭据动作字段。读取、校验、探测和保存响应均设置 `Cache-Control: no-store`；
管理日志只记录配置版本、目标 ID、目标数量、探测状态和稳定错误码。

## 管理 API

以下接口统一位于 `/api/security-audit`，并使用 `RootAuth`：

- `GET /config`、`PUT /config`
- `GET /builtin-policy`、`PUT /builtin-policy`
- `GET /builtin-policy/channels`：仅返回真实渠道的 ID、名称、状态、类型和渠道标签，
  不返回密钥、地址、用户可访问分组或倍率同步的虚拟价格预设。
- `GET /builtin-policy/channel-tags`：按渠道管理的 `Channel.Tag` 汇总分组，仅返回
  `tag` 和使用该标签的真实渠道数量 `channel_count`。
- `POST /endpoints/probe`
- `GET /runtime`
- `GET /events`
- `GET /events/:id`、`DELETE /events/:id`
- `POST /events/batch-delete`
- `POST /events/delete-preview`
- `POST /events/delete-by-filter`

完整请求归档接口单独列在上一节。它们仍属于同一个安全审计 Root 路由，不注册系统
设置接口或扩展模块入口。

内置策略页只通过专用 Root-only 的 `/api/security-audit/builtin-policy` 读取和保存。
接口底层继续沿用 `CheckSensitiveEnabled`、`CheckSensitiveOnPromptEnabled`、
`SensitiveRules`、`SensitiveRuleChannelIds` 等既有 Option，不复制配置。`PUT` 必须
携带 `expected_version`，并在同一数据库事务中通过 CAS 更新内置审计开关和全部
屏蔽词配置；冲突返回 HTTP 409。旧 `SensitiveWords` 会作为有效规则显示并在首次
保存时迁移为结构化 `SensitiveRules`，原 Option 保留不清空，避免页面保存隐式删除
用户数据。系统设置中的旧入口在两套前端移除，防止同一配置出现两个管理入口。
页面保存、通用 Option 批量更新和周期数据库同步都会先完整解析四个屏蔽词 Option，
再一次发布不可变运行时快照；任一字段无效时保留上一份完整快照，不能让请求读取到
新开关、旧规则或旧渠道范围的混合状态。

`SensitiveRules` 中每条规则的路由范围契约如下：

- `target_type=channels` 时，`channel_ids` 保存一个或多个真实渠道 ID；
  `channel_tags` 必须为空。
- `target_type=channel_tags` 时，`channel_tags` 保存一个或多个规范化后的非空
  `Channel.Tag` 字符串，运行时直接匹配候选或实际渠道的标签；`channel_ids` 必须
  为空。用户分组、令牌分组和路由分组不会成为规则目标。
- 两种方式互斥，但各自都支持多选。启用的显式范围规则必须至少包含一个有效渠道 ID
  或非空渠道标签；禁用规则允许暂时为空，便于管理员分步编辑。
- 历史规则没有 `target_type` 时继续使用全局 `SensitiveRuleChannelIds`，保持升级前
  行为。两套管理页加载历史规则时会把该全局渠道范围复制到每条规则草稿，首次保存后
  写成显式逐规则范围；旧 Option 保留用于旧实例和回滚，不自动删除。
- `group_refs` 仍只引用 `PrefillGroup(type=sensitive_word)` 关键词词库，不表示渠道
  分组。页面统一显示为“关键词组引用”，避免与 `channel_tags` 混淆。
- 删除或停用的渠道、已经不存在的渠道标签不会在配置读取时静默丢弃；页面保留失效
  引用供 Root 清理，运行时不把失效渠道当成候选目标。

配置保存必须携带 `expected_version`，版本冲突返回 HTTP 409。节点令牌更新只允许
`token_action=keep|replace|clear`：`replace` 必须携带非空 `token`，`keep` 和
`clear` 禁止携带 `token`，接口不接受旧的 `clear_token` 字段，也永不返回令牌
明文或密文。保存配置或探测使用 `keep` 时，只有目标地址与已保存节点地址一致才会
复用旧令牌；修改节点地址必须显式 `replace` 或 `clear`，避免把旧令牌发送到新地址。
提示词 Guard 总配置写入、Guard 节点探测、原文详情和删除仍必须通过敏感操作验证
并限流；内置策略写入、请求归档配置写入和请求归档存储探测仅要求 Root 权限和
限流，不启动 Passkey 或两步验证。所有响应使用 `Cache-Control: no-store`。

按筛选删除必须先预览；预览返回匹配数量、快照最大 ID、筛选哈希和五分钟确认
令牌。确认令牌使用显式配置的稳定 `CRYPTO_SECRET` 派生签名密钥，并绑定发起
预览的 Root 管理员 ID；未配置密钥时不得签发或验证。实际删除只处理快照内
数据，避免删除预览后新增事件。

## 页面与兼容性

- Default：`/security-audit`
- Classic：`/console/security-audit`

页面包含概览、审计事件、内置策略、Guard 节点、审计策略、请求归档六个页内标签。
侧栏入口与渠道、用户、日志同级，仅 Root 可见；不注册扩展 manifest，不新增系统
设置页面。原系统设置“屏蔽词”区块迁移到本页且不保留重复入口。请求归档标签统一
管理启用状态、运行指标、多存储目标、活动目标切换和连通性探测。Guard
节点标签负责节点增删改、优先级、令牌状态和连通性探测；按节点生效的超时与
Unicode 分片大小统一在审计策略标签编辑，避免与策略参数分散管理。
内置策略的渠道选择器使用专用真实渠道接口，包含使用默认上游地址的渠道，并排除
倍率同步页专用的“官方倍率预设”和“models.dev 价格预设”。该接口同时返回渠道的
`tag`；页面在渠道选项中显示所属渠道分组，不读取用户分组列表。
每条屏蔽词规则提供“指定渠道 / 整个渠道分组”分段选择和对应多选器；渠道分组选项
来自专用的 `/api/security-audit/builtin-policy/channel-tags` 接口，渠道数据来自
`/api/security-audit/builtin-policy/channels`。页面不再把全局
`SensitiveRuleChannelIds` 作为唯一可编辑范围，也不得调用 `/api/group/details`
把用户可用分组误作渠道分组。

数据库迁移只新增表和索引，默认关闭时不改变现有转发行为。回滚时先切换为
`off` 并停止 Worker，历史表和事件保留，任何物理删除都必须另行备份和确认。

## 验证要求

- 覆盖三种模式、九类解析、Unicode 分片、节点故障切换、加密、配置 CAS、队列
  容量、任务领取、租约回收、重试、保留期和删除确认令牌。
- 覆盖屏蔽词 request/response 的 block/mask 事件、流式去重、无
  `CRYPTO_SECRET` 元数据事件，以及 HTTP/SSE/Realtime 对精确 `cyber_policy`
  的识别和普通错误不误报。
- 覆盖旧全局渠道范围迁移、逐规则多渠道和多渠道分组、固定渠道、动态候选集与模型
  能力求交、未知候选 fail-safe、跨渠道重试后的响应精确匹配，以及关键词组引用、
  渠道标签分组和用户/令牌分组互不混淆。
- 覆盖请求正文加密、本地原子写入、S3/R2 目标校验、配置 CAS、目标切换、任务领取、
  租约续期、计数与字节容量、重试、过期任务、对象精确清理及密文任务绑定；覆盖
  `exact/unversioned/absent` 状态迁移、AWS `null` 版本、R2 无版本删除、首次已存在对象、
  多次晚到 Put、10 分钟静默期和多实例删行 CAS；断言 Authorization、Cookie、query、
  除 Content-Type 外的请求头和底层存储错误不会泄露。
- 验证请求归档失败不改变 HTTP Relay 结果，关闭后仍排空已有任务，无 Redis 时仍可
  投递；验证 Realtime 首轮与后续帧恰好归档一次，文本 JSON、二进制 JSON 和原始
  二进制音频保持原字节、同一 `request_id` 与任务 ID 顺序。
- 阻断、不可用和非法响应必须断言渠道、计费及上游调用次数均为零。
- 验证 SQLite、MySQL、PostgreSQL 查询兼容性以及无 Redis 场景。
- Default、Classic 均验证 Root 入口、直达权限、空态、错误态和移动端布局。
- 本地手工测试固定使用 `scripts/local-test.ps1`、3000/3001 端口和
  `tmp-local-v10101.db`。

模型包提供可选的真实数据库集成测试：分别设置 `TEST_MYSQL_DSN` 和
`TEST_POSTGRES_DSN` 后运行 `go test ./model -run '^TestPromptAuditIntegration'`。
测试只接受尚无安全审计表的空测试库，覆盖迁移、配置 CAS、队列领取与完成、
服务端分页及两种删除路径；完成后会移除本次创建的五张测试表。
