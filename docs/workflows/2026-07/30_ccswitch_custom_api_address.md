# CC Switch 自定义 API 地址

## 变更目标

令牌页和聊天设置中的 CC Switch 一键导入不再只能使用网站主域名。管理员可在聊天设置中
单独配置 API 根地址，并让 Default、Classic 两套前端的所有 CC Switch 入口使用同一配置。

## 配置与公开状态

- 后端选项名为 `CCSwitchAPIAddress`，管理端在聊天设置中编辑。
- `/api/status` 以 `cc_switch_api_address` 返回该值；空值保持为空，由前端执行兼容回退。
- 运行时配置通过原子快照读写，管理员更新选项与并发读取 `/api/status` 不会产生数据竞态。
- 配置值必须是包含 `http` 或 `https` 协议的绝对地址，不能携带账号、查询参数或锚点；
  保存时去除首尾空白和末尾斜杠。
- 留空时，导入端点依次回退到 `/api/status` 的 `server_address` 和当前页面来源地址。

## 一键导入语义

- Claude 与 Gemini 的 `endpoint` 使用 API 根地址。
- Codex 的 `endpoint` 使用 API 根地址下的 `/v1`；若管理员已填写以 `/v1` 结尾的地址，
  不重复追加。
- `homepage` 始终使用网站 `server_address`，避免把仅用于 API 的子域名显示成官网。
- 令牌列表独立 CCS 按钮与聊天配置中的 `ccswitch` 特殊入口必须打开同一个导入弹窗，
  不把 `ccswitch` 当成普通外部链接。
- `ccswitch` 特殊入口忽略首尾空白和大小写，Default 与 Classic 保持一致。
- 管理员保存地址成功后，Default 立即覆盖 React Query 与浏览器本地状态，
  Classic 立即覆盖浏览器本地状态。后续公开状态刷新失败不得恢复旧地址，
  避免把真实 API Key 导入已废弃或不再可信的端点。

## 安全与兼容性

该地址会写入浏览器可见的 CC Switch 导入链接，只能用于公开的 API 根地址，不能包含密钥、
用户名或密码。旧部署未配置新选项时继续使用原有网站服务器地址，不影响已有聊天链接或令牌。

## 测试与结果

1. 验证选项默认值、更新和 `/api/status` 输出，并通过并发读写测试覆盖运行时配置更新。
2. 验证自定义地址优先级、空值回退、尾斜杠和 Codex `/v1` 幂等处理。
3. 验证 Default 与 Classic 的令牌页按钮均使用自定义 API 地址。
4. 验证两套前端的聊天 `ccswitch` 入口均打开同一导入弹窗。
5. 验证保存新地址或清空地址时立即覆盖旧缓存，不依赖二次状态请求成功。
6. 执行 Go 测试、双前端测试与构建、国际化同步和 `git diff --check`。

上述验证均已通过。Go 全量包测试按 60 秒上限拆分执行；Default 类型检查和生产构建、
Classic 生产构建、两套前端 CCS 专项测试及国际化同步均通过。固定本地测试环境使用
`localhost:3000`、`localhost:3001` 和 `tmp-local-v10101.db`，已验证 `/api/status`、
Classic 首页、默认账号和演示数据；状态接口会返回 `cc_switch_api_address`，默认空值按契约
回退网站服务器地址。

运行时配置并发专项测试已通过：

```powershell
go test -count=1 -timeout 60s ./setting -run TestCCSwitchAPIAddressConcurrentAccess
```

该用例可在支持竞态检测的 Go 环境执行 `go test -race`。本机 Go 目标为 `windows/386` 且
`CGO_ENABLED=0`，工具链不支持 `-race`，因此本次以并发压力用例和包级测试完成本地验证。
