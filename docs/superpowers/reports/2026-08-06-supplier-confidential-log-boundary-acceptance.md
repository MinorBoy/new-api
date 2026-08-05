# 供应商敏感信息日志边界验收报告

## 验收结论

本轮完成使用日志、任务日志、成本核算和媒体预览的角色权限根治。普通用户接口改为服务端强类型白名单投影，未知字段默认丢弃；前端再执行公开字段投影，不把界面隐藏当作权限边界。管理员接口和具备对应管理权限的页面继续保留渠道、供应商成本、路由尝试和审计能力。

普通用户不能从 API、桌面页面、移动页面或媒体预览中获得渠道信息、供应商名称、渠道 Base URL、API Key、上游模型 ID、供应商价格、成本、路由尝试、审计历史、原始请求、上游响应或供应商资源 URL。成本核算详情对普通用户返回 HTTP 403。Seedance、Suno 和非 Seedance 视频任务均只返回用户侧事实及 new-api 本地媒体代理地址。

实现分支为 `codex/supplier-log-confidentiality`，基线为本地 `ysr@458b8f296`。主体实现提交截至 `2da8b6c9b`；本报告与媒体安全审查修复位于后续收尾提交。

## 角色与权限矩阵

| 数据面 | 普通用户 | 管理员/对应管理权限 |
| --- | --- | --- |
| `/api/log/self`、`/api/log/token` | 仅用户请求 ID、用户 Token 名、客户端模型、最终消耗、公共计量和任务标记 | 不适用 |
| `/api/log/` | 无权访问管理员接口 | 保留渠道、分组、上游请求 ID、价格、成本关联、路由和审计信息 |
| `/api/task/self` | 仅公共任务 ID、客户端模型、状态、进度、时间、最终消耗和公开任务结果 | 不适用 |
| `/api/task/` | 无权访问管理员接口 | 保留渠道、平台、路径、原始请求、上游响应、上游任务事实和最终结果 |
| 成本核算请求详情 | HTTP 403 | 保留供应商成本、规则、尝试时间线、利润和审计历史 |
| 普通使用日志页面 | 不显示渠道、分组、上游请求、价格拆分、成本、尝试和审计入口 | 管理员页面完整显示 |
| 普通任务日志页面 | 不显示用户、渠道、平台、端点、原始请求和上游响应 | 管理员页面完整显示 |
| 媒体预览 | 仅请求同账号本地鉴权代理 URL | 可使用同一代理；管理员审计数据仍在管理详情中 |

这里的“用户”明确指普通终端用户。管理员以及拥有成本核算、渠道或任务审计权限的管理账号仍可查看职责范围内的完整供应商信息。

## 服务端安全边界

普通使用日志不再复用数据库日志模型，也不再依靠删除少数字段的黑名单。`ProjectPublicLog` 只序列化公开 DTO；`other` 直接解码到固定结构，未知键自动丢弃；`content` 固定为空，避免管理、登录或历史日志正文成为旁路。

普通任务日志由 `ProjectPublicTask` 投影，拒绝输出数据库 ID、用户、渠道、平台、分组、请求路径、原始请求、上游响应、上游任务 ID、计费上下文和供应商 URL。失败原因固定为用户侧通用错误，不回退到供应商原文。

任务结果合同如下：

- Seedance 成功结果强制补齐官方 Ark 公共字段，并把任务 ID、客户端模型和视频 URL覆盖为可信用户侧事实。
- Seedance 上游 `usage` 只有通过一致性校验后才进入公开结果；供应商返回的枚举字符串、模型、任务 ID和 URL不能覆盖公开字段。
- Suno 只保留 `title`、`text` 和本地 `audio/video/image` 媒体代理 URL。
- 非 Seedance 视频任务只返回 `/v1/videos/{公开任务ID}/content`，不返回数据库中保存的供应商资源地址。

普通接口忽略 `channel`、`channel_id`、`platform`、`group`、`user_id` 和 `upstream_request_id` 等管理员筛选维度，避免利用筛选命中数量探测内部渠道或供应商事实。

## API 递归扫描

E2E 对以下普通接口的完整 JSON 同时执行字段名和字符串值递归扫描：

```text
GET /api/log/self
GET /api/log/token
GET /api/task/self
```

扫描禁止字段包括 `channel`、`channel_id`、`channel_name`、`platform`、`group`、`upstream_request_id`、`upstream_model_name`、`user_request_data`、`upstream_response_data`、`request_path`、`model_price`、`duration_price`、`group_ratio`、`admin_info`、`audit_info`、`cost_accounting_request_id`、`rule_config_json` 和 `original_cost`。同时扫描供应商名称、上游模型、上游任务 ID、供应商域名和审计标记等注入值，结果均为 0 命中。

公开使用日志仍保留 new-api 请求 ID、客户端模型和最终用户消耗，且 `content=""`。公开 Seedance 任务结果包含 `usage`、时间、`seed`、`resolution`、`ratio`、`duration`、`framespersecond`、`service_tier`、`execution_expires_after`、`generate_audio`、`draft` 和 `priority`，视频 URL只指向本地代理。

管理员接口反向断言继续存在渠道、内部分组、上游请求 ID、上游模型、上游任务 ID、供应商资源事实、成本规则、供应商原价和审计标记，证明本轮没有以删除管理审计能力换取普通用户隔离。

## 媒体代理安全

媒体播放不再把供应商 URL交给浏览器。普通页面只接受与公开任务 ID严格匹配的本地路径；即使后端返回不同域名的 new-api 网关绝对地址，前端也只取经校验的本地路径发起鉴权请求。Bearer 凭据不会发送到任意外部来源。

视频和 Suno 媒体代理执行以下边界：

- 按当前账号查询任务，其他账号任务返回不存在。
- 只允许成功终态任务；Suno 还限制平台、媒体索引和 `audio/video/image` 类型。
- HTTP(S) 媒体始终使用拨号阶段 SSRF 防护的专用直连客户端，不使用渠道代理或 `HTTP_PROXY`/`HTTPS_PROXY` 解析不受信任的结果 URL。
- Gemini 媒体 URL必须与配置的 Gemini Base URL同源；API Key 只放入可信首跳的 `x-goog-api-key` 请求头，任何离开原始 Origin 的重定向都会删除该头，URL 和 `Referer` 均不携带密钥。
- 外部响应只转发 `Content-Type`、`Content-Length`、`Content-Range` 和 `Accept-Ranges`；供应商控制的 `Content-Disposition`、`ETag`、`Last-Modified`、`Server`、Cookie 和自定义请求 ID全部丢弃。
- `Content-Type` 按 `audio/video/image` 白名单验证，拒绝 HTML、SVG 和媒体类型错配；响应统一设置 `X-Content-Type-Options: nosniff`。
- SSRF 拦截和上游失败对用户只返回固定错误，不返回供应商主机、IP、端口、签名 URL或底层错误。
- 响应使用 `Cache-Control: private, no-store`，禁止共享缓存保存账号私有媒体。
- 前端通过鉴权请求取得 Blob 后才生成 Object URL；关闭弹窗、加载失败、重试或组件卸载都会中止旧请求并撤销 Object URL，请求版本号阻止旧响应覆盖新任务。

定向红绿回归先观察到渠道代理连接失败、Gemini 跨域重定向携带密钥、供应商标识头透传、共享缓存和 Object URL 未及时释放，随后验证上述行为全部被阻断。

## 页面验收

普通账号在桌面和窄屏移动视口验收 `/usage-logs/common?type=["2"]` 与 `/usage-logs/task`：

- 使用日志仅显示客户端模型、请求 ID、Token、用户侧用量和最终消耗。
- 任务日志仅显示客户端模型、公共任务 ID、状态、进度、最终消耗、公开任务详情和媒体预览。
- 页面文本和详情弹窗均不存在供应商名称、渠道列、成本核算、尝试时间线、审计历史、上游模型、端点、原始请求或上游响应。
- 桌面与移动布局未发现控件或文本重叠。
- 视频按钮通过本地鉴权代理加载；历史无效 mock 地址返回通用 502 和重试状态，不暴露原地址或凭据。

管理员页面验收确认：使用日志详情仍显示渠道、供应商成本核算、尝试时间线、冻结成本规则和审计历史；任务日志仍显示渠道、端点、原始请求、上游响应和任务详情；`/cost-accounting` 仍显示供应商成本、毛利润、毛利率和尝试次数。移动管理页面保留紧凑但完整的供应商审计上下文。

## 临时夹具清理

浏览器验收使用的两个临时普通账号均已删除。临时改绑的使用日志已恢复到原测试用户，临时改绑的任务已恢复原用户和原 mock 结果 URL；临时日志、Token、登录会话、用户记录及 Redis 会话版本键均已清理。数据库复核结果为临时用户、Token 和会话数量均为 0。

## 验证结果

提交前验证命令：

```text
go test ./controller ./model ./relay ./service ./e2e -count=1 -p=1
cd web && bun test --parallel=1 src/features/usage-logs
cd web && bun run typecheck
cd web && bun run build
cd web && oxlint -c .oxlintrc.json <本次变更前端文件>
git diff --check
```

当前验证证据：

- 后端 `controller`、`model`、`relay`、`service`、`e2e` 全部通过，E2E 包耗时约 26 秒。
- 使用日志前端测试 51 pass、0 fail，其中任务审计与鉴权媒体测试 12 pass、0 fail。
- `bun run typecheck` 通过。
- `bun run build` 通过。
- 本次变更的前端文件 Oxlint 通过。
- 最终独立安全复审未发现剩余 Critical 或 Important 问题。
- 项目全量 `bun run lint` 仍被本分支之外的既有错误阻断，例如 `channel-affinity/cache-stats-dialog.tsx`、`confirm-dialog.tsx` 和多个旧类型导入规则错误；本轮未修改这些文件，也未把无关全库 Lint 清理混入供应商保密修复。

最终提交和合并后将在本地 `ysr` 再执行关键后端测试、51 条使用日志前端测试、类型检查、构建和 `git diff --check`。
