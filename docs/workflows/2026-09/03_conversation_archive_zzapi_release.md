# 对话归档扩展 zzapi 发版记录

## 目标与范围

- 发布版本：`v1.0.0-rc.10.1.10.299`。
- 源码基线：最新 `origin/custom-main`，包含对话归档扩展及其安全审计、Realtime
  归档上下文修复。
- 扩展能力：Root 用户可按多个稳定分组代码、多个用户 ID 和时间范围筛选对话，命中后
  才保存清洗后的紧凑消息；详情接口支持线上预览。
- 安全边界：归档正文删除媒体、base64、工具 schema、请求头和凭据等高体积或敏感
  字段后再加密存储；配置和详情接口保持 Root 权限门禁。数据库和 Redis 不在本次更新
  范围内。
- 部署目标：CloudSSH 项目“API中转站”的 `serverId=52`，Compose 目录
  `/home/docker/zzapi`。
- 应用服务：`zzapi-slave-1`、`zzapi-slave-2`、`zzapi`，按此顺序滚动更新。

## 发布流程

1. 从最新 `origin/custom-main` 创建独立发版分支，更新根目录 `VERSION`，并执行受影响
   的 Go 测试、前端构建和 `git diff --check`。
2. 推送版本提交和同名标签，等待 GitHub Actions 的 Linux Release 与 GHCR 多架构镜像
   构建完成；核对 Release 资产、镜像 manifest 和 amd64/arm64 架构摘要。
3. 更新前读取 `/home/docker/zzapi/docker-compose.yml` 并计算 SHA-256，创建带版本号的
   远端备份。只替换三个应用服务的镜像标签，不重建或重启 PostgreSQL、Redis。
4. 按 `zzapi-slave-1`、`zzapi-slave-2`、`zzapi` 逐个拉取并重建。每个节点都必须确认
   `running/healthy`、`RestartCount=0`，以及对应本地 `/api/status` 返回 `.299`，再继续
   下一个节点。
5. 全部节点完成后复核 Compose、数据库和 Redis 状态，并通过公网状态接口确认版本。

## 验证记录

以下字段在发布和部署完成后补录真实提交、工作流、镜像摘要、CloudSSH 作业、备份摘要、
容器健康与公网状态证据。

- 发布提交：待发布。
- 标签：`v1.0.0-rc.10.1.10.299`。
- GitHub Actions：待完成。
- GHCR manifest：待完成。
- CloudSSH Compose 备份与 SHA-256：待完成。
- 三个应用服务滚动更新与最终核验：待完成。

## 回滚

若任一应用节点更新失败，立即停止后续节点，使用发布前 Compose 备份恢复原应用镜像，
仅重建已修改的应用服务。不得重建或重启 PostgreSQL、Redis。回滚后重新核对三个应用
端口、健康状态、重启次数和版本。
