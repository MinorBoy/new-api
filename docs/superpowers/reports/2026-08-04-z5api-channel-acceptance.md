# Z5API Seedance 渠道验收报告

## 验收结论

Z5API 的本地协议、Ark SDK 兼容、任务生命周期、公开任务隐私、计费与失败退款自动化验收通过。

真实 Z5API 上游契约验收尚未执行：本机未提供 `Z5API_API_KEY`，因此没有向真实上游发送请求，也没有生成虚假的真实验收通过结论。渠道类型 `211` 继续保持禁用，待提供临时凭据后再执行真实门禁。

## 已完成的本地验证

| 验收项 | 结果 | 证据 |
| --- | --- | --- |
| Z5API 请求协议 profile | 通过 | `relay/channel/task/newapivideo/z5api_request_test.go` |
| 文生、图片、视频、音频混合请求编码 | 通过 | Z5API focused tests、E2E 精确 JSON 断言 |
| 图片/视频/音频数量、公网 URL、角色和时长校验 | 通过 | request tests、route contract tests、E2E 本地 400 |
| `/v1/videos` 提交与 `/v1/videos/{task_id}` 轮询 | 通过 | `e2e/z5api_upstream_e2e_test.go` |
| `processing`、`completed/object/seconds`、`failed` 状态转换 | 通过 | Z5API response tests、Ark 生命周期 E2E |
| Ark 单查、列表和公开任务 ID 隔离 | 通过 | 生命周期 E2E 与响应转换测试 |
| 成功结算、失败退款、重复轮询不重复退款 | 通过 | Z5API E2E |
| 类型注册、配置导入和默认禁用 | 通过 | `constant`、`service`、前端渠道测试 |
| 前端 typecheck 和生产构建 | 通过 | `bun run typecheck`、`bun run build` |

## 自动化命令

```text
go test ./relay/channel/task/newapivideo ./constant ./relay ./controller ./service ./e2e -count=1
go build ./...
Set-Location web; bun test --parallel=1 tests/channel-type-config.test.ts src/channel-config-converter/__tests__/v1.test.ts
Set-Location web; bun run typecheck
Set-Location web; bun run build
```

上述命令均通过。`go vet ./...` 仍报告项目已有问题，包括 `common/custom-event.go` 的锁复制、`common/email_test.go` 的 IPv6 地址格式，以及多个既有适配器的 unreachable code；本次 Z5API 接入未引入或修改这些问题。

## 真实验收门禁

真实验收只允许从本机环境变量读取：

```text
Z5API_BASE_URL（可选）
Z5API_API_KEY（必需）
```

获得凭据后，必须通过 Ark `/api/v3/contents/generations/tasks` 提交至少一个导入模型，覆盖文生和一项多模态请求，轮询四种状态，确认 `object` 视频 URL、`seconds`、MP4 可读性、计费、退款和公开任务隐私。真实响应如与当前设计不符，应先更新设计与失败测试，再修改实现。

