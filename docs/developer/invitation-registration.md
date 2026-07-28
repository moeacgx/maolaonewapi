# 邀请制注册

## 目标与范围

邀请制注册用于在关闭公开注册后，仍允许持有有效邀请链接的新用户完成注册。它是内置注册策略，不新增邀请码表，也不改变现有返利邀请关系：邀请码继续使用邀请人的 `users.aff_code`，注册成功后把邀请人 ID 写入新用户的 `inviter_id`。关闭公开注册时，低熵的返利码不能单独作为准入凭证，必须同时提交服务端生成的 HMAC-SHA-256 签名。

该能力同时覆盖密码注册和 OAuth 新用户创建。已有 OAuth 绑定用户的登录不受注册开关影响。

## 配置与默认值

- `RegisterEnabled`：公开注册总开关，沿用现有配置，默认 `true`。
- `InvitationRegisterEnabled`：公开注册关闭时是否允许有效邀请码放行，默认 `false`。
- `PasswordRegisterEnabled`：密码注册方式开关，沿用现有配置，默认 `true`。

`GET /api/status` 返回：

- `register_enabled`
- `invitation_register_enabled`
- `invitation_register_signing_ready`
- `password_register_enabled`

`InvitationRegisterEnabled` 只扩展关闭公开注册时的准入条件，不会开启已关闭的密码注册方式，也不会替代各 OAuth 提供方自身的启用开关。

登录页仍只在公开注册开启时展示通用“注册”入口，避免把邀请制误展示为公开注册。`GET /api/affiliate/summary` 返回的邀请链接格式为 `/register?aff=<邀请码>&invite=<签名>`；Default 会保留查询参数并跳转到 `/sign-up`，Classic 直接打开 `/register`。两套页面读取邀请凭证后立即通过替换当前历史记录移除地址栏中的 `aff` 和 `invite`，保留其他查询参数，并且只在当前标签页的 `sessionStorage` 中暂存本次凭证；成功注册、成功登录或启动 OAuth 后立即清理浏览器中的临时值。

签名使用稳定的 `CRYPTO_SECRET` 派生；未显式配置 `CRYPTO_SECRET` 时允许回退到显式配置的 `SESSION_SECRET`。两者都未配置时 `invitation_register_signing_ready=false`，不能生成可跨重启使用的邀请注册链接，也不能在关闭公开注册时通过邀请准入。多实例部署时所有实例必须使用相同密钥。

## 准入契约

新用户创建统一按以下规则判断：

1. `RegisterEnabled=true` 时允许公开注册。请求携带有效邀请码时建立邀请关系；未携带或邀请码无效时仍按普通公开注册处理。为兼容已经发出的旧链接，此时仅有 `aff_code`、没有 `invite` 签名仍可建立邀请关系。
2. `RegisterEnabled=false` 且 `InvitationRegisterEnabled=false` 时禁止所有新用户注册。
3. `RegisterEnabled=false` 且 `InvitationRegisterEnabled=true` 时，仅允许同时携带非空 `aff_code` 和有效 `invite` 签名的新用户注册。
4. 签名固定绑定规范化后的邀请码和版本域 `invitation-registration:v1:`，服务端使用常量时间比较，不能从一个邀请码挪用到另一个邀请码。
5. 邀请码必须由 `model.GetActiveInviterIdByAffCode` 验证成功。不存在、邀请人账号已禁用、已失效或因返利风控设置 `block_invite_code` 的邀请码均不得绕过关闭的公开注册；风控状态查询失败时按无效处理。
6. 用户创建事务内会再次校验邀请码、签名、邀请人状态和返利风控状态；复核时锁定邀请人记录，封禁邀请码、禁用邀请人与注册创建按提交顺序串行化，复核结果与事务前不一致时终止创建。
7. 密码注册始终额外受 `PasswordRegisterEnabled` 约束；该开关关闭时，即使邀请凭证有效也不能使用密码注册。
8. OAuth 只在确认第三方账号尚未对应本地用户、准备创建新用户时执行注册准入检查。已存在用户继续登录，不受 `RegisterEnabled` 或 `InvitationRegisterEnabled` 影响。

## 密码注册

`POST /api/user/register` 从请求体的 `aff_code` 和 `invite` 读取邀请凭证。公开注册关闭时，服务端在创建用户前验证签名和邀请码并取得邀请人 ID；验证失败统一按注册关闭处理，不创建用户、不发放默认令牌，也不增加邀请计数。

邮箱验证、用户名校验、默认令牌创建和新用户额度等既有流程保持不变。

## OAuth 注册

`GET /api/oauth/state?aff=<邀请码>&invite=<签名>` 把本次 OAuth 流程的完整邀请凭证写入服务端会话。请求缺少任一字段或值为空时必须删除会话中对应的旧值，防止此前残留凭证被后续无邀请流程复用。

通用 OAuth、GitHub、Linux DO、Discord、OIDC 与微信的新用户创建路径都从会话读取邀请码，并使用同一注册准入规则。准入成功后，创建用户时传入已验证的邀请人 ID。OAuth 账号绑定现有登录用户不属于新用户注册，不使用此策略。

## 安全边界

- 不能只判断邀请码字符串非空；关闭公开注册时必须先验证签名，再查询数据库并通过返利风控状态校验。
- 关闭公开注册时，邀请准入必须早于邮箱验证码和账号存在性查询，所有无效邀请统一返回注册关闭，不能用差异响应枚举账号。
- 不信任客户端提交的邀请人 ID，服务端只接受邀请码并解析为用户 ID。
- 公开注册关闭时，对签名缺失或篡改、邀请码无效、邀请人已禁用、风控封禁和风控状态不可确认使用相同的注册关闭响应，避免通过响应枚举邀请码状态。
- 邀请码只在创建新用户时消费；OAuth 回调必须先查找现有绑定用户，再执行准入判断。
- 签名是可重复使用的邀请链接凭证，不是一次性口令；撤销依赖禁用邀请人、返利风控的 `block_invite_code` 或轮换全局密钥。
- 浏览器不得把邀请签名长期写入 `localStorage`；页面进入无邀请参数的登录或注册流程时必须清理当前标签页内的旧凭证。
- 页面接收邀请参数后必须立即从地址栏和当前历史记录移除 `aff`、`invite`，避免可重复使用的签名残留在浏览器历史中。
- 配置默认关闭，升级后不会改变当前关闭注册站点的行为。

## 兼容性与回滚

该变更只新增 Option，不新增数据表或数据库专用字段，兼容 SQLite、MySQL 与 PostgreSQL。回滚时将 `InvitationRegisterEnabled` 设为 `false` 即可立即恢复“公开注册关闭后禁止所有新用户”的行为，既有邀请关系不受影响。

## 测试计划

- 覆盖公开注册开关、邀请注册开关的全部组合。
- 覆盖有效、缺失、篡改和跨邀请码复用签名，以及不存在和风控封禁邀请码。
- 覆盖密码注册方式关闭时有效邀请码仍不得放行。
- 覆盖 OAuth 已存在用户登录不受注册开关影响，以及 OAuth 新用户必须使用会话中的有效邀请码。
- 覆盖创建事务内邀请人被禁用或风控撤销时回滚。
- 覆盖无效邀请在邮箱验证码和账号存在性检查前被统一拒绝，以及注册与邀请码封禁使用同一邀请人行锁。
- 覆盖两套前端的 `sessionStorage` 保存、无参数清理、密码注册透传和 OAuth state 透传。
- 检查 `GET /api/status` 和 Option 初始化、更新后的值一致，并验证稳定密钥缺失时签名状态为不可用。
