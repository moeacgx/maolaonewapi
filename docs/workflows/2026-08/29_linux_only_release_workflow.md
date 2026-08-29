# Release 工作流仅构建 Linux 二进制

## 目标

减少不需要的桌面平台二进制构建。Git 标签发布仍构建前端、Linux amd64 和 Linux arm64
二进制，并生成 Linux 校验文件；不再构建 macOS 或 Windows 二进制。

## 修改范围

- `.github/workflows/release.yml` 的工作流名称改为 `Release (Linux)`。
- 保留 Linux job 的前端构建、amd64/arm64 Go 构建、`checksums-linux.txt` 和 GitHub Release
  上传。
- 删除 macOS 与 Windows job 及其 runner、构建步骤、校验文件和 Release 上传配置。
- `.github/workflows/docker-build.yml` 不变，Docker 镜像仍按既有流程发布 amd64/arm64 多架构
  manifest。

## 兼容性与边界

该变更只影响 GitHub Release 附件和 Actions runner，不改变 API、数据库、Docker 镜像或线上
容器运行方式。Linux Docker 部署继续使用 GHCR 多架构镜像；需要 macOS/Windows 原生二进制
时，必须另行设计和授权对应构建流程。

## 验证计划

- 使用 YAML 解析器检查 `.github/workflows/release.yml` 语法。
- 静态确认工作流只包含 `linux` job，不包含 macOS/Windows runner、输出文件或校验文件。
- 执行 `git diff --check`。
