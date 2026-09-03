# Classic 福利活动模块容器边界

## 目标

修复 Classic “活动福利”和“营销福利”页面缺少明确业务容器的问题，并把后台页面统一的
模块边界要求固化到项目开发规则，避免后续新增页面依赖逐项提醒。

## 范围

- 模板：仅 Classic（`web/classic`）。Default 未修改，仍由共享项目规则约束后续实现。
- 页面：`/console/benefits` 用户福利页、`/console/redemption` 营销福利页，以及营销福利中的
  时效额度券管理面板。
- 后端接口、数据模型、金额/额度计算和业务状态机不变。

## 实现

- 在 Classic `index.css` 增加统一的 `classic-console-panel`、标题区和内容区样式，使用主题边框、
  背景和阴影变量，兼容窄屏内边距。
- 用户福利页将“我的福利券”和“可领取活动”分别放入有边界的业务面板，券卡片保留为重复项边界。
- 营销福利页将标题和标签内容放入统一业务面板；时效额度券管理卡片显式声明边框和背景，报表
  内部的金额、状态和流水分组边界保持不变。
- 根目录 `AGENTS.md` 与 `web/AGENTS.md` 新增后台容器规范：页面壳层可平铺，但独立业务模块
  必须有可见边框、背景、圆角和稳定层级，避免裸表格/裸表单及无意义的卡片嵌套。

## 验证

- Classic 福利契约测试：覆盖两个福利页面的模块容器边界及既有 API/报表/操作契约。
- Classic Prettier、ESLint、Vite 构建。
- `git diff --check`。
- 本地浏览器访问因开发服务未提供登录态而重定向到 `/login`，未进行登录后的像素级验收；需在有管理员/用户会话的环境复核浅色、暗色和窄屏显示。

## zzapi 部署

- 目标：CloudSSH 项目“API中转站”的 `serverId=52`，Compose 目录 `/home/docker/zzapi`；未操作受保护的 `maolaoapi`。
- 当前工作区归档已上传并校验，归档 SHA-256 为
  `e54498c899325f841f304aaa679f29e6fd40d209faf1e85d1800ec2f25535a95`。
- 远端构建镜像：`ghcr.io/moeacgx/maolaonewapi:zzapi-benefit-ui-20260901`，镜像 ID 为
  `sha256:7f3b983d95876390fe2c11b7195a668a99a4ad8d29515be958b8b5fe68e7f393`。
- Compose 备份为 `docker-compose.yml.bak-benefit-ui-20260901`。按
  `zzapi-slave-1`、`zzapi-slave-2`、`zzapi` 顺序逐个重建应用容器，PostgreSQL 与 Redis 未重建。
- 三个应用容器最终均为 `running/healthy`，重启计数为 `0`；三个本地端口
  `18097`、`18098`、`18099` 及公网 `https://zzapi.maolaoapi.com/api/status` 均返回
  `success=true`。

## 回滚

仅回滚 Classic 样式与页面容器类及对应文档，不触碰福利活动后端表结构和接口。
