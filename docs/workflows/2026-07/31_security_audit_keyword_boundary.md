# 安全审计关键词智能边界与处理结果筛选

## 问题与根因

屏蔽词此前对所有关键词统一执行忽略大小写的连续子串匹配。英文短语
`Master Key` 因而会跨入 `Webmaster Keyword` 的两个单词内部，产生与管理员配置
意图不一致的阻断或脱敏事件。

同一页面此前虽然保存了事件的 `action`，列表却只展示风险判定，Root 无法直观看出
记录究竟是被拦截还是已脱敏过滤，也不能按实际处置结果筛选。

## 修改范围

- block 与 mask 使用同一套智能边界匹配。
- 关键词首尾为 ASCII 字母、数字或下划线时，候选前后不能紧邻同类 ASCII 字符。
- 中文等非 ASCII 关键词保留原有连续子串语义，不要求依赖空格分词。
- Default 与 Classic 的详情高亮使用同一边界，不再高亮后端不会命中的复合词片段。
- SSE 仅缓冲可能继续组成关键词的尾部分片；普通安全分片保持即时发送。
- ASCII 关键词位于分片末尾时等待下一字符或流结束确认，跨分片 block 与 mask 均使用
  完整左右边界；mask 命中可跨多个 JSON 分片改写，并保留原 `event:` 类型与顺序。
- 匹配继续忽略大小写，规则 JSON、动作、作用阶段和路由范围契约不变。
- 事件列表新增“处理结果”列，明确区分“已拦截”和“已过滤（脱敏）”。
- 新增服务端 `action` 筛选，Default 与 Classic 共用 `block`、`mask`、`warn`、
  `allow`、`pending`、`error` 契约。
- 列表、删除预览和按条件删除复用同一 `action` 条件；事件表原有 `action` 列继续使用，
  不新增数据库字段。

## 兼容性与边界

本次不修改数据库结构；管理 API 的事件筛选新增可选 `action` 条件。英文关键词位于
更长英文标识符内部时不再命中；
如果确实需要拦截复合词，管理员应把完整复合词作为独立关键词配置。连字符、空格和
常见标点仍可作为英文词边界，中文与英文直接相邻时不把中文字符视为 ASCII 词内字符。
流式输出中，只有可能成为关键词前缀或等待 ASCII 右边界确认的尾部才会短暂延迟；
流结束会先完成待确认的阻断或脱敏，再决定是否发送 `[DONE]`。命中 block 时，仍在缓冲
且参与关键词拼接的分片不会先行暴露给客户端。

## 验证结果

- `Bing Webmaster Keyword Research` 不命中 `Master Key`。
- 独立的 `Use the Master Key now` 正常命中。
- mask 只替换独立短语，不改写 `Webmaster Keyword` 或
  `Master Keywordization`。
- `Web` + `Master Key` 与 `Master Key` + `word` 均不会在 SSE 分片边界误阻断。
- `Master Ke` + `y` 在流结束时可正确阻断；配置 mask 时会输出 `[MASK]`，不会原样
  泄漏跨分片关键词。
- 缓冲后的 Responses 风格 SSE 仍保留各分片原有 `event:` 标签。
- `包含敏感词内容` 继续命中中文关键词 `敏感词`。
- `action=block` 只返回实际处置为 Block 的事件，`action=mask` 可独立筛出脱敏记录。
- 两套前端列表、详情和筛选器均直接使用事件 `action`，不从风险判定反推。
- 执行 `go test ./service -run '^TestSensitiveKeywordSmartBoundary' -count=1`
  通过。
- 执行 `go test ./relay/helper -run '^Test(StringData|FilteredEventData)' -count=1`
  通过。
