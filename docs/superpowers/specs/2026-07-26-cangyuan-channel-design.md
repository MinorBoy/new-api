# 苍原算力 Seedance 渠道接入设计

**日期：** 2026-07-26  
**状态：** 已确认，进入接入计划阶段

## 1. 目标

为苍原算力的视频任务接口增加独立渠道类型，使标准 Ark SDK 可通过以下接口完成创建、单查和列表，用户调用代码不变：

- `POST /api/v3/contents/generations/tasks`
- `GET /api/v3/contents/generations/tasks/{id}`
- `GET /api/v3/contents/generations/tasks`

本期只承诺本地文档已经明确的 Seedance 文生视频能力。未有契约证据的参考图片、参考视频和参考音频不得静默删除或猜测字段。

## 2. 已确认的上游契约

- 默认 Base URL：`https://ai.cangyuansuanli.cn`
- 鉴权：`Authorization: Bearer <API_KEY>`
- 创建：`POST /v1/videos`，文档确认 JSON 或 multipart；本接入固定使用 JSON
- 查询：`GET /v1/videos/{task_id}`
- 可选内容下载：`GET /v1/videos/{task_id}/content`
- 示例 Seedance 模型：`seedance-2.0-720p`
- 示例字段：`model`、`prompt`、`aspect_ratio`、`duration`，并提及 `resolution`
- 状态：`queued`、`in_progress`、`completed`、`failed`
- 结果可从任务响应或 `data[0].url` 取得

本地资料没有给出完整模型表、参考素材字段、完整响应 JSON 和错误 envelope。因此初始接入只暴露文档可证实的请求面；管理员仍可手工增加上游模型并通过模型映射使用。

## 3. 架构决策

苍原算力复用 Lucen/MegaByAI 已建立的 `newapivideo` profile 核心，新增 Cangyuan JSON 方言：

1. `newapivideo` 共享任务核心负责公开 ID、提交、轮询、Ark 转换和计费生命周期。
2. Cangyuan profile 使用 `/v1/videos`，把 Ark 文本、时长、比例和分辨率映射为上游字段。
3. 直接任务响应解析器增加 `data[0].url` 结果位置，但保持成功必须有 URL 的闭合规则。
4. 模型和能力筛选继续由路由策略负责；profile 只执行已确认的上游硬约束。

备选方案：独立 adaptor 会复制公共任务逻辑；直接透传 Ark JSON 与上游 `prompt/aspect_ratio` 契约不匹配。两者均不采用。

## 4. Ark 请求语义

- 请求必须包含 `model` 和恰好一个非空文本项。
- Ark `duration` 转为上游整数 `duration`，上限继续受公共视频时长限制。
- Ark `ratio` 转为 `aspect_ratio`；不在 adaptor 中自行填默认值，缺省值由路由事实或上游处理。
- Ark `resolution` 转为 `resolution`。
- 任何图片、视频、音频或 `draft_task` 输入均返回 `InvalidParameter.content`，直到新的上游资料和真实验收明确字段语义。
- 影响能力或计费的字段不能静默忽略。未支持的 `generate_audio`、`draft`、`tools`、`service_tier` 等显式值返回 400。
- `routing` 是 new-api 内部字段，只用于候选选择，不发送给上游。

## 5. 响应、错误和隐私

- 创建只返回公开 Ark 任务 ID。
- 上游任务 ID 只写入 `TaskPrivateData.UpstreamTaskID`。
- 结果 URL 支持顶层 `video_url/url/result_url`、`metadata.url/content_url/local_url`、`content.video_url`、`data.url` 和 `data[0].url`。
- 只有终态成功状态且 URL 非空才标记成功。
- 404/410 映射为任务不存在或过期；4xx 业务错误映射为失败；5xx 和网络错误保持可重试。
- 单查和列表继续按用户隔离，不能回显上游模型名、渠道 ID、上游任务 ID或管理员信息。

## 6. 路由和计费

- 标准 Ark 模型通过现有路由目标映射到 `seedance-2.0-720p` 或管理员手工配置的其他苍原模型。
- 路由目标负责限定文生、比例、分辨率和时长，避免不支持的请求进入候选。
- adaptor 保留服务器侧边界校验作为第二道保护。
- 价格按管理员配置的按次或按时长规则预扣；上游没有可验证 usage 时不得臆造 Token 结算。
- 成功保持合法结算，失败由公共任务逻辑退款。

## 7. 管理端

- 新增苍原算力 task-only 渠道类型。
- 默认 Base URL 为 `https://ai.cangyuansuanli.cn`。
- 初始模型目录只包含文档示例 `seedance-2.0-720p`，模型输入仍允许管理员手工补充。
- 不增加专用分组字段或多 Key 结构。
- 通用聊天渠道测试禁用，改由 Ark 视频 mock/真实验收验证。

## 8. 测试与验收

- 单测精确断言 `ratio -> aspect_ratio`、模型映射、时长、分辨率、Authorization 和端点。
- 断言任何媒体内容都在访问上游前返回 400，不能静默退化为文生。
- 响应测试覆盖顶层 URL、`data[0].url`、失败、未知状态和 URL 缺失成功。
- mock E2E 覆盖 Ark 创建、轮询、单查、列表、失败退款和公开 ID 隔离。
- 真实验收使用 `seedance-2.0-720p` 完成一次文生任务，并保存脱敏的创建/查询响应作为后续契约 fixture。
- 在没有真实验收结果前，生产发布清单必须把该渠道标记为未验收，不能宣称参考素材已支持。

## 9. 非目标

- 不实现未文档化的苍原多模态字段。
- 不实现 Ark DELETE 或内容代理下载。
- 不自动抓取模型广场页面作为运行时模型目录。

