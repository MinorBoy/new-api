# 8yes Seedance 渠道接入设计

## 目标

接入 8yes 视频生成上游，同时保持用户继续使用 Ark SDK 的
`/api/v3/contents/generations/tasks/*` 接口。用户不需要修改请求代码；网关负责将 Ark 任务转换为 8yes 的 OpenAI Videos 风格请求，并把上游私有任务 ID、API Key 和原始响应隔离在本地任务私有数据中。

## 上游契约

文档来源：`docs/new-channels/cn-8yes.md` 与 `docs/new-channels/cn-8yes.html`。

- Base URL：`https://8yes.cc`
- 创建：`POST /v1/videos`
- 查询：`GET /v1/videos/{task_id}`
- 内容：`GET /v1/videos/{task_id}/content`
- 鉴权：`Authorization: Bearer <token>`
- 请求示例使用 JSON，字段包括 `model`、`prompt`、`duration`、`ratio`、`referenceImages`、`referenceVideos`、`referenceAudios`。
- 文档页面同时描述了 OpenAI 风格的 `image`、`seed`、`metadata` 等可选字段。没有可靠的完整模型目录，也没有承诺轮询成功响应始终包含视频直链；因此模型映射由管理员配置，成功任务允许通过网关内容代理取回视频。

## 方案

新增 8yes provider profile，不改变共享 Ark task 生命周期：

1. Ark 创建请求先经过现有 JSON、媒体 URL、媒体数量、时长和计费校验。
2. 8yes dialect 将文本拼成 `prompt`，图片/视频/音频按内容顺序分别写入三个引用数组。`duration`、`ratio`、`seed` 使用指针或安全整数，显式零/false 不被静默丢弃；不支持的 `watermark`、非默认 `service_tier`、`draft`、`tools`、`callback_url` 返回 400。
3. 上游创建响应兼容 `id`、`task_id`、`taskId`，冲突 ID 拒绝；公共响应只返回本地 `task_*` ID。
4. 轮询状态映射为 Ark queued/running/succeeded/failed。8yes 成功状态如果没有直链，允许共享轮询逻辑将本地公开任务 ID 生成 `/v1/videos/{public_task_id}/content` 代理地址。
5. 内容代理按本地任务归属校验，从私有上游任务 ID 拼出 8yes `/v1/videos/{upstream_task_id}/content`，带上渠道 API Key，并复用现有 SSRF、代理和响应头处理。
6. 预扣、成功结算、失败退款、重复轮询幂等、列表过滤与用户隔离均继续使用共享 task service；8yes 只实现 provider profile、请求编码、响应解析、测试和渠道管理配置。

模型静态目录保持为空，避免把文档示例 `videos-4-mini-480p` 或旧配置中的 `seedance-2.0*` 误认为完整目录。已有导入映射保留上游名称，由管理员在真实验收前确认。

## 错误和安全边界

- 解析失败、未知状态、成功但内容代理无法准备时 fail closed，触发任务失败与一次退款。
- 上游错误信息执行现有敏感信息清理，不向用户暴露 API Key、上游任务 ID 或原始诊断字段。
- 图片、视频、音频必须是允许的 HTTP(S) 公共媒体 URL；请求大小、媒体数量和 duration 使用现有 Seedance 上限。
- `seed` 只允许整数 `-1..4294967295`，不做无界转换。
- 真实 API Key 只从环境变量读取，不能写入代码、测试 fixture、文档或 Git。

## 验收

先用 httptest 覆盖精确上游路径、请求 JSON、字段省略、任务状态、内容代理和 ID 隔离，再运行 Ark 创建/单查/列表、预扣/结算/退款 focused tests。提供 `EIGHTYES_API_KEY` 后才执行真实创建、轮询、内容下载和失败退款验收；通过前保持渠道禁用。
