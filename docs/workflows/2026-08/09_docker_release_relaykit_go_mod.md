# Docker Release relaykit 本地模块复制修复

日期：2026-08-09

## 问题

`v1.0.0-rc.10.1.10.230` 推送后，二进制 Release 工作流成功，但 `Publish Docker image (Multi-arch)` 在 Docker 构建阶段失败：

```text
go: github.com/QuantumNous/new-api/relaykit@v0.0.0 (replaced by ./relaykit): reading relaykit/go.mod: open /build/relaykit/go.mod: no such file or directory
```

根因是根 `go.mod` 使用：

```text
replace github.com/QuantumNous/new-api/relaykit => ./relaykit
```

而 Dockerfile 在 `RUN go mod download` 前只复制了根 `go.mod` 和 `go.sum`，没有复制 `relaykit/go.mod` 和 `relaykit/go.sum`。本地普通 `go test` 成功，是因为完整工作区本来就包含 `relaykit/`；Docker 分层构建的依赖下载层没有这个本地模块目录。

## 变更

- `Dockerfile` 在 `go mod download` 前复制 `relaykit/go.mod` 和 `relaykit/go.sum` 到 `/build/relaykit/`。
- `Dockerfile.dev` 同步修复，避免后端开发镜像走同一依赖下载层时失败。
- 只复制模块清单文件，不提前复制整个 `relaykit/` 源码，保持 Docker 依赖缓存粒度。

## 兼容性

不新增配置、接口、数据表或迁移。该修复只影响 Docker 构建上下文；二进制 Release 工作流不受影响，因为它在完整 checkout 目录内执行 `go mod download`。

## 验证计划

- 执行 `docker build --target builder2 --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 .`，确认 `go mod download` 能读取本地 replace 模块。
- 执行 `go test ./controller ./service -count=1 -timeout 120s`，确认 relay 修复未回归。
- 执行 `git diff --check`。
- 推送修正 Tag 后观察 Release 与 Docker 两个 Actions 工作流。
