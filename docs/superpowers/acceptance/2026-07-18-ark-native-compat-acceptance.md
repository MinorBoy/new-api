# ARK Native / Seedance 改动验收清单

## 1. 路由与鉴权

- [x] `POST /api/v3/contents/generations/tasks` 进入 ARK Native 视频提交链路。
- [x] `GET /api/v3/contents/generations/tasks/:task_id` 进入 Native 单任务查询链路。
- [x] `GET /api/v3/contents/generations/tasks` 进入 Native 列表查询链路。
- [x] Seedance Native 视频和图片入口均经过 `TokenAuth`；无效令牌应在到达上游前被拒绝。
- [x] 任务查询按当前用户过滤；另一个用户不能读取任务。
- [x] 公开任务 ID 与上游任务 ID 分离，公开 ID 使用 `task_*` 格式。

## 2. Seedance Native 请求解析

- [x] 模型别名在提交前保留为 `OriginModelName`，映射后的真实模型写入 `UpstreamModelName`。
- [x] `content` 支持 `text`、`image_url`、`video_url`、`audio_url`、`draft_task` 的结构化解析。
- [x] Seedance 2.0 多模态请求同时包含参考图、参考视频、参考音频和提示词时，四类元素均保留。
- [x] 参考媒体的 `role` 分别保留为 `reference_image`、`reference_video`、`reference_audio`。
- [x] `watermark:false`、`duration`、`ratio`、`resolution`、`generate_audio`、`service_tier` 以及未知字段透传。
- [x] 原始请求缺失字段与显式零值/`false` 的语义不混淆。

## 3. 能力限制与错误路径

- [x] 音频不能脱离图像或视频单独提交。
- [x] Seedance 2.0/Fast/Mini 的参考视频、参考音频、参考图能力边界按模型族校验。
- [x] Fast/Mini 不支持 `1080p`，在访问上游前返回 400。
- [x] 1.5 Pro 的 `generate_audio` 默认值、`draft`、`flex` 和 `480p` 约束有单元回归覆盖。
- [x] 时长、帧数、媒体数量、工具和其他原生字段的范围校验有单元回归覆盖。
- [x] `draft_task.id`、视频/音频参考素材 `role` 等必填协议字段有错误路径覆盖。

## 4. 上游请求与响应转换

- [x] 上游提交 URL 为 `/api/v3/contents/generations/tasks`，Authorization 使用渠道 key。
- [x] 上游收到真实 Seedance 2.0 模型 ID，不收到客户端别名。
- [x] 上游响应的真实 `id` 仅保存到私有任务数据，不出现在公开提交、查询和列表响应中。
- [x] 上游 `pending/queued/processing/running/succeeded/failed` 状态转换为内部任务状态，再转换为 ARK `queued/running/succeeded/failed`。
- [x] 上游视频 URL 写入任务结果，并在公开响应中保留。
- [x] 上游 `resolution`、`completion_tokens`、`total_tokens` 保留为终态计费事实。

## 5. 计费、退款与统计

- [x] 预扣使用映射前客户端模型的计费配置和请求事实快照。
- [x] 终态响应分辨率覆盖提交时分辨率对应的 `video_input` 倍率。
- [x] `completion_tokens` 存在时优先于 `total_tokens`；显式 `0` 不被当成缺失。
- [x] 最终 token、分辨率、服务层级、参考视频、生成音频和 draft 事实写入 `BillingContext`/日志。
- [x] 预扣、正差额、负差额和失败退款使用有界 quota 数学转换。
- [x] 用户 quota、用户 used quota、渠道 used quota、令牌 used quota 和 `quota_data` 调整保持守恒。
- [x] `quota_data.Count` 不因差额结算重复增加；token 使用量在同一小时/用户/模型/渠道/令牌桶内补齐。
- [x] 相关单元测试覆盖异步退款、差额结算、看板守恒和 token 边界。

## 6. 图像 Native 回归

- [x] Seedream Native 图片请求只替换模型字段，`watermark`、`seed`、未知字段等原始字段保留。
- [x] 组图预扣与 `generated_images` 权威计费回归已覆盖。
- [x] `generated_images=0`、`completion_tokens:null` 和 OpenAI usage normalization 回归已覆盖。

## 7. 本轮自动化验收出口

- [x] 使用本地 `httptest.Server` 模拟 ARK Doubao，不发送真实上游请求、不产生真实费用。
- [x] 用户公开入口与 ARK SDK 固定路径一致，不需要 `/seedance` 前缀；旧 `/seedance/...` 路径返回 404。
- [x] ARK SDK 只需把 `base_url` 设置为 `https://<new-api-host>/api/v3`，调用代码和任务轮询逻辑不变。
- [x] 自动化测试覆盖 Seedance 2.0 参考图 + 参考视频 + 参考音频 + 提示词完整组合。
- [x] 自动化测试覆盖提交、单查、列表、轮询终态、跨用户隔离、非法能力组合。
- [x] 自动化测试输出客户端请求、mock 上游请求、转换结果、响应和计费断言。
