# 渠道列表分组显示名称修复

## 问题

Classic 渠道列表的分组列优先使用 `record.group_details[].name` 渲染显示名称，但 `GetAllChannels` 控制器在筛选、排序、tag mode 分支中直接查询 `channels` 表，没有像 `model.GetAllChannels` / `model.SearchChannels` 一样调用 `HydrateChannelGroupBindings`。因此响应只有旧 CSV 镜像 `group`，前端只能回退显示稳定 code。

## 修复

- 渠道列表响应返回前统一水合 `group_ids` 和 `group_details`。
- tag mode 的列表和搜索分支同样补齐分组详情。
- `group` 继续保留稳定 code，用于筛选、兼容旧客户端和颜色稳定；展示使用 `group_details[].name`。

## 验证

- `go test ./controller -run 'TestGetAllChannelsIncludesGroupDisplayDetails|TestGetAllChannelsTagModeIncludesGroupDisplayDetails' -count=1 -timeout 120s`
- `go test ./controller -count=1 -timeout 180s`
- `node --test channel-group-copy-display-name.test.mjs`（`web/classic/src`）
