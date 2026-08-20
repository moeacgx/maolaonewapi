# zzapi 自更新检查路由 404 修复

## 问题

zzapi 后台的“检查更新”按钮调用 `GET /api/status/github-latest-release`，一键更新调用 `POST /api/status/self-update`。当前 `custom-main` 后端没有注册这两个接口，也缺少对应自更新服务实现，前端因此收到 404。

## 范围

- 后端恢复 GitHub Release 查询和 Docker 二进制自更新服务。
- `/api/status/github-latest-release` 与 `/api/status/self-update` 均保持 Root 鉴权。
- 不改变前端请求路径、数据库结构或普通 `/api/status` 契约。

## 安全边界

- 自更新仓库仍可由 `SELF_UPDATE_REPO` / `SELF_UPDATE_GITHUB_REPO` 覆盖；未配置时默认查询 `moeacgx/maolaonewapi`，避免检查更新和详情跳转落到上游 `moeacgx/new-api`。
- 私有仓库认证只读取 `SELF_UPDATE_GITHUB_TOKEN`，兼容 `GITHUB_TOKEN`，不在源码或配置中写入 Token。
- 下载 URL 仍限制为 GitHub 官方 Release/API/对象存储域名，并校验属于配置仓库。
- 替换二进制前校验 SHA-256；非 Linux 或非 Docker 环境默认禁用一键更新。
- Classic 详情按钮优先打开后端返回的 `html_url`，无该字段时才回退拼接 `moeacgx/maolaonewapi` Release 标签页。

## 验证
- `go test ./service ./router ./controller -run 'Test(SelfUpdate|ValidateGitHubDownloadURL|SetApiRouterRegistersSelfUpdateStatusRoutes)' -count=1 -timeout 90s`：通过。
- `node src/self-update-release-link.test.mjs`（`web/classic`）：通过，覆盖 Classic 详情跳转优先使用后端 `html_url` 并回退 `moeacgx/maolaonewapi`。
- `npx --no-install eslint src/components/settings/OtherSetting.jsx src/self-update-release-link.test.mjs`（`web/classic`）：通过。
- 路由测试断言两个前端调用路径已注册，并且未登录请求不再返回 404，而是进入 Root 鉴权链路。
