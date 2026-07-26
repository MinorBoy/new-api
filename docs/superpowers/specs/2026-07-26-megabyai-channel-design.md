# MegaByAI Seedance 渠道接入设计

**日期：** 2026-07-26  
**状态：** 已确认，进入接入计划阶段

## 1. 目标

为 MegaByAI 的三个 Seedance 视频模型增加独立任务渠道，使用户继续使用标准 Ark SDK 调用：

- `POST /api/v3/contents/generations/tasks`
- `GET /api/v3/contents/generations/tasks/{id}`
- `GET /api/v3/contents/generations/tasks`

用户不修改业务代码。接入支持文生视频以及参考图片、参考视频、参考音频，公开响应只暴露 new-api 生成的 Ark 任务 ID。

删除任务和视频内容代理下载不属于本次硬性要求。完成任务直接在 Ark `content.video_url` 返回 MegaByAI 的内容地址。

## 2. 已确认的上游契约

- 默认 Base URL：`https://newapi.megabyai.cc`
- 鉴权：`Authorization: Bearer <API_KEY>`
- 创建：`POST /v1/videos`，JSON
- 查询：`GET /v1/videos/{task_id}`
- 模型：`videos-standard`、`videos-fast`、`videos-mini`
- 时长：4 至 15 秒，默认 5 秒
- 比例：`16:9`、`9:16`、`1:1`
- 分辨率：`480p`、`720p`
- 参考图最多 9 张，参考视频最多 3 个，参考音频最多 3 个
- 素材仅支持公开 HTTP(S) URL
- 上游不支持 `first_image`、`last_image` 字段
- 状态：`queued`、`in_progress`、`completed`、`failed`

## 3. 架构决策

采用“共享 `newapivideo` 任务核心 + `/v1/videos` 请求方言 + MegaByAI profile”。Lucen 计划先在 `relay/channel/task/newapivideo/` 建立 profile 机制，本接入继续扩展该机制：

1. 共享核心负责鉴权、提交、轮询、公开/私有任务 ID、状态投影、Ark 查询响应和任务计费生命周期。
2. `/v1/videos` 方言负责把 Ark `content` 拆为 `referenceImages`、`referenceVideos`、`referenceAudios`，并统一解析直接任务响应。
3. MegaByAI profile 只声明端点、模型目录、媒体规则和字段映射，不复制任务框架。
4. 标准 Ark 模型名继续由路由策略映射到 MegaByAI 上游模型，公开响应返回客户端原始模型名。

备选方案及取舍：

- 独立复制一套 MegaByAI adaptor：实现直接，但会重复 NewAPIVideo 的任务、计费和隐私逻辑，不采用。
- 使用 Advanced Custom 原样透传：无法把 Ark `content` 转换为 MegaByAI 数组字段，也无法完成任务查询响应转换，不采用。

## 4. Ark 请求语义

- `model`、`content` 必填，且必须恰有一个非空文本项。
- Ark `duration` 转为上游整数 `duration`。
- Ark `ratio` 原样转为上游 `ratio`。
- Ark `resolution` 原样转发，只允许 `480p`、`720p`。
- `reference_image`、`reference_video`、`reference_audio` 分别进入三个上游数组。
- 单张无 role 或 `first_frame` 图片可作为普通参考图进入 `referenceImages`。
- `last_frame` 以及严格首尾帧组合返回 `InvalidParameter.content`，不得降级成普通多图参考。
- 只接受公开 HTTP(S) URL；`data:`、`asset://` 和其他协议在调用上游前返回 400。
- 参考音频不能单独存在，必须同时含图片或视频。
- `generate_audio` 不映射为上游字段；若显式为 `false` 且请求含参考音频，返回参数冲突。其他情况下不以该字段承诺上游音轨行为。
- `callback_url`、`return_last_frame`、`priority`、`execution_expires_after`、`service_tier`、`draft`、`tools` 等上游无法兑现的控制字段返回明确的 Ark 参数错误，不静默丢弃。

## 5. 响应、错误和隐私

- 创建成功只向客户端返回 `{"id":"task_<public>"}`。
- 私有上游 `id/task_id` 只保存于任务私有数据，用于轮询。
- `completed` 只有在 `video_url`、`url`、`metadata.content_url` 或 `metadata.local_url` 至少一个非空时才转为 Ark `succeeded`。
- 视频地址优先级为 `video_url`、`url`、`metadata.content_url`、`metadata.local_url`。
- `failed` 保留上游 `error.code` 与清洗后的 `error.message`；凭据、签名 URL 查询参数和内部任务信息不得进入普通用户日志。
- `completed_at` 非零不能单独判定成功。
- Ark 单查和列表均只使用公开任务 ID，并按当前用户隔离。

## 6. 路由和计费

- 渠道 profile 不维护一套重复的销售模型能力矩阵；模型路由目标负责标准模型到上游模型、分辨率、时长、比例和多模态条件。
- adaptor 仍执行上游硬边界校验，避免路由配置错误造成不可信请求。
- 创建阶段使用现有按时长或管理员配置的固定价格预扣。
- 上游没有权威 Token usage；`metadata.cost_credits` 仅供管理员诊断，不作为用户销售结算依据。
- 成功任务保持既有预扣/结算规则，失败任务走公共全额退款路径。
- 所有 quota 转换继续使用 `common/quota_math.go` 和现有饱和审计。

## 7. 管理端

新增普通 MegaByAI 渠道类型：

- 默认 Base URL `https://newapi.megabyai.cc`
- 普通 API Key 输入
- 三个上游模型选择项
- task-only 提示，禁用通用聊天渠道测试
- 不增加专用分组、多 Key 或隐藏 JSON 配置

管理员通过现有模型映射和路由策略把客户端 Seedance 模型绑定到三个上游模型。

## 8. 测试与验收

- 请求翻译覆盖纯文本、图片、视频、音频、混合素材和所有上限。
- 覆盖 `last_frame`、非 HTTP URL、纯音频、非法时长/比例/分辨率和未支持控制字段。
- 响应投影覆盖四种状态、四个结果 URL 位置、失败错误和 URL 缺失的伪成功。
- mock E2E 必须从 Ark 创建入口进入，经过分发、预扣、上游提交、轮询、Ark 单查/列表和失败退款。
- 真实验收至少覆盖 `videos-mini` 的纯文本和图片+音频场景，并核对最终 MP4、时长、公开 ID 隔离和账单。

## 9. 非目标

- 不实现 Ark DELETE。
- 不代理 `/v1/videos/{task_id}/content` 的二进制流。
- 不把 `cost_credits` 直接换算成用户 quota。
- 不伪造 MegaByAI 未支持的严格首尾帧语义。

