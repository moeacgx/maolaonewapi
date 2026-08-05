# maolaonewapi 私有仓库自更新适配

## 目标

后台自更新接口需要读取当前项目 `maolaonewapi` 的 GitHub Release，而不是依赖
默认上游仓库。当前远程仓库为私有仓库，Release 查询和资产下载都必须携带
GitHub Token。

## 配置契约

容器运行环境使用以下变量覆盖仓库和认证信息：

```text
SELF_UPDATE_REPO=moeacgx/maolaonewapi
SELF_UPDATE_GITHUB_TOKEN=<具有私有仓库读取权限的令牌>
```

`SELF_UPDATE_REPO` 优先级高于 `SELF_UPDATE_GITHUB_REPO`，两者都未设置时保留
原有默认仓库行为，避免影响通用发行版。Token 只从运行环境读取，支持
`SELF_UPDATE_GITHUB_TOKEN`，并兼容已有的 `GITHUB_TOKEN` 回退变量；源码、镜像
标签和 Git 提交中不得出现 Token 明文。

## 接口与安全边界

- `GET /api/status/github-latest-release` 查询指定仓库的最新 Release。
- `POST /api/status/self-update` 下载当前平台资产，校验 SHA-256 后替换二进制。
- Release API 和资产下载均复用同一认证头；私有仓库无 Token 时应返回 GitHub API
  错误，不得退回匿名访问或切换到其他仓库。
- 下载 URL 必须属于配置的仓库，仍限制为 GitHub 官方 API、Release 下载或对象
  存储域名，避免私有仓库配置扩大下载范围。
- 检查更新使用 GitHub `/releases/latest`，因此可供一键更新的版本必须发布为正式
  Release，不能勾选 Prerelease。仅创建标签或预发布不会进入该接口。
- Release 必须同时提供当前 Linux 架构的 `new-api-<tag>` 或
  `new-api-arm64-<tag>`，以及 `checksums-linux.txt`。缺少资产时页面只能展示版本，
  一键更新会按安全设计保持禁用，不能下载源码包冒充可执行文件。
- 发布二进制通过 Go `ldflags` 内嵌的版本号优先于工作目录中的 `VERSION` 文件；自更新
  只替换可执行文件，即使旧镜像残留旧 `VERSION`，重启后也必须报告新版本。显式设置的
  `VERSION` 环境变量仍具有最高优先级，供部署方主动覆盖。
- 生产 Compose 只注入环境变量，不把 Token 写入仓库文件；更新容器时不能重启
  数据库或 Redis。

## 验证计划

- 使用 `go test ./service -run 'TestSelfUpdate|TestValidateGitHubDownloadURL'`。
- 检查私有仓库的 Release 资产包含 Linux amd64/arm64 二进制及
  `checksums-linux.txt`。
- 部署后以 Root 身份访问 Release 查询接口，确认返回仓库对应的 Tag；不主动执行
  计费或生产资产下载。

## 回滚

移除生产环境中的 `SELF_UPDATE_REPO` 与 Token 变量即可恢复默认仓库行为；不需要
数据库迁移或回滚。
