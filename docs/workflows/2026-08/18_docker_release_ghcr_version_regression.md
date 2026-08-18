# Docker 发布目标与版本回退修复

## 问题

`v1.0.0-rc.10.1.10.244` 发布时，Docker 工作流被合并回上游 Docker Hub
配置。工作流依赖本仓库未配置的 Docker Hub 凭据，因此两个架构都在登录阶段
失败，没有生成正式容器镜像。

同一标签中的 `VERSION` 为空。手工构建如果未在构建上下文写入标签版本，最终
二进制和前端会显示错误或空版本，无法可靠判断线上运行代码。

## 修改

- Docker 镜像只发布到 `ghcr.io/${{ github.repository }}`。
- 使用工作流内置的 `GITHUB_TOKEN` 和 `packages: write` 权限登录 GHCR。
- 架构镜像、`latest` 和多架构 manifest 使用同一个 GHCR 仓库地址。
- 构建前根据推送标签或手工输入标签重写 `VERSION`，镜像内版本以标签为准。
- 当前分支的 `VERSION` 修正为 `v1.0.0-rc.10.1.10.244`。

## 安全与兼容性

工作流不再依赖外部 Docker Hub 凭据，也不会向其他镜像仓库推送内容。发布标签、
镜像版本和程序报告版本保持一致。此次修改不改变 API、数据库或运行时业务逻辑。

## 验证

- 检查工作流中不存在 Docker Hub 登录和上游镜像推送目标。
- 检查两个 Job 均具备所需 GHCR 权限和登录配置。
- 检查标签解析步骤在 Docker 构建前写入 `VERSION`。
- 使用 YAML 解析器校验工作流语法，并执行 `git diff --check`。

线上 `zzapi` 已使用标签 `v1.0.0-rc.10.1.10.244` 对应提交重新构建；容器内
`/new-api --version`、本机 `/api/status` 和公网 `/api/status` 均返回该版本。
