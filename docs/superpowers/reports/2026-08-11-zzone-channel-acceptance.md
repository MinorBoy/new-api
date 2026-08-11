# ZZone 渠道接入验收报告

## 验收结论

ZZone 已按独立 task-only 渠道类型接入，渠道类型为 `212`，复用 `newapivideo` 任务框架。Mock 契约测试、后端聚焦测试、全仓 Go 测试、前端契约测试、i18n 同步和生产构建均通过。

本次未提供真实 ZZone API Key，因此没有执行真实上游 Canary。当前渠道在后端和前端均默认禁用；Mock 通过只证明本地协议实现与已提供文档一致，不能替代真实上游验收。

## 已验收范围

- Ark 提交入口：`POST /api/v3/contents/generations/tasks`。
- 上游提交：`POST /v1/videos`，使用 Bearer 鉴权。
- 上游轮询：`GET /v1/videos/{task_id}`。
- 内容代理：`GET /v1/videos/{task_id}/content`。
- `duration` 转换为字符串字段 `seconds`，`ratio` 转换为 `aspect_ratio`。
- 图片、视频、音频分别转换为 `images`、`videos`、`audios`，上限分别为 `4`、`3`、`1`。
- 只接受公网媒体 URL，拒绝未记录字段和超限输入，失败请求不创建任务且不调用上游。
- `processing -> completed` 无结果 URL 时，使用 new-api 本地公开内容代理 URL。
- 公开任务列表、详情和内容响应不泄漏上游任务 ID、上游模型名或渠道 Key。
- 提交错误 `400`、`401`、`429`、`500` 不在单次请求内重试。
- 失败任务只退款一次；未知上游状态不完成任务、不结算或退款。
- 配置导入支持 `CH-ZZONE -> 212`，保留模板转换规则 `"14": "CH-ZZONE"`。
- ZZone 支持请求时长、上游实际值和上游 usage 三种任务成本计量来源。
- 前端注册 task-only 类型、受管 Base URL、空模型目录、禁用通用渠道测试和预验收默认禁用状态。

## Mock 契约测试

执行命令：

```powershell
go test ./e2e -run 'TestZZone' -count=1
```

结果：退出码 `0`。覆盖 Ark 多模态提交、精确上游 JSON、Bearer 鉴权、无 URL 完成态、公开列表和详情、内容下载、私有字段隔离、失败退款、上游错误不重试、无副作用校验和未知状态保护。

执行命令：

```powershell
go test ./e2e ./relay ./service -run 'TestZZone|TestCostAccounting|Test.*Billing|Test.*Polling' -count=1
```

结果：退出码 `0`。`e2e`、`relay`、`service` 三个包均返回 `ok`。

执行命令：

```powershell
go test ./constant ./relay/channel/task/newapivideo ./controller ./service -run 'TestZZone|TestSupportsGenericChannelTestRejectsDimensio|TestValidateVideoRouteTargetContract' -count=1
```

结果：退出码 `0`。渠道身份、请求转换、内容代理和配置路由契约均返回 `ok`。

执行命令：

```powershell
go test ./e2e ./relay ./router ./service -run 'TestZZone|Test.*Seedance|Test.*Billing|Test.*Polling|TestVideo' -count=1
```

结果：退出码 `0`。`e2e`、`relay`、`router`、`service` 四个包均返回 `ok`。

## 前端和全仓验证

以下前端测试分别执行，以规避 Bun 同一进程混用 `node:test` 测试文件时的嵌套执行限制：

```powershell
bun test src/features/channels/lib/__tests__/new-api-channel.test.ts
bun test src/channel-config-converter/__tests__/v1.test.ts
bun run i18n:sync
bun run typecheck
bun run build
```

结果：每条命令退出码均为 `0`。渠道测试 `9` 项通过，配置转换测试 `21` 项通过；七个 locale 的 `missingCount` 和 `extrasCount` 均为 `0`；TypeScript 检查和 Rsbuild 生产构建通过。

执行命令：

```powershell
go test ./...
git diff --check
```

结果：两条命令退出码均为 `0`。全仓 Go 测试通过，`git diff --check` 无输出。初始基线因 worktree 尚无 `web/dist` 不能编译根包；执行前端生产构建后，最终全仓测试已通过。

## 模型与敏感信息检查

执行命令：

```powershell
rg -n 'video-ds-2\.0|video-ds-2\.0-fast|as-sd2\.0-fast' constant relay controller service web/src
```

结果：退出码 `1`，无匹配；这是负向检查的期望结果。生产代码中没有写入文档 HTML 示例模型，ZZone 的前后端静态模型目录均为空，运行模型必须来自 `sd收录.xlsx`、配置导入或已发布快照。

执行命令：

```powershell
rg -n 'mock-zzone-key|Authorization: Bearer' --glob '!**/*_test.go' --glob '!docs/new-channels/cn-zzone.html'
```

结果：退出码 `0`。宽泛的 `Authorization: Bearer` 条件命中仓库既有 API 文档、示例和实现计划；`mock-zzone-key` 只命中本任务实现计划。随后对生产目录执行专项检查：

```powershell
rg -n 'mock-zzone-key' constant relay controller service web/src --glob '!**/*_test.go' --glob '!**/*.test.ts'
```

结果：退出码 `1`，无匹配；这是负向检查的期望结果。Mock Key 未进入生产文件。

执行检查：

```powershell
Select-String -Path web/scripts/channel-model-template/conversion-rules.json -Pattern '"14": "CH-ZZONE"'
```

结果：退出码 `0`，在第 `17` 行找到 `"14": "CH-ZZONE"`。

Mock Key `mock-zzone-key` 只存在于测试和实现计划中，未进入 `constant`、`relay`、`controller`、`service` 或 `web/src` 的生产文件。渠道 Key 不写日志、不进入公开任务响应；跨域内容重定向会移除 `Authorization`。

## 真实上游验收

未执行。当前未提供 ZZone API Key，因此没有验证真实提交、状态枚举、内容下载、鉴权错误或限流行为。Mock 契约通过不能替代真实上游 Canary；渠道继续保持 disabled。

本次未验证以下外部事实：

- ZZone 生产环境的真实鉴权、限流和错误体。
- 上游实际状态枚举及状态转换时序。
- 完成态无 URL 时 `/content` 的真实媒体响应、Range 行为和跨域重定向。
- 已导入模型在真实 ZZone 账号下的可用性、时长、比例和素材能力。

获得测试专用 API Key 后，应保持渠道禁用，先对一个已导入模型执行最小真实 Canary；核对提交体、轮询状态、内容下载、账单与日志脱敏后，再决定是否启用渠道。
